# Go 链上数据 Demo

这是一个面向 EVM 兼容链的只读链上数据服务。它通过 `go-ethereum/ethclient` 连接 Ethereum JSON-RPC 节点，对外提供 HTTP API，并内置了一套可视化链上数据控制台。

## 需要哪些外部 API？

### 1. 必需：EVM JSON-RPC

本 Demo 只需要一个 `RPC_URL`。它可以来自：

- 自建 Geth / Erigon / Nethermind / Besu 节点；
- Alchemy、Infura、QuickNode 等节点服务商；
- 本地 Anvil、Hardhat Node 或 Geth `--dev` 开发链。

服务实际使用的 RPC 方法如下：

| 场景 | JSON-RPC 方法 | Go 调用 |
|---|---|---|
| 网络识别 | `eth_chainId` | `ChainID` |
| 最新高度 | `eth_blockNumber` | `BlockNumber` |
| 区块详情 | `eth_getBlockByNumber` | `BlockByNumber` |
| 交易详情 | `eth_getTransactionByHash` | `TransactionByHash` |
| 交易回执 | `eth_getTransactionReceipt` | `TransactionReceipt` |
| 原生币余额 | `eth_getBalance` | `BalanceAt` |
| 合约事件日志 | `eth_getLogs` | `FilterLogs` |

> 结论：做基础链上数据读取，不需要 Etherscan API，也不需要钱包私钥。RPC 服务商的 API Key 通常直接包含在 `RPC_URL` 中。

### 2. 按需增加的 API

| 需求 | 建议 API | 为什么 JSON-RPC 不够 |
|---|---|---|
| 合约源码、已验证 ABI | Etherscan API 或各链浏览器 API | ABI/源码不是链上共识数据 |
| USD 价格、K 线、市值 | CoinGecko、CoinMarketCap 或交易所行情 API | 链上只有池子与成交原始数据，没有统一法币价格 |
| 地址标签、风险评分 | 浏览器标签库或风控供应商 API | “交易所地址/钓鱼地址”等标签是链下加工数据 |
| 大规模历史查询/聚合 | The Graph/Subgraph、自建索引器、数据仓库 | RPC 适合按哈希/高度读取，不适合复杂检索和统计 |
| 实时新区块/事件订阅 | WebSocket RPC | HTTP RPC 只能轮询，订阅需要 `eth_subscribe` |
| NFT 图片与 metadata | IPFS 网关、Arweave 网关、业务 HTTP 源 | 链上常只保存 URI |

## 架构与数据流

```text
HTTP Client
    |
    | GET /api/v1/...
    v
Go net/http API
    |
    | go-ethereum ethclient
    v
EVM JSON-RPC Provider
    |
    v
Ethereum / EVM-compatible chain
```

该服务不落库，因此没有表关系或事务一致性问题。请求数据来自节点当前视图；对一致性敏感的生产业务应固定 `blockNumber` 或 `blockHash`，并根据业务风险使用 `safe` / `finalized` 语义，而不是始终读取 `latest`。

## 快速启动

要求 Go 1.25+（当前锁定的 `go-ethereum v1.17.8` 所需版本）。

PowerShell：

```powershell
$env:RPC_URL = "https://你的节点地址/你的API_KEY"
go run ./cmd/server
```

`.env.example` 是配置项清单；程序直接读取进程环境变量，不会自动加载 `.env` 文件。

服务默认监听 `http://localhost:8080`。

## Web 控制台

启动服务后访问：

```text
http://localhost:8080/
```

控制台无需单独安装 Node.js 或执行前端构建，HTML、CSS 和 JavaScript 已通过 Go `embed` 打包进服务二进制。

页面包含：

- RPC 连接状态、Chain ID、最新区块与响应耗时；
- 最新区块摘要和完整 JSON 检视；
- 按高度查询区块；
- 按交易哈希查询交易及 receipt；
- 按地址和区块查询原生币余额；
- 按合约地址、区块范围和 Topic 0 查询事件日志；
- JSON 结果复制与浏览器本地查询历史；
- 桌面端与移动端响应式布局。

## Demo HTTP API

```text
GET /healthz
GET /api/v1/network
GET /api/v1/blocks/latest
GET /api/v1/blocks/{number}
GET /api/v1/transactions/{hash}
GET /api/v1/accounts/{address}/balance?block=latest
GET /api/v1/logs?address={contract}&fromBlock={n}&toBlock={n}&topic0={eventSignatureHash}
```

示例：

```powershell
Invoke-RestMethod http://localhost:8080/api/v1/network
Invoke-RestMethod http://localhost:8080/api/v1/blocks/latest
Invoke-RestMethod http://localhost:8080/api/v1/accounts/0x0000000000000000000000000000000000000000/balance
```

查询日志时必须提供合约地址和十进制区块范围；默认最大跨度为 5000 个区块，避免误扫整条链导致 RPC 超时或额度耗尽。

## 设计说明

- 只读 Demo 不接触私钥，也不包含发送交易接口。
- 金额在 JSON 中使用字符串，避免 JavaScript `Number` 精度丢失。
- 交易详情同时读取 receipt，用于展示成功状态、Gas 消耗、合约创建地址和日志数量。
- 日志返回原始 topics/data。生产业务通常应加入 ABI 解码层，再将事件转换成领域对象。
- 所有 RPC 请求都有超时；节点错误统一转换为 `502 rpc_error`，未找到数据返回 `404`。

## 从 Demo 演进到生产

1. 增加 RPC 主备切换、限流、重试和熔断，但不要对所有写操作盲目重试。
2. 若要持续同步数据，保存“最后已确认区块”，并处理链重组：先写入未确认数据，达到确认数后再标记 finalized；发现 parent hash 不连续时回滚受影响区块。
3. 为区块、交易、日志建立数据库索引；写入顺序通常是 block → transaction → receipt/log，并在同一数据库事务中维护一致性。
4. 合约事件使用 ABI 解码；代理合约还要处理 implementation 变更和历史 ABI 版本。
5. 不在日志、配置文件或仓库中保存 RPC Key、私钥、Token；生产环境使用密钥管理服务。

## 验证

```powershell
go test ./...
go vet ./...
```

如果节点能访问，再启动服务并调用 `/healthz` 与 `/api/v1/network` 做连通性验证。
