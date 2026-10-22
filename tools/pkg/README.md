用于快速部署：在 `.env` 里填好物料表参数后，一条 `make quickStart` 起 ES + 初始化配置 + 起展开服务。
本包是老链路（chain33 推送 → `sync` → ES `seq05_seq` → `convert` → ES，`jrpc`/`rpc` 查询）的部署包；
直连链路（`pull_convert`）用另一个包 `externaldb-pull.tgz`。

只支持 ES 7.x（`escli/v6` 已删除）。包内 `env/es7` 是默认模板（7.17），`env/es7.4` 是 7.4 备用：
老环境要用 7.4，把 `env/es7.4` 的内容换到 `env/es7` 即可，不改脚本、不加版本配置。

### 1. 软件包的生成
##### （1）`make build`：编译运行程序
##### （2）`make pkg`：生成部署包 `build/externaldb.tgz`

包内结构：

```
externaldb/
├── bin/            sync、convert、rpc、jrpc（四个程序都装，起哪些由 EX_SERVICE_TYPE 决定）
├── etc/            externaldb.toml、chain33.toml
├── env/es7/        ES 7.17 的 compose + proof-init.bash（默认）
├── env/es7.4/      ES 7.4.2 的 compose + proof-init.bash（备用，见上）
├── docker-compose.yaml      四个服务的声明；按场景只起其中几个
├── alter-config.sh          把 .env 的参数写进 etc/externaldb.toml（并按场景改 browser 配置段）
├── es-init.sh               起包内 ES
├── browser-init.bash        浏览器的索引 settings（max_result_window / refresh_interval / translog）
├── reindex-shards.sh        给已有索引换多分片（reindex + 同名别名）
├── Makefile / README.md / .env
└── docker-compose-update.yaml   边同步边切换新前缀时用（init-update 系列目标）
```

### 2. 运行程序
##### （1）解压程序包
##### （2）修改 `.env`（物料表相关参数）
- `EX_SERVICE_TYPE`：`proof`（存证）/ `browser`（区块链浏览器）/ 其它（通用，全部服务）
- `ES_VERSION`：只支持 7（填 6 会被 `make` 直接拦下）
- `PROJECT_NAME`：容器名前缀（同一台机器多套时用来区分）
- ES 相关：`ES_NAME`/`ES_DIR`/`ES_BIND`/`ES_PORT_HTTP`/`ES_PASSWORD`
- 链相关：`CHAIN_TITLE`/`CHAIN_HOST`/`CHAIN_GRPC_HOST`/`CHAIN_SYMBOL`/`ADDR_DRIVER`/`DEAL_OTHER_CHAIN`
- 索引前缀：`SYNC_PREFIX`/`CONVERT_PREFIX`；起始高度：`START_SEQ`/`START_HEIGHT`
- 推送注册：`PUSH_BIND`/`PUSH_HOST`/`PUSH_NAME`/`PUSH_FORMAT`
- 存证管理员：`PROOF_MANAGER`（逗号分隔，proof-init.bash 写入 `proof_config`）
- 端口：`RPC_HOST`/`JRPC_HOST`/`SWAGGER_HOST`

##### （3）启动（推荐分开启动；连已有 ES 时跳过 `initES`）
**A. 一键启动**：`make quickStart`（起 ES → alter-config → proof-init + browser-init → 按场景起服务）

**B. 分开启动**
- `make initES`：起包内 ES
- `make initConfig`：写配置 + 跑 ES 初始化（`proof-init.bash` 建存证索引/管理员；`browser-init.bash` 调索引 settings）。
  两步都会跑，多跑的没有副作用，所以三个场景可以共用一套物料。
- `make run`：按 `EX_SERVICE_TYPE` 起服务（proof → sync/convert/rpc，browser → sync/convert/jrpc，其它 → 全部）
- 其它：`make down` / `make downES` / `make restart` / `make stop` / `make rm` / `make logs[-sync|-convert|-rpc|-jrpc]` / `make clean`
- 索引换分片：`make reindex-shards N=3 IDX="account transaction"`（先停 convert）

### 3. 更新 sync 和 convert 模块 —— 重新同步区块数据
（1）改 `.env` 的 `PUSH_BIND`/`PUSH_HOST`/`PUSH_NAME` 重新注册推送；要换 index 就改 `SYNC_PREFIX`/`CONVERT_PREFIX`
（2）`make initConfig`：初始化管理员地址 + 把配置刷进 `externaldb.toml`
（3）重启 `sync`、`convert` 和查询服务

### 4. 只更新 convert 模块 —— 重新解析区块数据
（1）改 `CONVERT_PREFIX`
（2）`make initConfig`
（3）重启 `convert` 和查询服务

### 5. 场景说明（不再需要手改 compose）
compose 里四个服务都声明好了，起哪些由 `EX_SERVICE_TYPE` 决定，不需要像旧包那样手工注释 `rpc`/`jrpc`：
- 存证（proof）：`sync convert rpc`，`alter-config.sh` 会把 browser 配置段注释掉并设 `openAccessControl=true`/`saveBlockInfo=false`
- 浏览器（browser）：`sync convert jrpc`，browser 配置段打开、`saveBlockInfo=true`
- 通用（common/其它）：四个全起
