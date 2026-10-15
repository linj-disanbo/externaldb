# pkgV2：pull_convert 直连部署包

用于 **convert 直连（pull + seq 驱动）** 的一键部署，取代老链路 `sync + convert`。
设计与切换步骤见 `docs/requirements/convert-direct-from-node.md`（P4 Runbook、P5 对账）。

## 包内容

```
externaldb-proof/
├── bin/            rpc、jrpc、pull_convert、convert_check（+ 老链路 sync/convert，回退用）
├── etc/            externaldb.toml、chain33.toml、mime.types
├── env/es7/        ES 7.4.2 的 compose + proof-init.bash
├── env/es7.17/     ES 7.17.x 的 compose + proof-init.bash
├── ex-merge/       sync_convert 合并模式（老链路，回退用）
├── docker-compose.yaml        默认只起 pull_convert / rpc / jrpc（老链路已注释）
├── Makefile                    内层一键脚本
├── .env                        物料表相关配置
├── alter-config.sh / es-init.sh / browser-init.bash
└── README.md
```

## 使用

1. 改 `.env`：`EX_SERVICE_TYPE`（proof/browser/common）、`ES_VERSION`（**7** 或 **7.17**）、
   `PROJECT_NAME`、ES 密码/数据目录、链地址与 `CONVERT_PREFIX`/`SYNC_PREFIX` 等。
2. 按物料表改 `etc/externaldb.toml`（节点 gRPC/推送、ES 地址、`[convert].appName`、直连参数 `pullBatch`/`pullPollMs`/`pullDetailCache`/`pullMarkerKeep`）。
3. 一键启动：`make quickStart`；或分开 `make initES` → `make initConfig` → `make run`。
4. 对账：`make check`（= `convert_check`），或 `make es-marker` 看游标。
5. 回退：停掉 `pull_convert`，把 `docker-compose.yaml` 里 `sync`/`convert` 两段取消注释、注释掉 `pull_convert`，再 `make run`。

## 注意

- **同一批索引只能有一个写方**：`pull_convert` 与老 `convert` 不能同时跑（两套游标）。
- `pull_convert` 启动会自动自举游标；首次启动看日志里是 `已按旧 db05_last_seq 自举` 还是 `按配置 startSeq 起跑`——
  后者意味着会**从 startSeq 重跑**，切换前务必确认 `etc/externaldb.toml` 里 `[convert].startSeq` 与现状一致。
- ES 7.17 需要宿主机 `vm.max_map_count >= 262144`。
- 回滚/reorg 由节点 seq 日志里的 del 事件驱动，不需要额外配置；块体被 prune 的旧块靠 `pullDetailCache`（默认 1000）兜底。
