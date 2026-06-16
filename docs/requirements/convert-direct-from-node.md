# Convert 直连节点 + 本地文件进度

## 背景

当前数据流：

```
chain33 → (HTTP push) → sync → ES (block.Seq) → convert → ES (records)
                ↑                        ↑              ↑
          UpdateLastSeq           SaveSeqs       lastRecord (每个区块)
          (高频覆盖 last_seq 文档)               (高频覆盖 last_seq 文档)
```

问题：

1. **sync 是多余的中间层**：节点数据先存 ES 再读出来，多了 ES 中转
2. **进度热点文档**：`last_seq/<id>` 被每个区块高频覆盖更新，ES 写放大严重
3. **数据+进度耦合在 ES 中**：互相影响，崩溃时可能不一致
4. **sync 和 convert 是独立两个进程**：运维复杂

## 目标

1. **去掉 sync 程序**，convert 通过 chain33 gRPC 直接读取区块
2. **进度改用本地文件**，不再写 ES：
   - `last_sync` — 从节点已拉取到的最新 seq/height
   - `last_convert` — convert 已处理完成的最新 seq/height
3. 不引入 Redis、etcd 等外部模块

## 架构变更

```
之前:  chain33 → (push) → sync → ES → convert → ES
之后:  chain33 → (gRPC GetBlocks) → convert → ES
                       ↑
                 本地文件记录进度
```

## 详细任务

### T1. 新增本地文件进度存储 `util/localfile/progress.go`

- [ ] 创建 `util/localfile/progress.go`
- [ ] 实现 `FileProgress` 结构体
- [ ] 接口：`Load() (int64, error)` / `Save(seq int64) error`
- [ ] 文件路径通过配置指定，默认 `<workDir>/data/last_sync` 和 `<workDir>/data/last_convert`
- [ ] 原子写入：先写临时文件再 rename + sync 父目录
- [ ] 启动时若文件不存在，使用配置的 `startSeq` 作为初始值

### T2. convert 直连 chain33 读取区块

- [ ] 新建 `util/cli/convert/chain_reader.go`
- [ ] 实现 `ChainReader`：封装 chain33 gRPC 连接
- [ ] 使用 `types.Chain33Client.GetBlocks()` 按高度范围拉取（已有 `cfg.Chain.GrpcHost`）
- [ ] 批量拉取：一次 `batchSize` 个区块（建议 50-100）
- [ ] 获取当前链最新高度：`GetLastHeader`
- [ ] 设置 `MaxCallRecvMsgSize` 100MB 防止大区块超时

### T3. 重构 `ModuleConvert.BlockProc()`

**当前流程**：

```
LastSeq() → 从 ES 读 sync 进度
SeqStore.GetSeq(n) → 从 ES 读区块
dealBlock → ConvertBlock → 写 ES + lastRecord(进度)
```

**改为**：

```
FileProgress.Load() → 读本地 last_sync / last_convert
ChainReader.GetLastHeight() → 获取链最新高度
ChainReader.GetBlocks(start, end) → 从节点批量拉取区块
dealBlock → ConvertBlock → 批量写 ES（不含进度）
FileProgress.Save() → 更新本地进度文件
```

- [ ] 改造 `util/process.go` 中 `BlockProc()` 方法
- [ ] 去掉对 `store.SeqStore` / `store.SeqNumStore` 的依赖
- [ ] 新增 `ChainReader` + `FileProgress` 字段
- [ ] 保留 `BlockProcFixTool()` 并适配直连方式

### T4. 改造 `dealBlock()` — 去掉进度 record

- [ ] 去掉 `lastRecord := NewLastRecord(...)` 和 `records = append(records, lastRecord)`（[process.go:196-198](util/process.go#L196-L198)）
- [ ] 进度由 `BlockProc` 外层统一管理

### T5. 改造 convert 启动流程

- [ ] `cmd/convert/main.go`：`InitLastSyncSeqCache` → `FileProgress`
- [ ] `util/cli/convert/convert.go`：`InitSeqStore` / `InitSeqNum` → `InitChainReader` + `InitFileProgress`
- [ ] gRPC 连接生命周期：启动时连接，退出时 `defer conn.Close()`

### T6. 移除 sync 程序

- [ ] 删除 `cmd/sync/main.go` 和 `cmd/sync/` 目录
- [ ] 删除 `cmd/sync_convert/main.go` 和 `cmd/sync_convert/` 目录
- [ ] 删除 `util/cli/sync/` 目录 (`receive.go`, `receive_convert.go`, `block_proc.go`)
- [ ] 删除 `store/syncseq/` 目录
- [ ] 删除/简化 `store/seq.go` 相关接口
- [ ] 清理 `Makefile` 中 sync 相关构建目标
- [ ] 清理 `proto/exdbconfig.proto` 中 sync 配置字段（渐进）

### T7. 适配 combined 模式（原 sync_convert 替代）

- [ ] 新建 `cmd/convert_combined/` 或在 `cmd/convert/` 增加 `-mode combined` 选项
- [ ] combined 模式：一边从节点 gRPC 拉取新区块，一边 convert 写入 ES

### T8. 清理不再需要的代码

- [ ] `util/global_cache.go` 中的 `LastSyncSeqCache` — 可删除
- [ ] `util/status.go` 中的 `LastSyncSeq()` / `NewLastRecord()` — 可删除
- [ ] `db/block/block.go` 中的 `LastSyncSeqRecord` / `LastSyncSeq` — 可删除
- [ ] `db/table.go` 中的 `LastSeqDB` 常量 — 可删除
- [ ] ES 中 `last_seq` index 废弃

### T9. 测试与验证

- [ ] 单元测试 `util/localfile/progress_test.go`
- [ ] 集成测试：启动 convert，验证从指定高度开始正确读区块并写入 ES
- [ ] 断点续传测试：kill 后重启，从 `last_convert` 继续
- [ ] crash 场景测试：写 ES 成功但写文件前 crash，重启后重处理几个区块（幂等）
- [ ] 性能对比：直连节点 vs ES 中转方案

## 关键设计

### 进度文件原子写入

```go
func (p *FileProgress) Save(seq int64) error {
    tmp := p.path + ".tmp"
    // 写入 tmp
    // os.Rename(tmp, p.path)   // atomic on same filesystem
    // sync parent dir
}
```

### gRPC 批量拉取

```go
reply, err := cli.GetBlocks(ctx, &types.ReqBlocks{
    Start:    startHeight,
    End:      endHeight,        // startHeight + batchSize - 1
    IsDetail: true,
})
```

### BlockProc 主循环（新）

```go
func (mod *ModuleConvert) BlockProc() {
    lastConvert := mod.progressConvert.Load()   // last_convert 文件
    chainHeight := mod.chainReader.GetLastHeight()

    for lastConvert < chainHeight {
        batchEnd := min(lastConvert + batchSize, chainHeight)
        blocks := mod.chainReader.GetBlocks(lastConvert + 1, batchEnd)
        // accumulate records per block
        SaveToES(mod.WriteDB, allRecords)
        mod.progressSync.Save(chainHeight)        // last_sync
        mod.progressConvert.Save(batchEnd)        // last_convert
        lastConvert = batchEnd
        chainHeight = mod.chainReader.GetLastHeight()
    }
    time.Sleep(pollInterval)
}
```

### dealBlock 不再塞 lastRecord

```go
func (mod *ModuleConvert) dealBlock(detail *types.BlockDetail, blockHash string, seqType int) ([]db.Record, error) {
    blockSeq := &block.Seq{Hash: blockHash, Type: seqType, ...}
    return mod.ConvertBlock(blockSeq, detail)
    // 不再: records = append(records, NewLastRecord(...))
}
```

## 影响范围总览

| 模块 | 影响 |
|------|------|
| `cmd/sync/` | 删除 |
| `cmd/sync_convert/` | 删除 |
| `util/cli/sync/` | 删除 |
| `store/syncseq/` | 删除 |
| `store/seq.go` | 大幅简化 |
| `cmd/convert/main.go` | 改造启动流程 |
| `util/cli/convert/convert.go` | 替换 SeqStore → ChainReader + FileProgress |
| `util/process.go` | BlockProc 重写 |
| `util/global_cache.go` | 删除 LastSyncSeqCache |
| `util/status.go` | 删除 LastSyncSeq / NewLastRecord |
| `db/block/block.go` | 删除 LastSyncSeqRecord |
| `db/table.go` | 删除 LastSeqDB |
| `Makefile` | 删除 sync 构建目标 |
| **新增** `util/localfile/` | 本地文件进度存储 |
| **新增** `util/cli/convert/chain_reader.go` | gRPC 区块读取 |

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| gRPC 大区块超时 | `MaxCallRecvMsgSize=100MB`，控制 batch size |
| 节点压力 | 批量拉取 + 控制拉取间隔 |
| 本地文件损坏 | 启动时从 ES 数据反推进度作为兜底 |
| crash 丢失少量进度 | 重启后重处理，convert 幂等 |
