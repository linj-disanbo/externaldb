# 数据表、功能与 RPC 对应关系

本文整理 ERC20 扫块（`cmd/erc20scan`）写入的 MySQL 表结构、写入逻辑，以及 `erc20api`（`cmd/erc20api`，原 `rpc-server`）暴露的 HTTP 接口与表的读关系，便于联调与扩展。

**约定**：表结构定义见 [`database/schema.sql`](../../database/schema.sql)；RPC 入口见 [`cmd/erc20api/main.go`](../../../cmd/erc20api/main.go)。

---

## 一、数据表总览

| 表名 | 用途 |
|------|------|
| `function_signatures` | 常见函数选择器字典（schema 初始化插入；扫描器/RPC 当前不依赖其做业务查询） |
| `contracts` | 代币/合约元数据：地址、名称、符号、类型（ERC20 等）、decimals、总供应、部署信息等 |
| `transactions` | 与合约交互的**交易级**记录（调用目标合约、函数、input 等） |
| `events` | 链上**日志事件**（以 ERC20 `Transfer` 为主：from/to/value 等） |
| `token_balances` | **地址 × 合约** 维度下的代币余额快照及最后关联交易信息 |
| `scan_progress` | 扫描器断点（旧）：最后处理区块号等，仅排查用（权威进度见 `scan_seq_marker`） |

**视图**（便于展示，RPC 可直接查表或视图）：

| 视图 | 内容 |
|------|------|
| `v_token_balances` | `token_balances` JOIN `contracts`，带格式化余额 |
| `v_transaction_details` | `transactions` JOIN `contracts` |
| `v_event_details` | `events` JOIN `contracts` |

当前 RPC 实现多数使用**直接 SQL 查基表** + `JOIN contracts`，与视图语义等价。

---

## 二、扫描器（scanner）与表：谁写入什么

| 能力 | 主要表 | 说明 |
|------|--------|------|
| 扫块解析 ERC20 相关交易/日志 | `contracts` | 发现代币合约时 upsert 元数据 |
| 同上 | `transactions` | 每笔相关交易一条（与合约调用关联） |
| 同上 | `events` | 每个 `Transfer` 日志一条（`event_name='Transfer'`） |
| 同上 | `token_balances` | 内联 `balanceOf` 写余额；或 `skip_inline_balance_update` 时仅 touch 占位行 |
| 后台任务 | `token_balances` | `balance_refresher` 仅更新 `balance` + `last_updated_at`（见 [`balance_refresher.md`](balance_refresher.md)） |
| 进度 | `scan_seq_marker`（新）/ `scan_progress`（旧，保留排查） | 每个 seq 一行（游标=`max(seq)`，del 也写）；旧表仍存最后高度 |

**ES 模式（`StartWithEsClient`）**：ES 文档 id 为 **sync_seq**，与链 **block height** 一般相差常数 `d = sync_seq - height`（回滚后 seq 仍会增加而高度回退）。启动时用探测结果把下一 ES 键对齐为 `(last_height+1)+d`，循环内用 **SyncSeq+1** 作为下一游标；若当前文档高度与期望不一致且 `|d|≥100` 则按 `want_height+d` 重新对齐后再拉取，`|d|<100` 时可直接处理当前块以快速追上。实现见 `scanner/process.go`（`alignESScanStart`、`esSeqHeightSmallDrift`）。

**未由扫描器业务路径持续写入的表**：`function_signatures`（以 schema 种子数据为主）。

---

## 三、RPC 路由与表的读关系

基础路径：`http://{host}:{port}`，业务 API 前缀为 **`/evmapi`**。

### 1. `/evmapi/contracts`（[`rpc/contract.go`](../../rpc/contract.go)）

| 方法 / 路径 | 主要读取的表 | 说明 |
|-------------|--------------|------|
| `GET /evmapi/contracts` | `contracts` | 合约列表（可按 `symbol` 过滤），含多类型合约 |
| `GET /evmapi/contracts/{address}` | `contracts` | 单合约详情（`GetContractByAddress`） |
| `GET /evmapi/contracts/{address}/transfers` | `events` + `contracts` | 该合约下 **Transfer 事件**（`event_name='Transfer'`） |
| `GET /evmapi/contracts/{address}/transactions` | `transactions` + `contracts` | 该合约相关**交易**（非日志维度） |
| `GET /evmapi/contracts/{address}/holders` | `token_balances` + `contracts` | 持有者列表（按 `balance` 等），可 `min_balance` |
| `GET .../holders/{holder}/transfers` | `events` + `contracts` | 某 holder 在该合约上的 Transfer |
| `GET .../holders/{holder}/transactions` | `transactions` + `contracts` | 某 holder 在该合约上的交易 |

**与 `/evmapi/tokens` 的差异**：`contracts` 列表接口**不过滤** `contract_type`；`/evmapi/tokens` 仅 **ERC20**（见下）。

### 2. `/evmapi/tokens`（[`rpc/token.go`](../../rpc/token.go)）

| 方法 / 路径 | 主要读取的表 | 说明 |
|-------------|--------------|------|
| `GET /evmapi/tokens` | `contracts` | **仅 `contract_type = 'ERC20'`** 的代币列表 |
| `GET /evmapi/tokens/{address}` | `contracts` | 同上，单代币详情并校验 ERC20 |
| `GET /evmapi/tokens/{address}/transfers` | `events` + `contracts` | 该代币合约的 Transfer 事件（与 `contracts/.../transfers` 数据源一致，筛选条件均为 ERC20 合约） |

### 3. `/evmapi/accounts`（[`rpc/accounts.go`](../../rpc/accounts.go)）

| 方法 / 路径 | 主要读取的表 | 说明 |
|-------------|--------------|------|
| `GET /evmapi/accounts/{address}/erc20-balances` | `token_balances` + `contracts` | 某地址持有的 ERC20 及数量；仅 `contract_type='ERC20'`；支持 `page`/`size`/`min_balance` |

### 4. `/evmapi/transactions`（[`rpc/transaction.go`](../../rpc/transaction.go) + `tx_parse`）

| 方法 / 路径 | 数据库 | 说明 |
|-------------|--------|------|
| `GET /evmapi/transactions/{tx_hash}` | — | 当前实现：**未实现**（501） |
| `GET /evmapi/transactions/{tx_hash}/analysis` | **不读 MySQL** | 经 Chain33 gRPC 取交易 +（可选）ES ABI，内存解析 EVM 行为 |

### 5. 其它

| 路径 | 数据库 |
|------|--------|
| `GET /health` | 不访问业务表（健康检查） |

---

## 四、概念对照：Transfer「列表」两种接口

| 维度 | `.../transfers` | `.../transactions` |
|------|-------------------|----------------------|
| 数据源 | `events`（事件日志） | `transactions`（交易记录） |
| 典型用途 | 代币转账流水、按 from/to 筛 | 合约调用流水、按 `func_name` 筛 |
| 扫描器写入 | `saveEventToDB` | `saveTransactionToDB` |

浏览器若展示「链上转账」，一般用 **events**；若展示「调了合约哪条函数」，用 **transactions**。

---

## 五、快速索引：表 → 谁读谁写

| 表 | 扫描器写 | RPC 读 |
|----|----------|--------|
| `contracts` | 是 | 是（tokens/contracts 全系接口） |
| `transactions` | 是 | 是（`* /transactions`） |
| `events` | 是 | 是（`* /transfers`、holder transfers） |
| `token_balances` | 是 | 是（holders、`/evmapi/accounts/.../erc20-balances`） |
| `scan_progress` | 是 | 否（当前无 API） |
| `function_signatures` | 否（种子） | 否（当前无 API） |

---

## 六、相关文档

- [地址代币余额后台刷新（balance_refresher）](balance_refresher.md)
