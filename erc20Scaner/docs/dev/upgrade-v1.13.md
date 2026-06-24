# v1.13 升级说明

## DB 迁移

```sql
ALTER TABLE transactions ADD INDEX idx_to_block (to_address, block_number);
ALTER TABLE transactions ADD INDEX idx_from_block (from_address, block_number);
```

## 改动

- **数据采集**：所有 EVM 交易均写入 transactions 表（之前仅 ERC20 Transfer 相关）
  - ETH 转账 `func_name=eth_transfer`，合约创建 `func_name=contract_creation`，其他 `func_name=unknown`
  - `func_selector` 为 tx.Data 前 4 字节真实值
- **合约表**：非 ERC20 合约自动占位 `contract_type=UNKNOWN`，保证 FK
- **新 API**：`GET /evmapi/transactions?contract=&from=&start_block=&end_block=&page=&page_size=`
- **兼容**：`/evmapi/contracts` 过滤占位行，`/contracts/{addr}/transactions` 非 ERC20 时 token 字段返回空

## 升级步骤

1. 执行 DB 迁移
2. 部署新版本
3. 历史数据：`scannerfix -s <start> -e <end> -db -dsn "..." -chain-grpc <host>`
