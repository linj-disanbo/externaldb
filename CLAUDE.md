# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development

```bash
# Build all binaries（static, CGO_ENABLED=0）
make build

# Build only the convert module
make build_convert

# Build only the RPC module
make build_rpc

# Run all tests with race detection
make test

# Run tests without race detection (faster)
make testq

# Run vet checks
make vet

# Format Go, proto, and shell files
make fmt

# Regenerate protobuf types
make proto

# Generate swagger docs for rpc
make swag

# 部署包：老链路（sync/convert/rpc/jrpc，一套物料按 EX_SERVICE_TYPE 选场景）→ build/externaldb.tgz
make pkg
# 部署包：直连链路（5 程序 + 离线工具）→ build/externaldb-pull.tgz
make pkg-pull
```

The `Makefile` expects a `.env` file at the project root (included via `include .env`).

`make build` 产出：`build/{sync,convert,jrpc,rpc,sync_convert,pull_convert,convert_check,erc20scan,erc20api,account_refresh}`。
部分包测试依赖真实 ES/外网（如 `rpc/service/proof`、`jrpc` 的部分用例），本机跑会失败，用 `-run` 过滤。

## Architecture

两个链路并存：

```
老链路（保留备用）: chain33 node (push) → sync → ES(seq05_seq) → convert → ES → jrpc/rpc (query)
直连链路（当前生产）: chain33 node (gRPC pull + seq) → pull_convert → ES → jrpc (query)
```

- **pull_convert**（`cmd/pull_convert`, `util/cli/convert/{seq_source,pull_loop,seq_writer,seq_cursor}.go`）：直连节点，
  按 seq 拉块（`GetBlockSequences` + `GetBlockByHashes`）→ `ModuleConvert.DealBlockDetail` 展开 → 写 ES；
  进度是 append-only 的 `seq_marker` 索引（游标 = max(seq)，普通 bulk 写 + 启动时 refresh 一次）。
  `-mode fix -from -to` 可只修复一段 seq（不写 marker、不做统计累计）；切换/运维要点：停旧写方 → 起 convert（自动自举游标）→ `convert_check` 对账 → 观察 `seq_marker` 是否单调；
  ES 侧看 `seq_marker` 的 docs.count 与 translog，写放大时先调 `translog.flush_threshold_size`。
- **sync / convert**（`cmd/sync`, `cmd/sync_convert`, `cmd/convert`, `store/syncseq/`, `util/cli/convert`）：老链路，
  推送式 sync 把整块 seq 存 ES，convert 读 ES 再展开。**保留但不再作为新部署路径**（同一批索引只能有一个写方）。
- **jrpc**（`jrpc/`）：JSON-RPC 2.0 查询端点。每链一个 `rpc.Server`，注册 Account/Trade/Block/EVM 等服务；
  EVM 交易解析按合约地址从 ES `contractabi` 索引取 ABI（`jrpc/evmabi.go`，**只读**）；
  ABI 注册已从 jrpc 移除，改由仓库外私有工具（`local/tools/saveabi`，不随开源仓库发布）离线写入。
- **rpc**（`rpc/`）：Gin REST，proof 相关；与 jrpc 独立，含白名单与 Sentinel 限流中间件。
- **erc20 扫块/查询**：已并入主 module（不再是独立工程）。程序 `cmd/erc20scan`（扫 ERC20 转账写 MySQL）、
  `cmd/erc20api`（`/evmapi/*` 读 MySQL，原 `erc20Scaner/rpc`）；库在 `erc20scaner/`（`scanner/engine`、`txparser`、
  `database`、`config`、`erc20abi`、`blockalign`、`logger`、`txinspect`），文档在 `erc20scaner/docs/`；
  离线工具 `cmd/tools/erc20-{blockparse,scannerfix,txinfo}`。
- **account_refresh**（`cmd/account_refresh`, `util/accountrefresh/`）：常驻循环，把主币（BTY）真实余额
  按三档（top N 1h / 活跃 24h / 全量 168h）从链上 `Chain33.GetBalance` 校正回 ES `account` 文档
  （只部分更新 balance/frozen/total 与 `balance_refresh_at`；配置段 `[accountRefresh]`）。
- **离线/运维工具**：`cmd/convert_check`（对账，部署包里放 `tools/`）；`local/`（git 忽略）放不公开的过程文档与非通用工具：`local/tools/`、`local/docs/`。
- **部署物料**：两个包共用一套脚本 —— `deploy/pkg/`（包内 Makefile/README/compose/alter-config/es-init/browser-init/reindex-shards，老链路与直连包都从这里拷）、`deploy/env/`（ES7 模板 es7/es7.4 + proof-init.bash）；`deploy/` 下另有 pull-convert 包物料、docker/（Dockerfile、compose-ci）、ci/（CI 脚本与链配置模板）；`make pkgProof`/`make pkg-quickStart` 是 `make pkg` 的兼容别名，产物同名 `externaldb.tgz`。

## Key Abstractions

- `db.Convert` interface (`db/convert.go`): `ConvertTx(env *TxEnv, op int) ([]Record, error)` — every contract converter implements it.
- `db.Record` interface (`db/record.go`): ES record combining `Key` (Index/Type/ID) + `Operator` (Add/Del/Update) + `Value()`.
- `db.TxEnv`: Wraps a `types.BlockDetail` with transaction index and block hash, passed to every converter.
- `util.AppConvert` interface (`util/process.go`): Block-level convert with `ConvertBlock()` / `RecoverStats()`
  （直连链路另用 `RecoverStatsAt(height)` 让累计统计与游标高度对齐）; `DealBlockDetail` 是两条链路共用的块处理入口（del seq → 反做）。
- `store.SeqStore` / `store.SeqNumStore`（`store/`）：**老链路**的 seq 存储抽象（ES/Kafka）。
- `db/seqmark`：直连链路的进度索引（doc id = seq，字段 seq/height/hash/type）。

Converter plugins are registered in `db/` subdirectories (coins, trade, token, ticket, evm, evm/erc721, evm/erc1155, evm/nft, evmxgo, proof, pos33, coinsx, etc.) and imported via blank imports in `cmd/pull_convert/main.go`（老链路是 `cmd/convert/main.go`）。Each plugin registers itself in its `init()` function；新增 converter 记得同步两处 blank import。

## Configuration

The main config is `config/externaldb.toml`; 程序统一用 `-f`（直连程序）或 `-c`（erc20 程序）指定，部署时在 `etc/externaldb.toml`。
It configures:
- `[chain]`: chain33 node connection (gRPC `grpcHost`, HTTP `host`, title, symbol)
- `[syncEs]`（老链路 sync）/ `[convertEs]`: ES hosts and index prefixes（7.x）
- `[esIndex]`: 新建索引的分片/副本
- `[sync]`: 老链路推送服务（host/port/startSeq）
- `[rpc]`: jrpc 与 gin 的绑定地址、IP 白名单（`whiteList = ["*"]` 等于全开）
- `[convert]`: 处理哪些 exec/合约地址、appName、直连参数（`pullBatch`/`pullPollMs`/`pullDetailCache`/`pullMarkerKeep`）
- `[mysql]`: `enabled` + `dsn`，jrpc 的 `getTransferLogs` 与 erc20scan 落库共用
- `[erc20scan]`: `ethUrl`/`startBlock`/`endBlock`/`skipInlineBalanceUpdate`/`apiPort`，
  子段 `[erc20scan.balanceRefresher]`、`[erc20scan.log]`

Config types are generated from `proto/exdbconfig.proto`。改字段后手动生成：`cd proto && protoc --go_out=. exdbconfig.proto`
（`make proto` 用的 `proto/create_protobuf.sh` 仍是老的 `--go_out=plugins=grpc` 写法，protoc-gen-go v1.33 会报错；先确认重新生成只多出预期的 diff）。
erc20 程序读同一份 toml：`erc20scaner/config/toml.go` 的 `FromProto` 做映射（`.yaml` 老格式仍兼容）。

## ES Version Handling

只支持 ES 7.x：`escli/v6/` 已删除，客户端实现在 `escli/v7/`，`db.SetVersion()` 在启动时设置版本。
部署包也只有 ES7 模板（`deploy/env/es7` = 7.17 默认，`deploy/env/es7.4` = 7.4 备用），`.env` 里 `ES_VERSION` 填 6 会被 `make` 拦下。

## Dependencies

- `github.com/33cn/chain33` — the blockchain node (types, RPC, plugins)
- `github.com/33cn/plugin` — chain33 plugin definitions
- `github.com/ethereum/go-ethereum` — EVM ABI decoding / tx parsing
- `github.com/olivere/elastic` / `olivere/elastic/v7` — ES client
- `github.com/gin-gonic/gin` — REST API (rpc module)
- `github.com/go-sql-driver/mysql` — MySQL（erc20 scan/query、jrpc 可选）
- `github.com/Shopify/sarama` — Kafka client (configured but secondary)
