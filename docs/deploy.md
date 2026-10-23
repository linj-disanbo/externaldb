# externaldb 部署

两种部署包，按链路二选一（同一批索引只能有一个写方）：

| 包 | 产物 | 内容 | 适用 |
|---|---|---|---|
| 老链路 `make pkg` | `build/externaldb.tgz` | `sync` `convert` `rpc` `jrpc` | 已有 push 链路的环境、需要回退时 |
| 直连链路 `make pkg-pull` | `build/externaldb-pull.tgz` | `pull_convert` `scanner`(=erc20scan) `jrpc` `erc20api` `account_refresh` + `tools/convert_check` | 新部署（当前生产） |

两个包的物料脚本是同一套（`deploy/pkg`、`deploy/env` 拷进包里），区别只在 `bin/` 的程序、包内 `docker-compose.yaml`
和包内 `Makefile`。

## 1. 打包含什么

```
externaldb/                 externaldb-pull/
├── bin/                    程序（前者 4 个，后者 5 个 + tools/convert_check）
├── etc/                    externaldb.toml、chain33.toml
├── env/es7/                ES 7.17 的 compose + proof-init.bash（默认）
├── env/es7.4/              ES 7.4.2 的 compose + proof-init.bash（备用）
├── env/mysql/              包内 MySQL（仅 pull 包，MYSQL_LOCAL_START=true 时用）
├── docker-compose.yaml     服务声明；起哪些由 EX_SERVICE_TYPE / 开关决定
├── Makefile                包内一键脚本（quickStart / initES / initConfig / run / logs…）
├── README.md               包内说明（与本文件对应）
├── alter-config.sh         把 .env 写进 etc/externaldb.toml
├── es-init.sh              起包内 ES
├── browser-init.bash       浏览器索引 settings（max_result_window / refresh_interval / translog）
└── reindex-shards.sh       已有索引换分片（reindex + 同名别名）
```

ES 只支持 7.x（项目已删掉 ES6 客户端）。包内默认 `env/es7` 是 7.17.29；
老环境要用 7.4，把 `env/es7.4` 的内容换到 `env/es7` 即可，不改脚本、不加版本配置。

## 2. `.env` 配置

解压后先改 `.env`：

| 分类 | 变量 | 说明 |
|---|---|---|
| 场景 | `EX_SERVICE_TYPE` | `proof`（存证）/ `browser`（浏览器）/ 其它（通用） |
| | `PROJECT_NAME` | 容器名前缀，同机多套时用来区分 |
| ES | `ES_VERSION` | 只填 7（填 6 会被 `make` 直接拦下）|
| | `ES_SHARDS` / `ES_REPLICAS` | 新建索引的分片/副本 |
| | `ES_NAME` / `ES_DIR` / `ES_BIND` / `ES_PORT_HTTP` / `ES_PORT_TCP` / `ES_PASSWORD` | 包内 ES 容器 |
| 链 | `CHAIN_TITLE` | 主链留空；平行链填 `user.p.XXX.` |
| | `CHAIN_HOST` / `CHAIN_GRPC_HOST` | 节点 HTTP / gRPC 地址（直连链路用 gRPC）|
| | `CHAIN_SYMBOL` / `ADDR_DRIVER` / `DEAL_OTHER_CHAIN` | 币种、地址驱动（btc/eth）、是否处理它链交易 |
| 索引 | `SYNC_PREFIX` / `CONVERT_PREFIX` | 原始 seq 索引 / 解析后文档索引前缀 |
| 起点 | `START_SEQ` / `START_HEIGHT` | 起始 seq / 高度 |
| 老链路推送 | `PUSH_BIND` / `PUSH_HOST` / `PUSH_NAME` / `PUSH_FORMAT` | sync 注册到节点的推送配置（pb/json）|
| 端口 | `RPC_HOST` / `JRPC_HOST` / `SWAGGER_HOST` | rpc / jrpc 监听地址 |
| 存证 | `PROOF_MANAGER` | 存证管理员地址（逗号分隔，`proof-init.bash` 写入 `proof_config`）|
| ERC20 | `MYSQL_ENABLED` | `true` 才起 `scanner`/`erc20api` |
| | `MYSQL_LOCAL_START` | `true` 起包内 MySQL 容器；连已有实例填 `MYSQL_DSN` |
| | `MYSQL_NAME`/`MYSQL_DIR`/`MYSQL_BIND`/`MYSQL_PORT`/`MYSQL_ROOT_PASSWORD`/`MYSQL_DATABASE` | 包内 MySQL 参数 |
| | `ERC20_ETH_URL` / `ERC20_START_BLOCK` / `ERC20_API_PORT` | 链上 `balanceOf` 的 eth JSON-RPC、扫块起点、API 端口 |
| 余额校正 | `ACCOUNT_REFRESH_ENABLED` / `ACCOUNT_REFRESH_TOP_N` / `*_INTERVAL` | `account_refresh` 三档间隔（top 1h / 活跃 24h / 全量 168h）|

`etc/chain33.toml` 是**完整**的 chain33 配置（含所有 fork 段）。convert 家族的程序会做 fork 严格校验，
少一段就会启动 panic —— 升级 chain33/plugin 后要同步这份文件，不要只改局部。
（内嵌给单测/工具用的那份默认配置会 `DisableForkCheck`，生产不要用。）

## 3. 启动

```bash
unzip externaldb.tgz && cd externaldb
vi .env

make quickStart      # 起 ES → 等就绪 → alter-config → proof/browser 初始化 → 起服务
```

分步执行（推荐连已有 ES 时）：

```bash
make initES          # 起包内 ES（连已有 ES 则跳过）
make initConfig      # 写配置 + 索引初始化（proof-init + browser-init 都跑，幂等）
make run             # 按 EX_SERVICE_TYPE 起服务
```

`EX_SERVICE_TYPE` 只决定起哪些服务；初始化固定两步都跑（多跑没有副作用）：

| 值 | 起的服务（老链路包）| jrpc 的 browser 段 |
|---|---|---|
| `proof` | sync convert rpc | 关闭，`openAccessControl=true`、`saveBlockInfo=false` |
| `browser` | sync convert jrpc | 打开，`saveBlockInfo=true` |
| 其它 | 全部 | 打开 |

直连包同理：`pull_convert` 必起，`proof` 场景不起 jrpc；MySQL 开关与 `ACCOUNT_REFRESH_ENABLED` 决定
`scanner`/`erc20api`/`account_refresh` 是否启动。

其它常用命令：`make logs[-sync|-convert|-rpc|-jrpc]`、`make stop|restart|down`、`make downES`、
`make es-indices`（看索引与条数）、`make reindex-shards N=3 IDX="account transaction"`（换分片，先停写）。

## 4. 直连链路要点

- 起点优先级：`-from` → 已有 `seq_marker`（与节点比对）→ 旧 `last_seq` 自举 → `startSeq-1`。
- 进度索引 `seq_marker` 是 append-only（doc id = seq，游标 = max(seq)），回滚时补写 del 记录。
- 只修一段：`./bin/pull_convert -mode fix -from <seq> -to <seq> -f etc/externaldb.toml -c etc/chain33.toml`
  （不写 marker、不做统计累计；修完仍要用 `convert_check` 对账）。
- `[convert]` 里的 `pullBatch`/`pullPollMs`/`pullDetailCache`/`pullMarkerKeep` 控制批量、轮询间隔、
  详情缓存与 marker 保留条数。
- 切换写方：停旧写方 → 起 `pull_convert`（自动自举游标）→ `tools/convert_check` 对账 → 观察 marker 单调。

## 5. 常见问题

| 现象 | 处理 |
|---|---|
| 启动即 panic `exec xxx not enable in config file` | `etc/chain33.toml` 缺 fork 段（升级 chain33/plugin 后常见），补齐 |
| ES 写入 429 / coordination 断路器拒绝 | 堆太小（2g 起，按机器调）；大 seq（genesis）会写大请求 |
| ES 重启特别慢 | `seq05_seq`/大索引 translog 回放；重启前 `_flush`，或停写后再重启 |
| `browser-init.bash` 首次跑没效果 | 目标索引还不存在时 PUT `_settings` 返回 404，但 curl 退出码仍是 0；先让 convert 建索引再跑一次 |
| 直连扫块静默退出 | 节点 eth JSON-RPC 不通，程序启动自检会 FATAL（`erc20scan` 用 `[erc20scan].ethUrl`，注意用 HTTP 端口，不是 ws）|
| 分片想从 1 改 3 | 分片创建后不可改，只能 reindex 到新索引再建同名别名（`reindex-shards.sh`）|
| 7.17 起不来 `max virtual memory areas` | `sysctl -w vm.max_map_count=262144` |

## 6. 镜像部署（可选）

用 docker 镜像（`chain33/externaldb/*`）而非解包二进制时，用 `deploy/docker/docker-compose.yaml`：
先 `make build` 再 `make docker` 打镜像，compose 里 `etc/` 挂载配置目录，参数同样走 `.env`。
两种方式不要混用（同一批索引只能有一个写方）。

## 7. 相关文档

- 转换器与新增合约解析：`convert/README.md`
- 底层库分层：`lib/README.md`
- 需求文档：`docs/requirements/`
- 运行手册与排查记录：`local/docs/`（私有，不入库）
