# erc20：ERC20 扫块与查询（库 + 文档）

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
| `docs/deploy/` | 部署参考（`mysql.cnf`：自建 MySQL 实例的参考参数，非部署物料，不影响程序逻辑） |

## 扫块进度与回滚（2026-10 起）

- 进度改为 append-only 的 `scan_seq_marker`（一个 seq 一行，游标 = `max(seq)`，del seq 也写），
  与 convert 的 `seq_marker` 同一套语义；老的 `scan_progress`（height）仍保留用于排查，
  首次启动没有 marker 时会按它回退一次。
- 回滚（方案 A）：每个 add 块在**事务内**先按高度删掉旧的 `events`/`transactions` 再插入，
  重放与 reorg 重扫都幂等；del seq 只写 marker + 推进游标（旧行由随后同高度的 add 清掉）。
- 余额表 `token_balances` 不做反向运算：默认扫块时内联查链上 `balanceOf` 写绝对值（自愈）；
  若开启 `skipInlineBalanceUpdate`，则由 `[erc20scan.balanceRefresher]`（链上 `balanceOf` 定时校准）兜底。
- 已知边界：del seq 后该高度若**没有重新出块**（链缩短），旧行不会被自动清掉；用 `scannerfix` 区间重扫
  或人工清理。node 模式（不走 chain33 seq 的 eth JSON-RPC 模式）没有上述 reorg 保护。
