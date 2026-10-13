package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/example/go-chain-data-demo/internal/webui"
)

type Config struct {
	RequestTimeout   time.Duration
	MaxLogBlockRange uint64
	Logger           *slog.Logger
}

type Handler struct {
	client           *ethclient.Client
	requestTimeout   time.Duration
	maxLogBlockRange uint64
	logger           *slog.Logger
}

func NewHandler(client *ethclient.Client, cfg Config) *Handler {
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 12 * time.Second
	}
	if cfg.MaxLogBlockRange == 0 {
		cfg.MaxLogBlockRange = 5000
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Handler{
		client:           client,
		requestTimeout:   cfg.RequestTimeout,
		maxLogBlockRange: cfg.MaxLogBlockRange,
		logger:           cfg.Logger,
	}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /api/v1/network", h.network)
	mux.HandleFunc("GET /api/v1/blocks/latest", h.latestBlock)
	mux.HandleFunc("GET /api/v1/blocks/{number}", h.blockByNumber)
	mux.HandleFunc("GET /api/v1/transactions/{hash}", h.transactionByHash)
	mux.HandleFunc("GET /api/v1/accounts/{address}/balance", h.balance)
	mux.HandleFunc("GET /api/v1/logs", h.logs)
	mux.Handle("GET /", webui.Handler())
	return h.logging(mux)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := h.rpcContext(r.Context())
	defer cancel()
	if _, err := h.client.BlockNumber(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "rpc_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) network(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := h.rpcContext(r.Context())
	defer cancel()
	chainID, err := h.client.ChainID(ctx)
	if err != nil {
		h.rpcError(w, err)
		return
	}
	latest, err := h.client.BlockNumber(ctx)
	if err != nil {
		h.rpcError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"chainId":           chainID.String(),
		"latestBlockNumber": latest,
	})
}

func (h *Handler) latestBlock(w http.ResponseWriter, r *http.Request) {
	h.writeBlock(w, r, nil)
}

func (h *Handler) blockByNumber(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.ParseUint(r.PathValue("number"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_block_number", "block number must be a decimal integer")
		return
	}
	h.writeBlock(w, r, new(big.Int).SetUint64(number))
}

func (h *Handler) writeBlock(w http.ResponseWriter, r *http.Request, number *big.Int) {
	ctx, cancel := h.rpcContext(r.Context())
	defer cancel()
	block, err := h.client.BlockByNumber(ctx, number)
	if err != nil {
		h.rpcError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, blockResponse(block))
}

func (h *Handler) transactionByHash(w http.ResponseWriter, r *http.Request) {
	rawHash := r.PathValue("hash")
	if !isHash(rawHash) {
		writeError(w, http.StatusBadRequest, "invalid_transaction_hash", "hash must be a 32-byte 0x-prefixed hex value")
		return
	}

	ctx, cancel := h.rpcContext(r.Context())
	defer cancel()
	tx, pending, err := h.client.TransactionByHash(ctx, common.HexToHash(rawHash))
	if err != nil {
		h.rpcError(w, err)
		return
	}
	chainID, err := h.client.ChainID(ctx)
	if err != nil {
		h.rpcError(w, err)
		return
	}

	response := transactionResponse(tx, chainID)
	response["pending"] = pending
	if !pending {
		receipt, receiptErr := h.client.TransactionReceipt(ctx, tx.Hash())
		if receiptErr == nil {
			response["receipt"] = map[string]any{
				"status":            receipt.Status,
				"blockNumber":       receipt.BlockNumber.Uint64(),
				"transactionIndex":  receipt.TransactionIndex,
				"gasUsed":           receipt.GasUsed,
				"effectiveGasPrice": bigIntString(receipt.EffectiveGasPrice),
				"contractAddress":   nullableAddress(receipt.ContractAddress),
				"logCount":          len(receipt.Logs),
			}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) balance(w http.ResponseWriter, r *http.Request) {
	rawAddress := r.PathValue("address")
	if !common.IsHexAddress(rawAddress) {
		writeError(w, http.StatusBadRequest, "invalid_address", "address must be a 20-byte hex address")
		return
	}

	blockNumber, err := parseOptionalBlockNumber(r.URL.Query().Get("block"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_block_number", err.Error())
		return
	}
	ctx, cancel := h.rpcContext(r.Context())
	defer cancel()
	wei, err := h.client.BalanceAt(ctx, common.HexToAddress(rawAddress), blockNumber)
	if err != nil {
		h.rpcError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"address": common.HexToAddress(rawAddress).Hex(),
		"block":   blockLabel(blockNumber),
		"wei":     wei.String(),
		"ether":   weiToEther(wei),
	})
}

func (h *Handler) logs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	rawAddress := query.Get("address")
	if !common.IsHexAddress(rawAddress) {
		writeError(w, http.StatusBadRequest, "invalid_address", "address query parameter is required")
		return
	}
	from, err := parseRequiredUint64(query.Get("fromBlock"), "fromBlock")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_block_range", err.Error())
		return
	}
	to, err := parseRequiredUint64(query.Get("toBlock"), "toBlock")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_block_range", err.Error())
		return
	}
	if to < from || to-from > h.maxLogBlockRange {
		writeError(w, http.StatusBadRequest, "invalid_block_range", fmt.Sprintf("range must be ordered and no larger than %d blocks", h.maxLogBlockRange))
		return
	}

	filter := ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(from),
		ToBlock:   new(big.Int).SetUint64(to),
		Addresses: []common.Address{common.HexToAddress(rawAddress)},
	}
	if rawTopic := query.Get("topic0"); rawTopic != "" {
		if !isHash(rawTopic) {
			writeError(w, http.StatusBadRequest, "invalid_topic", "topic0 must be a 32-byte 0x-prefixed hex value")
			return
		}
		filter.Topics = [][]common.Hash{{common.HexToHash(rawTopic)}}
	}

	ctx, cancel := h.rpcContext(r.Context())
	defer cancel()
	entries, err := h.client.FilterLogs(ctx, filter)
	if err != nil {
		h.rpcError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		topics := make([]string, len(entry.Topics))
		for i, topic := range entry.Topics {
			topics[i] = topic.Hex()
		}
		items = append(items, map[string]any{
			"address":          entry.Address.Hex(),
			"topics":           topics,
			"data":             "0x" + hex.EncodeToString(entry.Data),
			"blockNumber":      entry.BlockNumber,
			"transactionHash":  entry.TxHash.Hex(),
			"transactionIndex": entry.TxIndex,
			"blockHash":        entry.BlockHash.Hex(),
			"logIndex":         entry.Index,
			"removed":          entry.Removed,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(items), "items": items})
}

func (h *Handler) rpcContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, h.requestTimeout)
}

func (h *Handler) rpcError(w http.ResponseWriter, err error) {
	if errors.Is(err, ethereum.NotFound) {
		writeError(w, http.StatusNotFound, "not_found", "chain data was not found")
		return
	}
	writeError(w, http.StatusBadGateway, "rpc_error", err.Error())
}

func (h *Handler) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		h.logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}

func blockResponse(block *types.Block) map[string]any {
	transactions := make([]map[string]any, 0, len(block.Transactions()))
	for _, tx := range block.Transactions() {
		transactions = append(transactions, map[string]any{
			"hash":  tx.Hash().Hex(),
			"to":    nullableAddressFromPointer(tx.To()),
			"value": tx.Value().String(),
			"nonce": tx.Nonce(),
		})
	}
	return map[string]any{
		"number":           block.NumberU64(),
		"hash":             block.Hash().Hex(),
		"parentHash":       block.ParentHash().Hex(),
		"timestamp":        block.Time(),
		"timestampISO8601": time.Unix(int64(block.Time()), 0).UTC().Format(time.RFC3339),
		"gasLimit":         block.GasLimit(),
		"gasUsed":          block.GasUsed(),
		"baseFeePerGas":    bigIntString(block.BaseFee()),
		"transactionCount": len(transactions),
		"transactions":     transactions,
	}
}

func transactionResponse(tx *types.Transaction, chainID *big.Int) map[string]any {
	from := ""
	if sender, err := types.Sender(types.LatestSignerForChainID(chainID), tx); err == nil {
		from = sender.Hex()
	}
	return map[string]any{
		"hash":                 tx.Hash().Hex(),
		"type":                 tx.Type(),
		"chainId":              bigIntString(tx.ChainId()),
		"from":                 from,
		"to":                   nullableAddressFromPointer(tx.To()),
		"nonce":                tx.Nonce(),
		"valueWei":             tx.Value().String(),
		"gasLimit":             tx.Gas(),
		"gasPrice":             bigIntString(tx.GasPrice()),
		"maxFeePerGas":         bigIntString(tx.GasFeeCap()),
		"maxPriorityFeePerGas": bigIntString(tx.GasTipCap()),
		"input":                "0x" + hex.EncodeToString(tx.Data()),
	}
}

func parseOptionalBlockNumber(value string) (*big.Int, error) {
	if value == "" || strings.EqualFold(value, "latest") {
		return nil, nil
	}
	number, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("block must be latest or a decimal integer")
	}
	return new(big.Int).SetUint64(number), nil
}

func parseRequiredUint64(value, name string) (uint64, error) {
	if value == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a decimal integer", name)
	}
	return parsed, nil
}

func isHash(value string) bool {
	if len(value) != 66 || !strings.HasPrefix(value, "0x") {
		return false
	}
	_, err := hex.DecodeString(value[2:])
	return err == nil
}

func blockLabel(number *big.Int) any {
	if number == nil {
		return "latest"
	}
	return number.Uint64()
}

func weiToEther(wei *big.Int) string {
	value := new(big.Float).SetPrec(256).SetInt(wei)
	value.Quo(value, new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)))
	return value.Text('f', 18)
}

func bigIntString(value *big.Int) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func nullableAddress(address common.Address) any {
	if address == (common.Address{}) {
		return nil
	}
	return address.Hex()
}

func nullableAddressFromPointer(address *common.Address) any {
	if address == nil {
		return nil
	}
	return address.Hex()
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
