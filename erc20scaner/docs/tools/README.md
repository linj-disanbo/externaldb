# 离线工具（`cmd/tools/erc20-*`）

不在部署包里常驻，用于排障 / 对账 / 补扫。构建（仓库根目录）：

```bash
go build -o build/erc20-txinfo     ./cmd/tools/erc20-txinfo
go build -o build/erc20-blockparse ./cmd/tools/erc20-blockparse
go build -o build/erc20-scannerfix ./cmd/tools/erc20-scannerfix
```

| 工具 | 用途 | 主要参数 |
| --- | --- | --- |
| `erc20-txinfo` | 按 tx hash 从 eth JSON-RPC 拉「交易 + 回执」，按 scanner 的处理口径打印 | `-rpc <url>`（必填）、`-json`、位置参数 `<tx hash>` |
| `erc20-blockparse` | 解析整块交易（可再用 chain33 gRPC 取同一高度做第二遍对齐） | `-rpc <url>`、`-height <n>` 或位置参数 `<n>`、`-seq-tx-count <n>`、`-chain33-grpc <host:port>`、`-json` |
| `erc20-scannerfix` | 按 seq 区间补扫历史（写 MySQL，等价于带区间的 scanner） | `-s <seq>` `-e <seq>`（必填）、`-c <toml/yaml>`、`-db`、`-dsn`、`-u`、`-chain-grpc` |

- txinfo：见 [txinfo.md](./txinfo.md)（输出内容、样本交易）。
- scannerfix 的 `-c` 支持 `externaldb.toml`（读 `[mysql]`/`[erc20scan]`）或老的 yaml。
- 配置里 `[mysql].enabled=false` 时 scannerfix 只解析不落库。
