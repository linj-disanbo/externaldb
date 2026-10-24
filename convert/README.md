# convert —— ES 落地管道

> 目录重组已完成（方案见 `local/docs/design/code-organization.md`）：
> 顶层只剩 `cmd/ convert/ erc20/ lib/ deploy/ proto→lib/proto config/ docs/ testdata/ local/`。

把 chain33 的区块/交易展开成 ES 文档（索引 + 文档 + 统计），两条链路共用这里的转换器与文档契约：

```
直连链路（当前生产）：chain33 node (gRPC pull + seq) → pull_convert → ES
老链路（保留备用）  ：chain33 node (push) → sync → ES(seq05_seq) → convert → ES
```

`jrpc` / `rpc` 只读 ES（见 `cmd/jrpc`、`cmd/rpc`），不在这里。

## 目录

| 路径 | 内容 |
|---|---|
| `convert.go` `plugin.go` `registry.go` | **转换器框架**：`Convert`/`ExecConvert`/`DBSaver` 接口、`TxEnv`、注册表（`RegisterConvert`/`LoadConvert`）|
| `pipeline/` | 运行驱动：`App`/`Service` 装配、pull 循环（`RunPull`）、seq 源/写/游标 |
| `receiver/` | 推送链路的块接收（`Receiver`/`SeqsProc`/`DealBlocks`）|
| `stat/` | 累积型转换的接口与注册表（`Stat`/`HeightRecover`）；`stat/block/` 是区块累计统计的实现 |
| `coins/` `coinsx/` `token/` `ticket/` `trade/` `multisig/` `pos33/` `proof/` `proof_config/` `exchange/` `evmxgo/` `unfreeze/` `common/` `filepart/` `filesummary/` `evm/` | 各合约/执行器的转换器；`evm/` 下另有 `erc20`/`erc721`/`erc1155`/`nft` |

依赖方向：`convert → lib`；`cmd → convert`。这里不反向依赖 `cmd`，也不依赖 `erc20/`（MySQL 那条链路）。文档契约（`Record`/`Key`/`IKey`/`Op`/`SeqType`/`TxEnv`/`WrapDB`）在 `lib/schema`，具体文档类型（`account`/`transaction`/`blockinfo`/…）在 `lib/esdoc`，ES 客户端在 `lib/escli`，索引维护与进度缓存在 `lib/esmaint`，chain33 初始化/地址驱动在 `lib/chaincfg`。

## 新增一个转换器

1. 在 `convert/<name>/` 下建包，实现 `ExecConvert`：
   - `ConvertTx(env *lib.TxEnv, op int) ([]schema.Record, error)` —— 把一笔交易展开成若干文档记录；
   - `InitDB(cli schema.DBCreator) error` —— 声明自己用到的索引（`util.InitIndex`）。
2. 在包内 `init()` 里注册：`convert.RegisterConvert("<name>", NewConvert)`（import `converter "github.com/33cn/externaldb/convert"`）。
3. 在 `cmd/convert/main.go`、`cmd/pull_convert/main.go` 加 blank import。
4. 配置里 `[convert].data` 的 exec 名单决定加载哪些转换器（`pipeline.NewApp` 按名字 `LoadConvert`），名字要与注册名一致。
5. 补单测：用 `util.GenEnv(blockHex, txIndex)` 喂真实区块，断言产出的 `Key()`/`Value()`（参考 `coins/convert_test.go`）。

## 注意

- **不要在这里发链上/外部请求**：转换是纯函数式的（输入 `TxEnv`，输出记录），查询与网络调用属于 `lib/` 或各自 `cmd/`。
- 回滚（`op = schema.SeqTypeDel`）必须能反做：文档 id 里带高度的（`*_record`）按高度删除，不带高度的（当前态）写回 `Prev` 值；见 `lib/esdoc/account` 的 `RecordHelper` 注释。
- 同一笔交易里同一 doc id 可能被写多次（如 coins 先扣费再转出），后写覆盖前写，属预期。
