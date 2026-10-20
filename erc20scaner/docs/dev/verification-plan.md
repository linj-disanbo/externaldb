# ETH 通用交易 — 验证计划

> **2026-10 布局变更**：程序已并入主 module —— `cmd/erc20scan`（scanner）、`cmd/erc20api`（原 rpc 服务）、
> `cmd/tools/erc20-*`（离线工具），配置统一读 `externaldb.toml` 的 `[mysql]` / `[erc20scan]`（yaml 仅兼容）。
> 下文出现的 `erc20Scaner/`、`./scanner`、`./rpc`、`-c config.yaml` 属旧布局，按新路径理解。


## 1. 编译验证

```bash
cd erc20Scaner
go build ./database/... ./scanner/... ./cmd/scannerfix/... ./rpc/...
```

## 2. 单元测试

```bash
cd erc20Scaner
go test ./... 2>&1 | grep -v "admin-rpc"
```

覆盖范围：
- `blockalign/align_test.go`
- `txparser/txparser_test.go`
- `scanner/integration_test.go`
- `config/*_test.go`

## 3. scannerfix 历史数据补扫

### 3.1 数据库迁移

```sql
ALTER TABLE transactions ADD INDEX idx_to_block (to_address, block_number);
ALTER TABLE transactions ADD INDEX idx_from_block (from_address, block_number);
```

### 3.2 小范围补扫（50~100 seq）

```bash
./scannerfix -s <start_seq> -e <end_seq> \
  -c config.yaml -db -dsn "user:pass@tcp(host:3306)/db" \
  -chain-grpc <host:port>
```

### 3.3 数据验证

| 验证点 | SQL / 方式 |
|--------|-----------|
| contracts 表出现 UNKNOWN 占位行 | `SELECT * FROM contracts WHERE contract_type = 'UNKNOWN' LIMIT 10;` |
| transactions 出现 eth_transfer | `SELECT * FROM transactions WHERE func_name = 'eth_transfer' LIMIT 10;` |
| transactions 出现 contract_creation | `SELECT * FROM transactions WHERE func_name = 'contract_creation' LIMIT 10;` |
| transactions 出现 unknown | `SELECT * FROM transactions WHERE func_name = 'unknown' LIMIT 10;` |
| func_selector 为真实值（非 nested_call） | `SELECT func_selector, COUNT(*) FROM transactions GROUP BY func_selector;` |
| ERC20 transfer 交易 events 正确 | `SELECT e.* FROM events e JOIN transactions t ON e.tx_hash = t.tx_hash WHERE t.func_name = 'unknown' LIMIT 10;` |
| 无 FK 约束报错 | 检查 scannerfix 日志无 `foreign key constraint fails` 错误 |
| 合约列表不出现 UNKNOWN | `curl "http://localhost:8080/evmapi/contracts?page=1&size=20"` — 无 contract_type=UNKNOWN |

### 3.4 扩大范围

小范围验证通过后，扩大 seq 范围或全量重扫。

## 4. API 查询验证

### 4.1 通用交易查询

```bash
# 按合约地址
curl "http://localhost:8080/evmapi/transactions?contract=0x...&page=1&page_size=20"

# 按发起地址 + 区块范围
curl "http://localhost:8080/evmapi/transactions?from=0x...&start_block=10000000&page=1"

# 缺少参数应返回错误
curl "http://localhost:8080/evmapi/transactions?page=1"
```

### 4.2 合约交易查询（非 ERC20 合约）

```bash
# 非 ERC20 合约应返回 token_symbol="" 
curl "http://localhost:8080/evmapi/contracts/0x...非ERC20.../transactions?page=1&size=20"
```

### 4.3 合约列表

```bash
# 不应出现 UNKNOWN 占位行
curl "http://localhost:8080/evmapi/contracts?page=1&size=20"
```

## 5. 实时扫描验证

```bash
./scanner -c config.yaml -db -dsn "..." -chain-grpc <host:port>
```

关注点：
- `[Init]` 日志：缓存初始化、DB 连接、进度恢复
- `[Round]` 日志：区块处理正常
- ERROR/WARN 日志：不应有异常增长
- Ctrl+C 停止后检查最后处理的区块数据

## 6. 回归验证

| 验证点 | 方式 |
|--------|------|
| ERC20 Transfer 事件记录正常 | 对比补扫前后 events 表数量 |
| ERC20 余额更新正常 | 随机抽查 token_balances 行 |
| 已有 API 响应不变 | 对比 `/evmapi/contracts/{addr}/transfers` 新旧响应 |
