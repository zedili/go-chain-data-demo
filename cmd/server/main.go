package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/example/go-chain-data-demo/internal/api"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	rpcURL := os.Getenv("RPC_URL")
	if rpcURL == "" {
		logger.Error("RPC_URL is required")
		os.Exit(1)
	}

	requestTimeout := envDuration("REQUEST_TIMEOUT", 12*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	client, err := ethclient.DialContext(ctx, rpcURL)
	cancel()
	if err != nil {
		logger.Error("connect to RPC", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	chainIDCtx, chainIDCancel := context.WithTimeout(context.Background(), requestTimeout)
	chainID, err := client.ChainID(chainIDCtx)
	chainIDCancel()
	if err != nil {
		logger.Error("read chain id", "error", err)
		os.Exit(1)
	}

	handler := api.NewHandler(client, api.Config{
		RequestTimeout:   requestTimeout,
		MaxLogBlockRange: envUint64("MAX_LOG_BLOCK_RANGE", 5000),
		Logger:           logger,
	})

	server := &http.Server{
		Addr:              envString("HTTP_ADDR", ":8080"),
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server started", "addr", server.Addr, "chain_id", chainID.String())
		serverErrors <- server.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-signals:
		logger.Info("shutdown requested", "signal", sig.String())
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server stopped", "error", err)
			os.Exit(1)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown", "error", err)
		os.Exit(1)
	}
}

func envString(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envUint64(name string, fallback uint64) uint64 {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
