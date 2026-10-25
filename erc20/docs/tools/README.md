# 离线工具（`cmd/tools/erc20-*`）

不在部署包里常驻，用于排障 / 对账 / 补扫。构建（仓库根目录）：

```bash
go build -o build/erc20-txinfo     ./cmd/tools/erc20-txinfo
go build -o build/erc20-blockparse ./cmd/tools/erc20-blockparse
```

| 工具 | 用途 | 主要参数 |
| --- | --- | --- |
| `erc20-txinfo` | 按 tx hash 从 eth JSON-RPC 拉「交易 + 回执」，按 scanner 的处理口径打印 | `-rpc <url>`（必填）、`-json`、位置参数 `<tx hash>` |
| `erc20-blockparse` | 解析整块交易（可再用 chain33 gRPC 取同一高度做第二遍对齐） | `-rpc <url>`、`-height <n>` 或位置参数 `<n>`、`-seq-tx-count <n>`、`-chain33-grpc <host:port>`、`-json` |
- txinfo：见 [txinfo.md](./txinfo.md)（输出内容、样本交易）。

## 区间补扫（不进 `cmd/tools/`，直接用 scanner 的修复模式）

补扫历史区间不需要单独的工具，`erc20scan` 自带修复模式（原 `erc20-scannerfix` 已并入）：

```bash
# chain33 模式（[chain].grpcHost 非空）：-s/-e 是 seq
./erc20scan -mode fix -s 1000000 -e 1000500 -c externaldb.toml

# eth JSON-RPC 节点模式：[chain].grpcHost 为空时，-s/-e 是区块高度
./erc20scan -mode fix -s 42399544 -e 42400000 -c externaldb.toml -u https://.../eth
```

- `-s`/`-e` **必须显式给出**（闭区间，含 `-e`），不走配置里的 `start_block`/`end_block`。
- fix 模式**不读/写** `scan_progress` 与 `scan_seq_marker`，不影响线上 scanner 的断点续扫；处理完即退出。
- 不启动 `balance_refresher`；`[mysql].enabled=false` 时只解析打印（可当自检用）。
- 启动时预检：`-e` 超出节点已有范围直接报错退出。
- 日志写到 `logs/scannerfix_app.log`（与 `scanner_app.log` 分开）。
