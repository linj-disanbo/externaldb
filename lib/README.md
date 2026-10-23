# lib —— 底层库

被 `convert/`（ES 管道）、`erc20/`（MySQL 管道）和 `cmd/` 共用；**不依赖 `convert/` 或 `erc20/`**。

| 目录 | 内容 |
|---|---|
| `schema/` | 文档契约：`Record`/`Key`/`IKey`/`Op`/`SeqType`/`Block`、`TxEnv`、`DBCreator`、`WrapDB`、seq 索引常量、`ErrDB*` |
| `esdoc/` | 具体文档类型与索引：`account` `transaction` `address` `block` `blockinfo` `seqmark` `contract` `contractverify` |
| `escli/` | ES7 客户端（`v7/`）+ 查询参数（`querypara/`）、聚合解码、状态 |
| `esmaint/` | 索引维护：`InitIndex`/`InitMapSet`/`ESCheckAndRepair`、`LastSyncSeq`/`SaveToES`、进度缓存 |
| `store/` | 老链路 seq 存储抽象（`SeqStore`/`SeqNumStore`）与 `syncseq` 实现 |
| `util/` | 块处理编排（`process.go` 的 `ModuleConvert`/`DealBlockDetail`）、日志、`GenEnv`、`accountrefresh`/`health`/`proof`/`localfile` |
| `chaincfg/` | chain33 初始化（内嵌 v1.72 完整配置）、默认配置常量、地址驱动/执行器地址 |
| `proto/` | 配置与协议 pb 生成物（`.proto` + `*.pb.go`） |
| `version/` | 版本号（`VERSION_LDFLAGS` 注入） |

**依赖方向**：`lib` ← `convert`、`lib` ← `erc20`、`lib`+`convert` ← `cmd`；`convert` 与 `erc20` 互不相识。
`lib` 内部：`schema` 最底，`escli`/`esdoc`/`store`/`esmaint`/`chaincfg`/`util` 在它之上。
