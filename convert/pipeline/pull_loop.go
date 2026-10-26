package pipeline

import (
	"context"
	"time"

	"github.com/33cn/chain33/types"
	"github.com/pkg/errors"

	"github.com/33cn/externaldb/lib/esdoc/block"
	"github.com/33cn/externaldb/lib/esdoc/seqmark"
	"github.com/33cn/externaldb/lib/schema"
)

// BlockDealer 处理单个区块（*util.ModuleConvert 满足：含 del 的逆序反做、不写 lastRecord）
type BlockDealer interface {
	DealBlockDetail(blockSeq *block.Seq, detail *types.BlockDetail) ([]schema.Record, error)
}

// 默认值
const (
	DefaultPullBatch        = 100
	DefaultPollInterval     = time.Second
	DefaultPruneEvery       = int64(10000)
	maxPullBatch            = 999
	pullLogEveryNDefault    = int64(1000)
	pullBatchRetrySleepBase = 500 * time.Millisecond
	// DefaultWriteRetryInterval 写记录失败（WriteRecords 内置 3 次重试用尽）后的重试间隔。
	// 之后按该间隔无限重试，不再让进程退出：游标不前进，成功后继续处理。
	DefaultWriteRetryInterval = time.Minute
)

// PullOptions 主循环参数
type PullOptions struct {
	Cursor       int64         // 已处理完成的最后一个 seq（无 marker 时为 startSeq-1）
	Batch        int           // 单批 seq 数，<=0 用 DefaultPullBatch，上限 999
	PollInterval time.Duration // 追平链尖后的轮询间隔，<=0 用 DefaultPollInterval
	MarkerKeep   int64         // marker 保留窗口（条数），0 表示不滚动删除
	PruneEvery   int64         // 每推进多少个 seq 触发一次滚动删除，<=0 用 DefaultPruneEvery

	// WriteRetryInterval 写记录失败后的重试间隔，<=0 用 DefaultWriteRetryInterval（1 分钟）。
	// 注意：只重发"写"（记录是绝对值，重发幂等），不重跑转换器（会把累计值再算一遍）。
	WriteRetryInterval time.Duration

	// FixMode=true 表示"修复模式"：只处理到 EndSeq（含）就正常退出，不跟链尖。
	// 修复模式同时应设 SkipMarker=true（不写 seq_marker，游标仍由正常 run 维护）。
	FixMode    bool
	EndSeq     int64
	SkipMarker bool
}

// RunPull 直连主循环：按 seq 拉取 → 解析 → 写记录 → 写 marker（游标）→ 前进。
//
// 关键约定：
//   - 只有该 seq 的记录全部写入成功才写 marker；
//   - 写记录失败不退出：WriteRecords 内部先只重试失败项（3 次），仍失败则按
//     WriteRetryInterval（默认 1 分钟）无限重试，游标不动，成功后继续；
//   - 解析（转换）失败直接返回错误，绝不跳过（重跑转换器会让累计值重复计数）；
//   - 数据完整性问题（seq 缺号）直接返回错误（source 层用 ErrSeqHole 标记），不做重试；
//   - 其他 RPC 错误按 poll 间隔重试，游标不动。
func RunPull(ctx context.Context, src SeqSource, w SeqWriter, dealer BlockDealer, cache *DetailCache, opt PullOptions) error {
	cursor := opt.Cursor
	batch := opt.Batch
	if batch <= 0 {
		batch = DefaultPullBatch
	}
	if batch > maxPullBatch {
		batch = maxPullBatch
	}
	poll := opt.PollInterval
	if poll <= 0 {
		poll = DefaultPollInterval
	}
	pruneEvery := opt.PruneEvery
	if pruneEvery <= 0 {
		pruneEvery = DefaultPruneEvery
	}
	writeRetry := opt.WriteRetryInterval
	if writeRetry <= 0 {
		writeRetry = DefaultWriteRetryInterval
	}

	log.Info("pull_convert start", "cursor", cursor, "batch", batch, "poll", poll,
		"markerKeep", opt.MarkerKeep, "cacheSize", cache.Len(), "writeRetry", writeRetry,
		"fixMode", opt.FixMode, "endSeq", opt.EndSeq, "skipMarker", opt.SkipMarker)

	// 启动时滚动删除一次旧 marker
	if opt.MarkerKeep > 0 && cursor > opt.MarkerKeep {
		if err := w.PruneMarkers(cursor - opt.MarkerKeep); err != nil {
			log.Error("pull_convert prune markers", "err", err)
		}
	}

	var sincePrune int64
	for {
		if ctx.Err() != nil {
			log.Info("pull_convert exit", "cursor", cursor)
			return nil
		}

		last, err := src.LastSeq()
		if err != nil {
			log.Error("pull_convert LastSeq", "err", err, "cursor", cursor)
			if sleepCtx(ctx, poll) != nil {
				return nil
			}
			continue
		}
		if opt.FixMode {
			if cursor >= opt.EndSeq {
				log.Info("pull_convert fix done", "cursor", cursor, "endSeq", opt.EndSeq)
				return nil
			}
			// 修复区间必须已在节点 seq 日志里（否则是配置错误，不重试）
			if last < opt.EndSeq {
				return errors.Errorf("pull_convert fix: 节点 seq 只到 %d，小于 -to %d", last, opt.EndSeq)
			}
		}

		if cursor >= last {
			// 追平：等待新区块/新 seq（含回滚产生的 del seq）
			if sleepCtx(ctx, poll) != nil {
				return nil
			}
			continue
		}

		end := cursor + int64(batch)
		if end > last {
			end = last
		}
		if opt.FixMode && end > opt.EndSeq {
			end = opt.EndSeq
		}
		bs, err := src.Batch(cursor+1, end)
		if err != nil {
			if errors.Is(err, ErrSeqHole) {
				// 数据完整性问题：交人工处理，不重试、不跳过
				return errors.Wrapf(err, "pull_convert: seq hole in %d-%d", cursor+1, end)
			}
			log.Error("pull_convert Batch", "err", err, "start", cursor+1, "end", end)
			if sleepCtx(ctx, poll) != nil {
				return nil
			}
			continue
		}
		if len(bs) == 0 {
			if sleepCtx(ctx, poll) != nil {
				return nil
			}
			continue
		}

		for _, b := range bs {
			if err := processSeq(ctx, w, src, dealer, cache, b, !opt.SkipMarker, writeRetry); err != nil {
				// 写记录失败已被 writeRecordsUntilSuccess 吞掉重试；走到这里只剩转换/取块等
				// 不可重试的错误，或 ctx 结束（重试中被 kill）。
				if ctx.Err() != nil {
					log.Info("pull_convert exit", "cursor", cursor)
					return nil
				}
				return err
			}
			cursor = b.Seq
			sincePrune++
			if opt.MarkerKeep > 0 && sincePrune >= pruneEvery {
				sincePrune = 0
				if cursor > opt.MarkerKeep {
					if err := w.PruneMarkers(cursor - opt.MarkerKeep); err != nil {
						log.Error("pull_convert prune markers", "err", err, "before", cursor-opt.MarkerKeep)
					}
				}
			}
		}
		log.Info("pull_convert batch done", "cursor", cursor, "lastSeq", last)
	}
}

// processSeq 处理一个 seq：补 detail（回滚兜底）→ 解析 → 写记录 → 写 marker。
// 任一步失败都不推进游标。writeMarker=false 用于修复模式（不碰游标）。
// 写记录失败按 writeRetry 无限重试（ctx 结束才返回），转换/取块失败直接返回。
func processSeq(ctx context.Context, w SeqWriter, src SeqSource, dealer BlockDealer, cache *DetailCache,
	b SeqBlock, writeMarker bool, writeRetry time.Duration) error {
	if b.Detail == nil {
		// del seq 的块体可能已被节点 prune：先查本地缓存，再按 hash 向节点兜底
		if cache != nil {
			b.Detail = cache.Get(b.Hash)
		}
		if b.Detail == nil && src != nil {
			d, err := src.BlockByHash(b.Hash)
			if err != nil {
				return errors.Wrapf(err, "pull_convert: 取旧块 detail seq %d (hash %s)", b.Seq, b.Hash)
			}
			b.Detail = d
		}
		if b.Detail == nil {
			return errors.Errorf("pull_convert: seq %d (hash %s, type %d) 无 block detail：缓存与节点都没有，无法回滚",
				b.Seq, b.Hash, b.Type)
		}
	}
	blk := &block.Seq{
		SyncSeq: int(b.Seq),
		Number:  int(b.Height),
		Hash:    b.Hash,
		Type:    b.Type,
		From:    "pull",
	}
	records, err := dealer.DealBlockDetail(blk, b.Detail)
	if err != nil {
		return errors.Wrapf(err, "pull_convert: convert seq %d (height %d)", b.Seq, b.Height)
	}
	if err := writeRecordsUntilSuccess(ctx, w, records, writeRetry); err != nil {
		return errors.Wrapf(err, "pull_convert: write seq %d (height %d)", b.Seq, b.Height)
	}
	if writeMarker {
		if err := w.WriteMarker(&seqmark.Marker{Seq: b.Seq, Height: b.Height, Hash: b.Hash, Type: b.Type}); err != nil {
			return errors.Wrapf(err, "pull_convert: marker seq %d (height %d)", b.Seq, b.Height)
		}
	}
	if cache != nil {
		cache.Put(b.Hash, b.Detail)
	}
	return nil
}

// writeRecordsUntilSuccess 写一个 seq 的记录，失败不退出：
// WriteRecords 内部已按大小分片、只重试失败项（3 次）、429 类整块退避；仍失败则按 interval
// 无限重试，直到成功或 ctx 结束。只重发"写"，不重跑 DealBlockDetail——后者含读改写累计
// （block_stat 等），重跑会重复计数；记录本身是绝对值，重发幂等。
func writeRecordsUntilSuccess(ctx context.Context, w SeqWriter, records []schema.Record, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultWriteRetryInterval
	}
	for attempt := 1; ; attempt++ {
		err := w.WriteRecords(records)
		if err == nil {
			return nil
		}
		log.Error("pull_convert write records failed, will retry",
			"attempt", attempt, "interval", interval, "err", err)
		if sleepCtx(ctx, interval) != nil {
			return err
		}
	}
}

// sleepCtx 可被取消的 sleep；ctx 结束返回非 nil
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
