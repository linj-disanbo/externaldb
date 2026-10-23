# 代币扫描器数据库设计文档

## 数据库概述

本数据库设计用于存储代币扫描器收集的所有数据，支持ERC20/ERC721/ERC1155等多种代币标准，包括合约信息、交易记录、地址余额和事件日志。

## 表结构说明

### 0. 函数签名表 (function_signatures)

存储常见的函数签名和对应的函数信息，用于识别和解析合约调用。

**主要字段：**
- `selector`: 函数选择器（4字节hex，主键，唯一索引，如 "a9059cbb"）
- `function_name`: 函数名称（如 "transfer"）
- `function_signature`: 完整函数签名（如 "transfer(address,uint256)"）
- `standard_type`: 标准类型（ERC20/ERC721/ERC1155等）
- `function_type`: 函数类型（view/pure/payable/nonpayable）
- `description`: 函数描述
- `is_standard`: 是否为标准函数

**索引：**
- 主键：`selector`
- 索引：`function_name`, `standard_type`, `function_type`

**预置数据：**
- ERC20标准函数（totalSupply, balanceOf, transfer, transferFrom, approve, allowance等）
- ERC721标准函数（ownerOf, safeTransferFrom等）
- ERC1155标准函数（balanceOf, safeTransferFrom, safeBatchTransferFrom等）

### 1. 合约表 (contracts)

存储所有检测到的代币合约的基本信息（支持ERC20/ERC721/ERC1155等）。

**主要字段：**
- `contract_address`: 合约地址（主键，唯一索引）
- `contract_name`: 代币名称（如 "Tether USD"）
- `contract_symbol`: 代币符号（如 "USDT"）
- `contract_type`: 合约类型（ERC20/ERC721/ERC1155/UNKNOWN等）
- `decimals`: 小数位数（ERC20使用，通常为18）
- `total_supply`: 总供应量（原始值，不考虑decimals）
- `deploy_tx_hash`: 部署交易哈希
- `deploy_block_number`: 部署区块号
- `deploy_block_time`: 部署时间
- `deployer_address`: 部署者地址
- `verification_status`: 验证状态（0-未验证, 1-已验证, 2-验证失败）
- `verified_functions`: 已验证的函数选择器列表（JSON格式）

**索引：**
- 主键：`contract_address`
- 索引：`deploy_block_number`, `deploy_block_time`, `contract_symbol`, `contract_type`, `verification_status`

### 2. 交易表 (transactions)

存储所有与ERC20合约相关的交易记录。

**主要字段：**
- `tx_hash`: 交易哈希（主键，唯一索引）
- `block_number`: 区块号
- `block_time`: 区块时间
- `from_address`: 发送者地址
- `to_address`: 接收者地址（通常是合约地址）
- `contract_address`: 合约地址（外键关联contracts表）
- `func_selector`: 函数选择器（4字节hex，外键关联function_signatures表）
- `func_name`: 函数名称（transfer/transferFrom/approve等）
- `value`: 交易金额（原始值）
- `gas_used`: 实际使用的Gas
- `gas_price`: Gas价格
- `tx_fee`: 交易手续费
- `status`: 交易状态（0-失败，1-成功）
- `tx_data`: 交易输入数据

**索引：**
- 主键：`tx_hash`
- 索引：`block_number`, `block_time`, `contract_address`, `from_address`, `to_address`, `func_selector`, `status`
- 外键：`contract_address` -> `contracts.contract_address`, `func_selector` -> `function_signatures.selector`

### 3. 地址持有代币余额表 (token_balances)

存储每个地址持有的代币余额。

**主要字段：**
- `address`: 地址
- `contract_address`: 合约地址（外键关联contracts表）
- `balance`: 余额（原始值，不考虑decimals）
- `balance_formatted`: 格式化后的余额（考虑decimals）
- `last_tx_hash`: 最后一次交易哈希
- `last_tx_block_number`: 最后一次交易区块号

**索引：**
- 主键：`(address, contract_address)` 联合唯一索引
- 索引：`address`, `contract_address`, `balance`, `last_tx_block_number`
- 外键：`contract_address` -> `contracts.contract_address`

**设计说明：**
- 使用联合唯一索引确保每个地址对每个合约只有一条余额记录
- 余额会随着Transfer事件自动更新
- 可以通过定时任务或实时更新来维护余额

### 4. 事件表 (events)

存储所有ERC20相关事件（主要是Transfer和Approval事件）。

**主要字段：**
- `tx_hash`: 交易哈希（外键关联transactions表）
- `block_number`: 区块号
- `log_index`: 日志索引（在交易中的位置）
- `contract_address`: 合约地址（外键关联contracts表）
- `event_name`: 事件名称（Transfer/Approval等）
- `from_address`: From地址（Transfer事件）
- `to_address`: To地址（Transfer事件）
- `value`: 金额（Transfer事件，原始值）
- `owner_address`: Owner地址（Approval事件）
- `spender_address`: Spender地址（Approval事件）
- `amount`: 授权金额（Approval事件）
- `topic0-3`: 事件主题（用于索引和查询）
- `data`: 事件数据

**索引：**
- 主键：`(tx_hash, log_index)` 联合唯一索引
- 索引：`block_number`, `block_time`, `contract_address`, `event_name`, `from_address`, `to_address`, `owner_address`, `spender_address`, `topic0`
- 外键：`tx_hash` -> `transactions.tx_hash`, `contract_address` -> `contracts.contract_address`

## 视图说明

### v_token_balances
代币余额视图，自动格式化余额显示，包含代币名称、符号等信息。

### v_transaction_details
交易详情视图，包含合约信息和格式化后的金额。

### v_event_details
事件详情视图，包含合约信息和格式化后的金额。

## 使用示例

### 查询某个地址的所有代币余额
```sql
SELECT * FROM v_token_balances 
WHERE address = '0x...' 
ORDER BY balance_formatted DESC;
```

### 查询某个合约的所有交易
```sql
SELECT * FROM v_transaction_details 
WHERE contract_address = '0x...' 
ORDER BY block_number DESC 
LIMIT 100;
```

### 查询函数签名信息
```sql
-- 根据选择器查询函数信息
SELECT * FROM function_signatures WHERE selector = 'a9059cbb';

-- 查询ERC20标准的所有函数
SELECT * FROM function_signatures WHERE standard_type = 'ERC20';

-- 查询某个合约类型的所有标准函数
SELECT * FROM function_signatures 
WHERE standard_type = 'ERC721' AND is_standard = TRUE;
```

### 查询特定类型的合约
```sql
-- 查询所有ERC20合约
SELECT * FROM contracts WHERE contract_type = 'ERC20';

-- 查询所有ERC721合约
SELECT * FROM contracts WHERE contract_type = 'ERC721';
```

### 查询某个地址的Transfer事件
```sql
SELECT * FROM v_event_details 
WHERE event_name = 'Transfer' 
AND (from_address = '0x...' OR to_address = '0x...')
ORDER BY block_number DESC;
```

### 查询某个合约的最新余额快照
```sql
SELECT 
    address,
    balance_formatted,
    contract_symbol,
    last_tx_block_number
FROM v_token_balances
WHERE contract_address = '0x...'
AND balance > 0
ORDER BY balance_formatted DESC;
```

### 统计某个合约的交易量
```sql
SELECT 
    DATE(block_time) as date,
    COUNT(*) as tx_count,
    SUM(value_formatted) as total_volume
FROM v_transaction_details
WHERE contract_address = '0x...'
AND func_name = 'transfer'
GROUP BY DATE(block_time)
ORDER BY date DESC;
```

## 数据维护建议

1. **余额更新**：每次处理Transfer事件时，更新相关地址的余额
2. **索引优化**：定期分析表并优化索引
3. **数据归档**：对于历史数据，可以考虑归档到历史表
4. **备份策略**：建议定期备份数据库

## 注意事项

1. **精度问题**：使用DECIMAL类型存储大数值，避免精度丢失
2. **地址格式**：所有地址统一使用小写格式存储（0x开头，42字符）
3. **外键约束**：确保数据完整性，但可能影响插入性能
4. **索引维护**：大量数据插入时，考虑临时禁用部分索引

