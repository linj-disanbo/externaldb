# pkg-pullConvert：直连（pull + seq）部署包

用于 **convert 直连（pull + seq 驱动）** 的一键部署，取代老链路 `sync + convert`。
数据流：chain33 按 seq 拉块（`GetBlockSequences` + `GetBlockByHashes`）→ 展开写 ES，进度是 append-only 的 `seq_marker`（游标 = max(seq)）。
切换步骤：停老链路写方（`sync`/`convert`）→ 起 `pull_convert` → 等追平后 `make check`（`convert_check`）对账 → 确认旧 `db05_last_seq` 不再增长。

修复历史区间（补空洞/回放）：`./bin/pull_convert -mode fix -from <起始seq> -to <结束seq> -f etc/externaldb.toml -c etc/chain33.toml`
（闭区间；不写 `seq_marker`、不做 `block_stat` 累计，跑完用 `convert_check -from/-to` 对账）。

## 包内容

```
externaldb-pull/
├── bin/            pull_convert、scanner（=erc20scan）、jrpc、erc20api、account_refresh（常驻程序）
├── tools/          convert_check（离线对账；后续一次性/排障工具也放这里）
├── etc/            externaldb.toml、chain33.toml
├── env/mysql/      包内 MySQL 8.0 的 compose（sql/schema.sql 首次启动自动建表）
├── env/es7/        ES 7.17.29 的 compose + proof-init.bash（默认）
├── env/es7.4/      ES 7.4.2 的 compose + proof-init.bash（备用，见下）
├── docker-compose.yaml        5 个服务：pull_convert / jrpc / scanner / erc20api / accountrefresh
├── Makefile                    内层一键脚本
├── .env                        物料表相关配置
├── alter-config.sh / es-init.sh / browser-init.bash
└── README.md
```

## 使用

1. 改 `.env`：`EX_SERVICE_TYPE`（proof/browser/common）、`ES_VERSION=7`、
   `PROJECT_NAME`、ES 密码/数据目录、链地址与 `CONVERT_PREFIX`/`SYNC_PREFIX` 等。
2. 按物料表改 `etc/externaldb.toml`（节点 gRPC/推送、ES 地址、`[convert].appName`、直连参数 `pullBatch`/`pullPollMs`/`pullDetailCache`/`pullMarkerKeep`）。
3. 一键启动：`make quickStart`；或分开 `make initES` → `make initConfig` → `make run`。
4. 对账：`make check`（= `tools/convert_check`，在宿主机跑），或 `make es-marker` 看游标。
5. 回退：停掉 `convert`，另行用老链路（`sync` → `convert`，不在本包内）从 `seq05_seq` 续跑；
   同一批索引只能有一个写方，切换前务必停干净。

## 5 个程序

| 程序 | 作用 | 依赖 |
| --- | --- | --- |
| `pull_convert` | 按 seq 从 chain33 拉块、展开写 ES（唯一写方） | chain33 gRPC、ES |
| `jrpc` | 浏览器/查询：读 ES | ES（MySQL 可选，`getTransferLogs`） |
| `scanner` | ERC20 扫块：解析转账写 MySQL | chain33 gRPC、MySQL、eth JSON-RPC（balanceOf） |
| `erc20api` | ERC20 查询：`/evmapi/*` 读 MySQL | MySQL |
| `account_refresh` | 主币（BTY）余额定时校正：三档（top 1h / 活跃 24h / 全量 168h）按地址查链上 `balance/frozen` 写回 ES `account` | chain33 gRPC、ES |

- `scanner`/`erc20api` 受 `.env` 的 `MYSQL_ENABLED` 控制：`true` 时 `make quickStart` / `make run` 才会启动这两个服务，
  同时 `alter-config.sh` 把 `[mysql].enabled` 写进配置；默认 `false`（只跑 convert + jrpc）。
- `MYSQL_DSN`、`ERC20_ETH_URL`、`ERC20_START_BLOCK`、`ERC20_API_PORT` 分别在 `.env` 里配置，
  由 `alter-config.sh` 写入 `[mysql]` / `[erc20scan]`。
- 包内 MySQL（可选）：`MYSQL_LOCAL_START=true` 时 `make quickStart` 会先起 `env/mysql` 的 MySQL
  （数据目录 `MYSQL_DIR`、端口 `MYSQL_PORT`、root 密码 `MYSQL_ROOT_PASSWORD`，首次启动自动执行
  `env/mysql/sql/schema.sql`）。生产可不用：把 `MYSQL_DSN` 指向已有实例并保持 `MYSQL_LOCAL_START=false`。
  单独操作用 `make initMySQL` / `make mysql-cli` / `make initMySQLSchema` / `make downMySQL`。
  `MYSQL_DSN` 留空时，若 `MYSQL_LOCAL_START=true` 会按 `MYSQL_BIND/PORT/DATABASE/ROOT_PASSWORD`
  自动拼出 DSN（密码/库名只写一处）；填了 DSN 则以 DSN 为准（外部实例）。
  用包内 MySQL 时记得同时把 `MYSQL_ENABLED=true`（scanner/erc20api 才会启动并连它）。
- `account_refresh` 受 `.env` 的 `ACCOUNT_REFRESH_ENABLED` 控制（默认 true），间隔在 `.env`
  （`ACCOUNT_REFRESH_TOP_INTERVAL`/`ACCOUNT_REFRESH_ACTIVE_INTERVAL`/`ACCOUNT_REFRESH_FULL_INTERVAL`）
  由 `alter-config.sh` 写入 `[accountRefresh]`；程序内部循环，不需要 cron。
- 日志：`pull_convert`/`jrpc` 在 `bin/logs/convert.log`、`jrpc.log`；`scanner`/`erc20api` 在 `bin/logs/scanner_app.log`、`rpc_app.log`
  （`make log-pull-convert`、`make log-scanner`、`make log-erc20api`）。

## 注意

- **ES 堆建议 ≥2g**：ES 的 coordination 断路器上限随堆走，512m 堆时只有 ~51MB，大 seq（genesis）写出大请求会被 429 拒绝
  （代码已按 ≤5MB 分片 + 429 退避重试兜底，但堆太小整体吞吐也差）。改 `env/es7/docker-compose.yaml` 的 `ES_JAVA_OPTS` 后 `make downES && make initES`。
- **同一批索引只能有一个写方**：`pull_convert` 与老 `convert` 不能同时跑（两套游标）。
- `pull_convert` 启动会自动自举游标；首次启动看日志里是 `已按旧 db05_last_seq 自举` 还是 `按配置 startSeq 起跑`——
  后者意味着会**从 startSeq 重跑**，切换前务必确认 `etc/externaldb.toml` 里 `[convert].startSeq` 与现状一致。
- ES 7.17.29（默认）需要宿主机 `vm.max_map_count >= 262144`。
- **要换成 ES 7.4.2**：把 `env/es7.4/` 里的两个文件覆盖到 `env/es7/`（或把目录对调），其它不用动——`ES_VERSION` 保持 7，脚本不区分 7 与 7.4。
- 回滚/reorg 由节点 seq 日志里的 del 事件驱动，不需要额外配置；块体被 prune 的旧块靠 `pullDetailCache`（默认 1000）兜底。
- **改了 `.env` 里的 `ES_NAME` 之后**：ES 的 `container_name` 就是 `${ES_NAME}`，改名属于"配置变更"，`docker-compose up -d` 会**重建**该 service——
  `make initES` 会打印两行（旧名 `Recreated` + 新名 `Started`），这是同一个 ES 的重建过程，不是起了两个。若旧容器残留占用 9200，用
  `docker ps -a | grep -i elastic` 查出来后 `docker rm -f <旧名>`；数据在 `${ES_DIR}` 卷里，不受改名影响。
- `es-init.sh` 判重只看**运行中**且同名容器（`docker ps | grep -w ${ES_NAME}`），所以改名后总会打印一次 `es don't exist, to create es`，属正常。
