# erc20scaner：ERC20 扫块与查询（库 + 文档）

原先是一个独立 module（`erc20Scaner/`，自带 go.mod / Dockerfile / 打包脚本）。2026-10 起已并入主 module，
**程序入口都在仓库 `cmd/` 下**，这里只放库代码和文档。

```bash
# 程序（部署包里的名字见括号）
go build ./cmd/erc20scan      # scanner ：按 seq 拉块解析 ERC20 转账写 MySQL
go build ./cmd/erc20api       # erc20api：/evmapi/* 查询 MySQL
go build ./cmd/tools/erc20-{txinfo,blockparse,scannerfix}   # 离线工具，见 docs/tools/

# 配置：externaldb.toml 的 [mysql] + [erc20scan]
#   [mysql]   enabled / dsn
#   [erc20scan] ethUrl / startBlock / endBlock / skipInlineBalanceUpdate / apiPort
#   [erc20scan.balanceRefresher] / [erc20scan.log]
```

## 目录

| 路径 | 内容 |
| --- | --- |
| `scanner/engine/` | 扫块主循环；chain33 模式（按 seq 调 `GetBlockBySeq`）与 node 模式（eth JSON-RPC） |
| `txparser/` | 交易 / 日志解析（ERC20 Transfer、Approval 等） |
| `erc20abi/` | ABI 定义与生成的 binding |
| `blockalign/` | seq ↔ height 对齐（chain33 记录的 seq 与区块高度差值） |
| `database/` | MySQL 模型、`schema.sql`、增量迁移 sql |
| `config/` | 配置结构；`LoadConfigFile` 按扩展名分发（`.toml` = externaldb.toml，其他按 yaml） |
| `logger/` | 结构化日志（slog + lumberjack，按 appName 加前缀） |
| `txinspect/` | 交易检查辅助 |
| `docs/dev/` | 设计/实现文档（交易记录、余额刷新、配置、升级、验证计划等） |
| `docs/rpc/` | `/evmapi/*` 接口文档与联调用例 |
| `docs/tools/` | 离线工具说明 |

## 已知待办

- 扫块进度目前按 **height** 记（有 `alignScanStart` 用 seq-height 差值兜底），del seq 直接跳过；
  要对齐 convert 的 seq 语义，需要给进度加 seq 并支持回滚。详见 `docs/dev/` 与仓库 `docs/tasks.md`。
