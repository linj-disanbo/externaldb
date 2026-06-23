# RPC API 接口列表

## 健康检查

- `GET /health` → 健康检查

## 合约 (Contracts)

- `GET /evmapi/contracts?page=1&size=20&symbol=USDT` → 查询合约列表 (分页、过滤)
- `GET /evmapi/contracts/{address}` → 查询指定合约的详细信息
- `GET /evmapi/contracts/{address}/transfers?page=1&size=20&from=0x...&to=0x...` → 查询指定合约的所有 Transfer 记录
- `GET /evmapi/contracts/{address}/transactions?page=1&size=20&func_name=transfer` → 查询指定合约的所有交易记录
- `GET /evmapi/contracts/{address}/holders?page=1&size=20&min_balance=0` → 查询指定合约的 Holder 列表

## 代币 (Tokens)

- `GET /evmapi/tokens?page=1&size=20&symbol=USDT&name=Token` → 查询 ERC20 token 列表 (分页、过滤)
- `GET /evmapi/tokens/{address}` → 查询指定 ERC20 代币的详细信息
- `GET /evmapi/tokens/{address}/transfers?page=1&size=20&from=0x...&to=0x...` → 查询指定代币的转账列表

## 账户 (Accounts)

- `GET /evmapi/accounts/{address}/erc20-balances?page=1&size=20&min_balance=0` → 按**用户地址**查询其持有的 **ERC20** 列表及余额（`token_balances` JOIN `contracts`，仅 `contract_type=ERC20`）

## 交易 (Transactions)

- `GET /evmapi/transactions?contract=0x..&from=0x..&start_block=N&end_block=N&page=1&page_size=20` → 通用交易列表查询（按合约地址或发起地址，`contract` 和 `from` 至少传一个）
- `GET /evmapi/transactions/{tx_hash}/analysis` → 解析并获取单笔 EVM 交易的详细动作

## 持有者与合约的交互 (Holder Interactions with a Contract)

- `GET /evmapi/contracts/{contractAddress}/holders/{holderAddress}/transfers?page=1&size=20&role=from|to|both` → 查询特定合约中，特定持有者的转账记录
- `GET /evmapi/contracts/{contractAddress}/holders/{holderAddress}/transactions?page=1&size=20&role=from|to|both&func_name=transfer` → 查询特定合约中，特定持有者的相关交易

## 参数说明

### 通用查询参数

- `page`: 页码，从1开始，默认1
- `size`: 每页数量，范围1-100，默认20

### 地址参数

- `address`: 合约地址或代币地址，0x开头的42字符十六进制地址
- `contractAddress`: 合约地址
- `holderAddress`: 持有者地址
- `accounts` 路径下的 `{address}`: 用户钱包地址（0x + 40 hex）
- `tx_hash`: 交易哈希，0x开头的66字符十六进制字符串

### 转账相关参数

- `role`: 地址角色，可选值：`from`（发送方）、`to`（接收方）、`both`（两者，默认）
- `from`: 发送方地址（可选）
- `to`: 接收方地址（可选）

### 通用交易列表参数

- `contract`: 合约地址（可选），匹配 `to_address`
- `from`: 发起地址（可选），匹配 `from_address`
- `start_block`: 起始区块号（可选）
- `end_block`: 结束区块号（可选）
- `page_size`: 每页数量（可选），范围1-100，默认20
- `contract` 和 `from` 至少传一个

### 交易相关参数

- `func_name`: 函数名称（可选），如 `transfer`

### 持有者相关参数

- `min_balance`: 最小余额（可选），用于筛选持有者

### 列表查询参数

- `symbol`: 代币符号（可选），支持模糊匹配
- `name`: 代币名称（可选），支持模糊匹配
