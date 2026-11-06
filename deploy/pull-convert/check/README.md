# 部署自检脚本（check）

部署完之后，从**运行状态、数据、接口**三个层面确认这套 pull 链路是不是真的在正常跑。
只读（MySQL 只有 `SELECT`/`SHOW`），不改数据、不碰索引、不停服务，随时可以跑。

```
check/
├── check.sh    # 全部检查
└── README.md   # 本文件
```

## 快速开始

在包根目录（有 `.env`、`etc/`、`bin/` 的那一层）：

```bash
make check-deploy          # 等价于 bash check/check.sh all
```

或者直接跑脚本：

```bash
bash check/check.sh                 # 全部
bash check/check.sh progress        # 只看 convert + scanner 进度
bash check/check.sh erc20api jrpc   # 只看接口
bash check/check.sh docker es       # 只看容器和 ES
bash check/check.sh data logs       # 只看数据健康与日志信号
```

退出码：**0 = 没有 FAIL**（WARN 不算失败）；**1 = 有 FAIL**；**2 = 前置条件不满足**（如 `.env` 缺失、没有 python3）。

## 检查项

| 段落 | 检查内容 | 判定 |
|---|---|---|
| `docker` | `pull_convert` / `jrpc` / `scanner` / `erc20api` / `accountrefresh` 容器是否在跑（按 `.env` 开关决定哪些必须） | 容器不存在或非 `Up` → FAIL |
| `runtime` | 容器 `RestartCount`/`ExitCode`/`StartedAt`（发现 crash-loop、启动即退出）；`ES_DIR`/`MYSQL_DIR`/包目录的磁盘使用率 | 非 running → FAIL；重启 >5 次、磁盘 ≥85% → WARN；≥95% → FAIL |
| `es` | 集群健康与未分配分片；`db01_` 前缀下 `seq_marker`/`last_seq`/`contract`/`transaction`/`account`/`block_info` 是否存在；`transaction` 文档数；**write 线程池拒绝数**；**堆使用率**；**数据盘水位** | 索引 404/文档数 0 → FAIL；429 拒绝、堆 ≥85%、盘 ≥85% → WARN；red/盘 ≥95% → FAIL |
| `progress` convert | `seq_marker` 的最大 `seq`/`height`、文档数；`last_seq` 的 convert 进度；**间隔 8 秒再采一次看是否推进**；与链高度算落后块数 | 读不到 → FAIL；不推进 → WARN；落后超阈值 → WARN/FAIL |
| `progress` scanner | `scan_progress` 的高度与已处理交易数、`scan_seq_marker` 最大 seq/height、**两次采样是否推进**、落后块数、`token_balances` 行数 | 查不到 → FAIL；不推进/落后 → WARN |
| `data` | 数据新鲜度（`MAX(created_at)`）、**回归信号**：近 1 小时新增 UNKNOWN 合约数、最近 2000 块主币转账 `value IS NULL` 数、`token_balances.last_tx_hash IS NULL` 数；`transactions` 行数是否增长；InnoDB 行锁等待增量 | 停写 → FAIL；出现上述回归信号 → WARN |
| `erc20api` | `/health`；`/evmapi/tokens`、`/evmapi/contracts` 的 `code=0` + `data.total>0` + 首条 `symbol`/`decimals`；`/evmapi/accounts/{余额最大地址}/erc20-balances` 与 `/approvals` | 非 200/`code!=0`/空数据 → FAIL/WARN |
| `jrpc` | `Block.Count`、`Block.BlockList`（最新 height）、`Account.TopAssetCount`、`Account.TopAsset`；与 convert 进度高度**交叉校验** | 有 `error` 或结果为空 → FAIL/WARN；高度不一致 → WARN |
| `logs` | `bin/logs/*.log` 最近 N 行的 `ERROR`/`panic`/`SIGSEGV` 计数；已知故障关键字（`saving as UNKNOWN`、`invalid connection`、`circuit_breaking`、`Lock wait timeout`、`unexpected EOF`、`empty return data`） | panic → FAIL；ERROR/关键字 → WARN |

> 进度检查特意做“两次采样”：一次采样只能说明“有数据”，两次才能说明“还在动”。
> 链本身没新块时“不推进”是正常的，所以这类是 WARN 不是 FAIL。

## 配置

**默认读包根目录的 `.env`**（就是 `quickStart`/`make` 用的那一份），不需要额外配置。
下面这些可以用环境变量临时覆盖（`.env` 里没有的键才会生效，避免和部署配置打架）：

| 变量 | 默认 | 说明 |
|---|---|---|
| `ENV_FILE` | `../.env` | 换一份 .env（例如测试环境） |
| `ES_URL` | `http://${ES_BIND}:${ES_PORT_HTTP}` | ES 地址（含账号时用 `ES_USER`/`ES_PASSWORD`） |
| `CHAIN_URL` | `${CHAIN_HOST}` | 节点 HTTP jsonrpc，用来算落后块数；空则跳过落后判定 |
| `ERC20_API_URL` | `http://127.0.0.1:${ERC20_API_PORT}` | erc20api |
| `JRPC_URL` | `http://127.0.0.1:<JRPC_HOST 的端口>` | jrpc |
| `CHECK_MYSQL_MODE` | `auto` | `auto`（优先本地 mysql 容器，其次本机 mysql 客户端）/ `docker` / `cli` / `skip` |
| `MAX_LAG_BLOCKS` | `100` | 落后链尖超过它算 WARN，超过 10 倍算 FAIL |
| `PROGRESS_WAIT` | `8` | 两次进度采样的间隔秒数 |
| `CHECK_TIMEOUT` | `10` | 单次 HTTP 超时 |
| `LOG_LINES` | `2000` | 每个日志文件只看最后多少行 |
| `DISK_WARN` / `DISK_FAIL` | `85` / `95` | 磁盘使用率阈值（%） |

MySQL 连接信息的来源：`.env` 里的 `MYSQL_ENABLED` / `MYSQL_DSN` / `MYSQL_NAME` /
`MYSQL_BIND` / `MYSQL_PORT` / `MYSQL_ROOT_PASSWORD` / `MYSQL_DATABASE`。
`MYSQL_ENABLED!=true` 或者无法访问 MySQL 时，scanner 进度自动 `SKIP`（不算失败）。

## 输出示例

```
=== externaldb（pull 链路）部署自检 ===
包目录 : /data/externaldb-pull-1022
ES     : http://127.0.0.1:9200（前缀 db01_）
链节点 : http://127.0.0.1:8801

[docker] 容器状态（项目名 expull）
  [PASS] expull-pull-convert  Up 3 hours
  ...

[progress] convert（pull_convert）进度
  [PASS] seq_marker: max seq=48220612 height=48220612（文档数 1）
  [PASS] 8s 内推进：seq 48220600→48220612，height 48220600→48220612
  [PASS] 落后链尖 3 块（链高 48220615，阈值 100）

[erc20api] 接口验证（http://127.0.0.1:8080）
  [PASS] /evmapi/tokens 共 352 个代币，示例 WBTY(decimals=18)
  ...

=== 汇总：PASS 28  WARN 0  FAIL 0  SKIP 0 ===
结果：通过
```

## 常见问题

| 现象 | 原因 / 处理 |
|---|---|
| `连不上 ES` | `ES_URL`/账号错，或 ES 容器没起（`make initES`） |
| `索引 db01_xxx 不存在` | convert 还没写过；确认 `PROJECT_NAME`/`CONVERT_PREFIX` 与 `etc/externaldb.toml` 的 `[convertEs].prefix` 一致 |
| `8s 内没有推进` | 先看链上是否真没新块；再看对应容器日志（`make log-pull-convert` / `make log-scanner`） |
| `落后链尖 N 块` 且持续变大 | 节点地址错、容器被限流（ES 429）、或 `scan_seq_marker` 卡住 |
| `/evmapi/tokens` 成功但为空 | `contracts` 表里还没有 ERC20（scanner 还没扫到或判定失败） |
| `SKIP` 掉 scanner/data | `MYSQL_ENABLED!=true`，或本机没有 mysql 客户端/容器；可 `CHECK_MYSQL_MODE=docker` 指定 |
| `近 1 小时新增 UNKNOWN 合约 N 条` | 合约判定又失败（历史事故：eth_call 字段不匹配），看 scanner 日志 `saving as UNKNOWN` |
| `主币转账 value 为 NULL` | scanner 版本过旧（缺 `txValue` 修复）或写库失败 |
| `last_tx_hash 为 NULL` | 老版本 erc20api 会 500；跑两条 UPDATE 或更新 erc20api |
| `日志关键字「circuit_breaking」` | ES 堆/磁盘压力触发 429，看 `runtime`/`es` 段与 pull_convert 日志 |
| 想跳过某段 | `bash check/check.sh erc20api` 只跑指定段落 |

## 依赖

- `bash`、`curl`、`python3`（仅用于解析 JSON，不需要 jq）
- `docker`：只用于容器状态和 `CHECK_MYSQL_MODE=auto/docker`，没有则自动 SKIP
- `mysql` 客户端：`CHECK_MYSQL_MODE=cli` 时需要
