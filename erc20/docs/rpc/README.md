# HTTP API 服务使用说明

## 概述

这是一个HTTP API服务，提供查询ERC20代币合约相关数据的接口。服务从MySQL数据库中读取数据并返回JSON格式的响应。

**路径说明**：下文部分示例仍使用历史前缀 `/api/...`；当前实现统一为 **`/evmapi/...`**，完整列表见 [`API_LIST.md`](API_LIST.md)。其中 **`GET /evmapi/accounts/{address}/erc20-balances`** 按用户钱包地址返回其持有的 ERC20 列表与余额（分页，可选查询参数 `min_balance`）。

## 启动服务

### 1. 编译程序

```bash
cd rpc
go build -o rpc-server main.go
```

### 2. 运行服务

```bash
# 使用默认配置
./rpc-server

# 指定端口和数据库连接
./rpc-server -port 8080 -dsn "root:password@tcp(localhost:3306)/token_scanner?charset=utf8mb4&parseTime=True&loc=Local"

# 使用配置文件
./rpc-server -c ../config.yaml -port 8080
```

### 3. 参数说明

- `-port`: HTTP服务端口（默认: 8080）
- `-dsn`: 数据库连接字符串
- `-c`: 配置文件路径

## API接口

### 1. 健康检查

**GET** `/health`

检查服务是否正常运行。

**响应示例：**
```json
{
  "code": 0,
  "message": "OK",
  "data": {
    "status": "healthy"
  }
}
```

### 2. 查询合约详情

**GET** `/api/contract/{address}`

查询指定合约的详细信息。

**路径参数：**
- `address`: 合约地址（0x开头，42字符）

**响应示例：**
```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "address": "0x...",
    "name": "Tether USD",
    "symbol": "USDT",
    "type": "ERC20",
    "decimals": 6,
    "total_supply": "1000000000000",
    "total_supply_formatted": "1000000.0",
    "deploy_tx_hash": "0x...",
    "deploy_block_number": 12345678,
    "deploy_time": "2024-01-01T00:00:00Z",
    "deployer": "0x...",
    "verification_status": 1
  }
}
```

### 3. 查询合约列表

**GET** `/api/contracts?page=1&size=20&symbol=USDT`

查询合约列表，支持分页和筛选。

**查询参数：**
- `page`: 页码（默认: 1）
- `size`: 每页数量（默认: 20，最大: 100）
- `symbol`: 代币符号筛选（可选，支持模糊匹配）

**响应示例：**
```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "contracts": [
      {
        "address": "0x...",
        "name": "Tether USD",
        "symbol": "USDT",
        "type": "ERC20",
        "decimals": 6,
        "total_supply": "1000000000000",
        "total_supply_formatted": "1000000.0",
        "deploy_time": "2024-01-01T00:00:00Z"
      }
    ],
    "page": 1,
    "size": 20,
    "total": 1
  }
}
```

### 4. 查询ERC20代币详情

**GET** `/api/token/{address}`

查询指定ERC20代币的详细信息。此接口专门用于查询ERC20类型的代币，如果地址不是ERC20代币，将返回错误。

**路径参数：**
- `address`: ERC20代币地址（0x开头，42字符）

**响应示例：**
```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "address": "0x...",
    "name": "Tether USD",
    "symbol": "USDT",
    "type": "ERC20",
    "decimals": 6,
    "total_supply": "1000000000000",
    "total_supply_formatted": "1000000.0",
    "deploy_tx_hash": "0x...",
    "deploy_block_number": 12345678,
    "deploy_time": "2024-01-01T00:00:00Z",
    "deployer": "0x...",
    "verification_status": 1
  }
}
```

**错误响应示例：**
```json
{
  "code": 400,
  "message": "Address is not an ERC20 token, contract type: ERC721"
}
```

**使用示例：**
```bash
# 查询ERC20代币详情
curl "http://localhost:8080/api/token/0x..."

# 与查询合约详情的区别：
# /api/contract/{address} - 查询任何类型的合约（ERC20, ERC721等）
# /api/token/{address}    - 只查询ERC20类型的代币，如果不是ERC20会返回错误
```

### 5. 查询ERC20 Token列表

**GET** `/api/tokens?page=1&size=20&symbol=USDT&name=Token`

查询ERC20代币列表，专门用于查询ERC20类型的代币，支持分页和筛选。

**查询参数：**
- `page`: 页码（默认: 1）
- `size`: 每页数量（默认: 20，最大: 100）
- `symbol`: 代币符号筛选（可选，支持模糊匹配）
- `name`: 代币名称筛选（可选，支持模糊匹配）

**响应示例：**
```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "tokens": [
      {
        "address": "0x...",
        "name": "Tether USD",
        "symbol": "USDT",
        "type": "ERC20",
        "decimals": 6,
        "total_supply": "1000000000000",
        "total_supply_formatted": "1000000.0",
        "deploy_time": "2024-01-01T00:00:00Z"
      }
    ],
    "page": 1,
    "size": 20,
    "total": 1
  }
}
```

**使用示例：**
```bash
# 查询所有ERC20 token（第一页，每页20条）
curl "http://localhost:8080/api/tokens?page=1&size=20"

# 按符号筛选
curl "http://localhost:8080/api/tokens?symbol=USDT&page=1&size=20"

# 按名称筛选
curl "http://localhost:8080/api/tokens?name=Tether&page=1&size=20"

# 同时按符号和名称筛选
curl "http://localhost:8080/api/tokens?symbol=USDT&name=Tether&page=1&size=20"
```

**说明：**
- 此接口专门用于查询ERC20类型的代币（`contract_type = 'ERC20'`）
- 与 `/api/contracts` 接口的区别：此接口只返回ERC20类型的代币，并且支持按名称筛选
- 返回的 `total` 字段表示符合条件的总记录数，可用于分页计算

### 6. 查询Transfer记录

**GET** `/api/transfers/{address}?page=1&size=20&from=0x...&to=0x...`

查询指定合约的Transfer事件记录。

**路径参数：**
- `address`: 合约地址

**查询参数：**
- `page`: 页码（默认: 1）
- `size`: 每页数量（默认: 20，最大: 100）
- `from`: 发送者地址筛选（可选）
- `to`: 接收者地址筛选（可选）

**响应示例：**
```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "transfers": [
      {
        "tx_hash": "0x...",
        "block_number": 12345679,
        "block_time": "2024-01-01T01:00:00Z",
        "from": "0x...",
        "to": "0x...",
        "value": "1000000",
        "value_formatted": "1.0",
        "token_symbol": "USDT",
        "token_decimals": 6
      }
    ],
    "page": 1,
    "size": 20
  }
}
```

### 7. 通用交易列表查询（新增）

**GET** `/evmapi/transactions?contract=0x..&from=0x..&start_block=N&end_block=N&page=1&page_size=20`

查询所有 EVM 交易的通用接口，按 `block_number DESC` 排序。与原有合约交易查询不同，此接口不要求合约已在 `contracts` 表中注册。

**查询参数：**
- `contract`: 合约地址（可选），匹配 `to_address`
- `from`: 发起地址（可选），匹配 `from_address`
- `start_block`: 起始区块号（可选，含）
- `end_block`: 结束区块号（可选，含）
- `page`: 页码（默认: 1）
- `page_size`: 每页数量（默认: 20，最大: 100）

> **注意**：`contract` 和 `from` 至少传一个。

**响应示例：**
```json
{
  "code": 0,
  "message": "Success",
  "data": {
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
        "gas_limit": 100000,
        "gas_used": 52100,
        "gas_price": "20000000000",
        "tx_fee": "1042000000000000",
        "status": 1
      }
    ],
    "page": 1,
    "page_size": 20,
    "total": 1234
  }
}
```

### 8. 查询合约交易记录（已有）

**GET** `/api/transactions/{address}?page=1&size=20&func_name=transfer`

查询指定合约的交易记录。

**路径参数：**
- `address`: 合约地址

**查询参数：**
- `page`: 页码（默认: 1）
- `size`: 每页数量（默认: 20，最大: 100）
- `func_name`: 函数名称筛选（可选，如: transfer, transferFrom, approve）

**响应示例：**
```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "transactions": [
      {
        "tx_hash": "0x...",
        "block_number": 12345679,
        "block_time": "2024-01-01T01:00:00Z",
        "from": "0x...",
        "to": "0x...",
        "func_name": "transfer",
        "value": "1000000",
        "value_formatted": "1.0",
        "gas_used": 21000,
        "status": 1,
        "token_symbol": "USDT",
        "token_decimals": 6
      }
    ],
    "page": 1,
    "size": 20
  }
}
```

### 9. 查询合约中指定地址的相关交易

**GET** `/api/contract/{contractAddress}/address/{address}/transactions?page=1&size=20&role=from|to|both&func_name=transfer`

查询某个合约中指定地址的相关交易。可以指定查询该地址作为发送者（from）、接收者（to）或两者都查询（both）。返回格式与"查询交易记录"接口相同，只列出所有相关的交易，不进行聚合。

**路径参数：**
- `contractAddress`: 合约地址
- `address`: 要查询的地址

**查询参数：**
- `page`: 页码（默认: 1）
- `size`: 每页数量（默认: 20，最大: 100）
- `role`: 地址角色筛选（可选）
  - `from`: 只查询该地址作为发送者的交易
  - `to`: 只查询该地址作为接收者的交易
  - `both`: 查询该地址作为发送者或接收者的交易（默认值，不指定时也是both）
- `func_name`: 函数名称筛选（可选，如: transfer, transferFrom, approve）

**响应示例：**
```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "transactions": [
      {
        "tx_hash": "0x...",
        "block_number": 12345679,
        "block_time": "2024-01-01T01:00:00Z",
        "from": "0x...",
        "to": "0x...",
        "func_name": "transfer",
        "value": "1000000",
        "value_formatted": "1.0",
        "gas_used": 21000,
        "status": 1,
        "token_symbol": "USDT",
        "token_decimals": 6
      }
    ],
    "page": 1,
    "size": 20
  }
}
```

**使用示例：**
```bash
# 查询某个合约中指定地址的所有相关交易（from和to都查询）
curl "http://localhost:8080/api/contract/0x.../address/0x.../transactions?page=1&size=20"

# 只查询该地址作为发送者的交易
curl "http://localhost:8080/api/contract/0x.../address/0x.../transactions?role=from&page=1&size=20"

# 只查询该地址作为接收者的交易
curl "http://localhost:8080/api/contract/0x.../address/0x.../transactions?role=to&page=1&size=20"

# 查询指定函数名称的交易
curl "http://localhost:8080/api/contract/0x.../address/0x.../transactions?func_name=transfer&role=both"
```

### 9. 查询合约中指定地址的Transfer事件记录

**GET** `/api/contract/{contractAddress}/address/{address}/transfers?page=1&size=20&role=from|to|both`

查询某个合约中指定地址的Transfer事件记录。可以指定查询该地址作为发送者（from）、接收者（to）或两者都查询（both）。返回格式与"查询Transfer记录"接口相同。

**路径参数：**
- `contractAddress`: 合约地址
- `address`: 要查询的地址

**查询参数：**
- `page`: 页码（默认: 1）
- `size`: 每页数量（默认: 20，最大: 100）
- `role`: 地址角色筛选（可选）
  - `from`: 只查询该地址作为发送者的Transfer事件
  - `to`: 只查询该地址作为接收者的Transfer事件
  - `both`: 查询该地址作为发送者或接收者的Transfer事件（默认值，不指定时也是both）

**响应示例：**
```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "transfers": [
      {
        "tx_hash": "0x...",
        "block_number": 12345679,
        "block_time": "2024-01-01T01:00:00Z",
        "from": "0x...",
        "to": "0x...",
        "value": "1000000",
        "value_formatted": "1.0",
        "token_symbol": "USDT",
        "token_decimals": 6
      }
    ],
    "page": 1,
    "size": 20
  }
}
```

**使用示例：**
```bash
# 查询某个合约中指定地址的所有Transfer事件（from和to都查询）
curl "http://localhost:8080/api/contract/0x.../address/0x.../transfers?page=1&size=20"

# 只查询该地址作为发送者的Transfer事件
curl "http://localhost:8080/api/contract/0x.../address/0x.../transfers?role=from&page=1&size=20"

# 只查询该地址作为接收者的Transfer事件
curl "http://localhost:8080/api/contract/0x.../address/0x.../transfers?role=to&page=1&size=20"
```

### 10. 查询Holder信息

**GET** `/api/holders/{address}?page=1&size=20&min_balance=0`

查询指定合约的Holder（持币者）信息，按余额降序排列。

**路径参数：**
- `address`: 合约地址

**查询参数：**
- `page`: 页码（默认: 1）
- `size`: 每页数量（默认: 20，最大: 100）
- `min_balance`: 最小余额筛选（可选，原始值，不考虑decimals）

**响应示例：**
```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "holders": [
      {
        "address": "0x...",
        "balance": "1000000000",
        "balance_formatted": "1000.0",
        "last_tx_hash": "0x...",
        "last_tx_block": 12345679,
        "last_updated": "2024-01-01T01:00:00Z"
      }
    ],
    "page": 1,
    "size": 20
  }
}
```

## 错误响应格式

所有错误响应都使用统一的格式：

```json
{
  "code": 400,
  "message": "错误描述信息"
}
```

常见错误码：
- `200`: 成功
- `400`: 请求参数错误
- `404`: 资源未找到
- `405`: 方法不允许
- `500`: 服务器内部错误

## 使用示例

### 使用curl

```bash
# 查询合约详情
curl http://localhost:8080/api/contract/0x...

# 查询合约列表
curl "http://localhost:8080/api/contracts?page=1&size=10&symbol=USDT"

# 查询ERC20代币详情
curl "http://localhost:8080/api/token/0x..."

# 查询ERC20 Token列表
curl "http://localhost:8080/api/tokens?page=1&size=20&symbol=USDT&name=Token"

# 查询Transfer记录
curl "http://localhost:8080/api/transfers/0x...?page=1&size=20"

# 查询交易记录
curl "http://localhost:8080/api/transactions/0x...?page=1&size=20&func_name=transfer"

# 查询合约中指定地址的相关交易
curl "http://localhost:8080/api/contract/0x.../address/0x.../transactions?role=both&page=1&size=20"

# 查询合约中指定地址的Transfer事件记录
curl "http://localhost:8080/api/contract/0x.../address/0x.../transfers?role=both&page=1&size=20"

# 查询Holder信息
curl "http://localhost:8080/api/holders/0x...?page=1&size=20&min_balance=1000000"
```

### 使用JavaScript (fetch)

```javascript
// 查询合约详情
const response = await fetch('http://localhost:8080/api/contract/0x...');
const data = await response.json();
console.log(data);

// 查询合约列表
const response2 = await fetch('http://localhost:8080/api/contracts?page=1&size=20');
const data2 = await response2.json();
console.log(data2);

// 查询ERC20代币详情
const response3 = await fetch('http://localhost:8080/api/token/0x...');
const data3 = await response3.json();
console.log(data3);

// 查询ERC20 Token列表
const response4 = await fetch('http://localhost:8080/api/tokens?page=1&size=20&symbol=USDT');
const data4 = await response4.json();
console.log(data4);
```

## 注意事项

1. **地址格式**：所有地址必须是有效的以太坊地址格式（0x开头，42字符）
2. **分页限制**：每页最大数量为100条
3. **数据格式**：
   - `total_supply` 和 `value` 字段返回原始值（字符串格式的大整数）
   - `total_supply_formatted` 和 `value_formatted` 字段返回格式化后的值（考虑decimals）
4. **性能考虑**：大量数据查询时建议使用分页，避免一次性查询过多数据
5. **数据库连接**：确保数据库服务正常运行，且连接字符串正确

## 部署建议

1. **使用反向代理**：建议使用Nginx等反向代理服务器
2. **启用HTTPS**：生产环境建议使用HTTPS
3. **添加认证**：可以根据需要添加API密钥认证
4. **监控和日志**：建议添加监控和日志记录
5. **数据库优化**：确保数据库索引正确，优化查询性能

