# ETH 交易通用记录与查询

## 1. 背景与动机

当前 scanner 的 `transactions` 表只记录三类交易：
- **失败/revert 交易**：无论类型，保存基本信息
- **合约创建交易**：仅写入 `contracts` 表，不写 `transactions`
- **产生 ERC20 Transfer 事件的交易**：transfer/transferFrom 直接调用，或 DEX 嵌套调用

以下交易被**静默丢弃**：
- 普通 ETH 转账（`txData` 长度 < 4）
- 不产生 Transfer 事件的合约调用（如单独的 `approve`）
- 任何成功的、不触发 ERC20 Transfer 的 EVM 交易

这意味着无法回答诸如"某个合约地址发生过哪些交易"、"某地址发起了哪些调用"等通用查询。短期无法实现全类型合约解析，但**先提供通用的交易记录和查询能力**是必要的中间步骤。

## 2. 需求

### 2.1 数据采集

| 编号 | 需求 |
|------|------|
| R1 | 记录所有 EVM 格式的交易（无论成功/失败、是否 ERC20 相关） |
| R2 | 记录交易基本字段：hash、区块号/时间、from、to、value、gas、status、input data |
| R3 | 合约创建交易的 `to` 为空，`contract_address` 记录新创建地址（同时写入 contracts 表） |
| R4 | 普通 ETH 转账 `input` 为空，`func_selector` 标记为 `0x` |
| R5 | 合约调用的 `func_selector` 提取 input data 前 4 字节 |
| R6 | 保存交易前确保 `contract_address` 在 `contracts` 表中有对应行（占位写入），保留 FK 约束 |

### 2.2 查询接口

| 编号 | 需求 |
|------|------|
| R7 | 按合约地址查询：`contract`（匹配 `to_address`），与 `from` 至少传一个 |
| R8 | 按发起地址过滤：`from`，可选 |
| R9 | 按区块范围过滤：`start_block` / `end_block`，可选 |
| R10 | 默认排序：`block_number DESC`（最新在前） |
| R11 | 分页：`page` / `page_size`，默认 page=1, page_size=20，最大 100 |
| R12 | 返回字段：tx_hash, block_number, block_time, from, to, contract_address, func_selector, func_name, value, gas_used, gas_price, tx_fee, status（列表省略 tx_data） |

### 2.3 非功能需求

| 编号 | 需求 |
|------|------|
| N1 | 不影响现有 ERC20 扫描逻辑（parseERC20Transfer 保持不变） |
| N2 | 保留 `fk_tx_contract` 外键约束，通过占位写入保证数据完整性 |
| N3 | `contracts` 表 schema 不做结构性修改 |

## 3. 设计

### 3.1 合约占位写入

核心思路：保留 `fk_tx_contract` 外键。在 `saveTransactionToDB` 之前，调用 `EnsureContract(addr)` 确保 `contracts` 表中存在该地址的行。

```
saveTransactionToDB()
      │
      ├─ EnsureContract(contractAddress)     ← 【新增】
      │     │
      │     ├─ GetContractByAddress() → 已存在 → 跳过
      │     └─ 不存在 → INSERT 占位行:
      │           contract_type = "UNKNOWN"
      │           decimals = 0
      │           verification_status = 0
      │           verified_functions = "[]"
      │           其余字段 NULL / 空字符串
      │
      └─ INSERT INTO transactions (...)
```

**`EnsureContract` SQL**（幂等，已存在则不做任何修改）：

```sql
INSERT IGNORE INTO contracts
  (contract_address, contract_name, contract_symbol, contract_type,
   decimals, verification_status, verified_functions)
VALUES (?, 'Unknown', 'UNKNOWN', 'UNKNOWN', 0, 0, '[]')
```

`INSERT IGNORE` + 唯一索引 `uk_contract_address` 保证：已存在的 ERC20 合约不会被覆盖，不存在的合约自动占位。

### 3.2 数据采集流程

```
chain33/ETH 节点
       │
       ▼
 Process.processTransactionWithReceipt()
       │
       ├─ 统一入口：saveTransactionToDB()  ← 【改动】从此处始终调用
       │     └─ EnsureContract()            ← 【新增】FK 占位
       │
       ├─ receipt.Status != 1  ──→  跳过事件解析，直接返回
       │
       ├─ tx.To() == nil       ──→  handleContractCreation()  ← 合约检测 + contracts 表详情
       │
       └─ tx.To() != nil       ──→  parseERC20Transfer()      ← 现有逻辑不变
       │
       ▼
  transactions 表（所有交易，FK 完整）
```

### 3.3 `contracts` 表兼容性分析

| 字段 | 非 ERC20 占位值 | 来源 |
|------|:---:|------|
| `contract_address` | 实际地址 | 交易 `tx.To()` |
| `contract_name` | `"Unknown"` | 占位常量 |
| `contract_symbol` | `"UNKNOWN"` | 占位常量 |
| `contract_type` | `"UNKNOWN"` | 占位常量 |
| `decimals` | `0` | 显式传值（覆盖 DEFAULT 18） |
| `total_supply` | NULL | 不传 |
| `deploy_tx_hash` | NULL | 不传 |
| `deploy_block_number` | NULL | 不传 |
| `deploy_block_time` | NULL | 不传 |
| `deployer_address` | NULL | 不传 |
| `verification_status` | `0` | 未验证 |
| `verified_functions` | `"[]"` | 空数组 |

无需修改表结构。不传值的字段全部支持 NULL。

### 3.4 Schema 变更

仅加查询索引，不修改约束：

```sql
-- 查询索引
ALTER TABLE transactions ADD INDEX idx_to_block (to_address, block_number);
ALTER TABLE transactions ADD INDEX idx_from_block (from_address, block_number);
```

### 3.5 `saveTransactionToDB` 各交易类型参数

| 交易类型 | contract_address | func_selector | func_name |
|---------|:---:|:---:|:---:|
| 普通 ETH 转账 | `tx.To()` | `0x` | `eth_transfer` |
| 合约创建 | `receipt.ContractAddress` | 从 tx.Data 提取 | `contract_creation` |
| 合约调用（有 Transfer） | `tx.To()` | 同现有逻辑 | transfer/transferFrom/nested_call |
| 合约调用（无 Transfer） | `tx.To()` | 从 tx.Data 提取 | `unknown` |

### 3.6 查询 API

```
GET /evmapi/transactions
```

**Query 参数**：

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|:---:|--------|------|
| `contract` | string | 否 | — | 合约地址（匹配 to_address） |
| `from` | string | 否 | — | 发起地址（匹配 from_address） |
| `start_block` | uint64 | 否 | — | 起始区块号（含） |
| `end_block` | uint64 | 否 | — | 结束区块号（含） |
| `page` | int | 否 | 1 | 页码 |
| `page_size` | int | 否 | 20 | 每页条数，最大 100 |

`contract` 和 `from` 至少传一个。

**响应格式**：

```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "total": 1234,
    "page": 1,
    "page_size": 20,
    "transactions": [
      {
        "tx_hash": "0x9a7b...",
        "block_number": 12345678,
        "block_time": "2024-01-15T10:30:00Z",
        "from_address": "0xabcd...",
        "to_address": "0x1234...",
        "contract_address": "0x1234...",
        "func_selector": "a9059cbb",
        "func_name": "transfer",
        "value": "1000000000000000000",
        "gas_used": 52100,
        "gas_price": "20000000000",
        "tx_fee": "1042000000000000",
        "status": 1
      }
    ]
  }
}
```

### 3.7 数据库查询 SQL

```sql
SELECT tx_hash, block_number, block_time, from_address, to_address,
       contract_address, func_selector, func_name, value,
       gas_limit, gas_used, gas_price, tx_fee, status
FROM transactions
WHERE 1=1
  AND (? = '' OR to_address = ?)          -- contract 过滤
  AND (? = '' OR from_address = ?)        -- from 过滤
  AND (? = 0 OR block_number >= ?)        -- start_block
  AND (? = 0 OR block_number <= ?)        -- end_block
ORDER BY block_number DESC
LIMIT ? OFFSET ?
```

## 4. 实施计划

### 阶段 1：DB 层
- [ ] `database.EnsureContract(addr)` — INSERT IGNORE 占位行
- [ ] `database.ListTransactions()` — 通用查询方法
- [ ] 添加 `idx_to_block`、`idx_from_block` 索引

### 阶段 2：数据采集
- [ ] 重构 `processTransactionWithReceipt`：`saveTransactionToDB` 提升为统一出口，调用前 `EnsureContract`
- [ ] 合约创建交易补充写入 `transactions`
- [ ] 普通 ETH 转账写入（`func_selector = "0x"`）
- [ ] 无 Transfer 事件的合约调用写入（`func_name = "unknown"`）

### 阶段 3：查询 API
- [ ] `rpc/transaction.go` `handleListTransactions` handler
- [ ] `rpc/types.go` 请求/响应类型
- [ ] `rpc/main.go` 路由注册 `GET /evmapi/transactions`

### 阶段 4：验证
- [ ] `scannerfix` 模式对历史数据补扫验证
- [ ] 实时扫描新数据验证

## 5. 代码改动范围

```
erc20Scaner/
├── database/models.go              # 新增 EnsureContract()、ListTransactions()
├── scanner/engine/process.go       # processTransactionWithReceipt 重构
├── rpc/
│   ├── main.go                     # 路由注册
│   ├── transaction.go              # handleListTransactions
│   └── types.go                    # 请求/响应类型
└── docs/dev/
    └── eth-transaction-records.md  # 本文档
```
