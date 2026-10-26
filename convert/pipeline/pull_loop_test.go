package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/33cn/chain33/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/33cn/externaldb/lib/esdoc/block"
	"github.com/33cn/externaldb/lib/esdoc/seqmark"
	"github.com/33cn/externaldb/lib/schema"
)

// ---------- 测试用的最小记录 ----------

type fakeRecord struct {
	*schema.IKey
	*schema.Op
}

func (r *fakeRecord) Value() []byte { return []byte(`{}`) }

func newFakeRecord(index, id string) *fakeRecord {
	return &fakeRecord{IKey: schema.NewIKey(index, index, id), Op: schema.NewOp(schema.OpAdd)}
}

// ---------- 落库重试（只重试失败项） ----------

type fakeBulk struct {
	calls   int
	failIDs map[string]bool
	failFor int // 前 failFor 次调用失败（0 = 从不失败）
}

func (f *fakeBulk) bulk(rs []schema.Record) error {
	f.calls++
	if f.failFor > 0 && f.calls > f.failFor {
		return nil
	}
	var errIDs []string
	for _, r := range rs {
		if f.failIDs[r.ID()] {
			errIDs = append(errIDs, r.ID())
		}
	}
	if len(errIDs) == 0 {
		return nil
	}
	return errors.Errorf("bulk failed: %v", errIDs)
}

func (f *fakeBulk) filter(rs []schema.Record, err error) []schema.Record {
	if err == nil {
		return nil
	}
	var out []schema.Record
	for _, r := range rs {
		if f.failIDs[r.ID()] {
			out = append(out, r)
		}
	}
	return out
}

func records(a, b, c bool) []schema.Record {
	rs := []schema.Record{newFakeRecord("account", "a")}
	if b {
		rs = append(rs, newFakeRecord("account", "b"))
	}
	if c {
		rs = append(rs, newFakeRecord("account", "c"))
	}
	return rs
}

func TestSaveWithRetryOnlyRetriesFailed(t *testing.T) {
	rs := []schema.Record{newFakeRecord("account", "a"), newFakeRecord("account", "b"), newFakeRecord("account", "c")}

	// 第一次整批（其中 b 失败）→ 第二次只写 b
	f := &fakeBulk{failIDs: map[string]bool{"b": true}, failFor: 1}
	assert.NoError(t, saveWithRetry(f.bulk, f.filter, rs, 3))
	assert.Equal(t, 2, f.calls, "整批 1 次 + 只重试失败项 1 次")

	// 一直失败：重试次数用尽后返回错误
	f2 := &fakeBulk{failIDs: map[string]bool{"b": true}, failFor: 99}
	err := saveWithRetry(f2.bulk, f2.filter, rs, 2)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "still failing after 2 retries")
	}
	assert.Equal(t, 3, f2.calls, "初次 + 2 次重试")

	// 无法定位失败项（filter 返回空）→ 立即失败，不做无谓重试
	f3 := &fakeBulk{failIDs: map[string]bool{"b": true}, failFor: 99}
	err = saveWithRetry(f3.bulk, func([]schema.Record, error) []schema.Record { return nil }, rs, 5)
	assert.Error(t, err)
	assert.Equal(t, 1, f3.calls)

	// 空列表不调用 ES
	f4 := &fakeBulk{failIDs: map[string]bool{}}
	assert.NoError(t, saveWithRetry(f4.bulk, f4.filter, nil, 3))
	assert.Equal(t, 0, f4.calls)
}

// ---------- 主循环 ----------

type fakeSource struct {
	last     int64
	blocks   map[int64]SeqBlock
	lastErr  error // 只返回一次的错误
	holeFrom int64 // >0 时该 seq 缺失（返回 ErrSeqHole）
	byHash   map[string]*types.BlockDetail
	calls    int
}

// BlockByHash 回滚兜底：按 hash 取旧块 detail
func (f *fakeSource) BlockByHash(hash string) (*types.BlockDetail, error) {
	if f.byHash == nil {
		return nil, nil
	}
	return f.byHash[hash], nil
}

func (f *fakeSource) LastSeq() (int64, error) {
	if f.lastErr != nil {
		err := f.lastErr
		f.lastErr = nil
		return 0, err
	}
	return f.last, nil
}

func (f *fakeSource) Batch(start, end int64) ([]SeqBlock, error) {
	f.calls++
	if f.holeFrom > 0 && start <= f.holeFrom && f.holeFrom <= end {
		return nil, errors.Wrapf(ErrSeqHole, "seq %d missing in node sequence log (range %d-%d)", f.holeFrom, start, end)
	}
	var out []SeqBlock
	for s := start; s <= end; s++ {
		if b, ok := f.blocks[s]; ok {
			out = append(out, b)
		}
	}
	return out, nil
}

type fakeWriter struct {
	calls      int
	records    [][]schema.Record
	markers    []*seqmark.Marker
	pruned     []int64
	failUntil  int // 前 failUntil 次 WriteRecords 失败（0 = 从不失败）
	markerFail bool
}

func (w *fakeWriter) WriteRecords(rs []schema.Record) error {
	w.calls++
	if w.calls <= w.failUntil {
		return errors.New("write records failed")
	}
	w.records = append(w.records, rs)
	return nil
}

func (w *fakeWriter) WriteMarker(m *seqmark.Marker) error {
	if w.markerFail {
		return errors.New("marker failed")
	}
	w.markers = append(w.markers, m)
	return nil
}

func (w *fakeWriter) PruneMarkers(before int64) error {
	w.pruned = append(w.pruned, before)
	return nil
}

type fakeDealer struct {
	called   int
	errOnSeq int64
}

func (d *fakeDealer) DealBlockDetail(blockSeq *block.Seq, detail *types.BlockDetail) ([]schema.Record, error) {
	d.called++
	if d.errOnSeq > 0 && blockSeq.SyncSeq == int(d.errOnSeq) {
		return nil, errors.New("convert failed")
	}
	return []schema.Record{newFakeRecord("block_info", blockSeq.Hash)}, nil
}

func seqBlock(seq, height int64, typ int) SeqBlock {
	return SeqBlock{
		Seq:    seq,
		Height: height,
		Hash:   "0x" + string(rune('a'+seq%26)),
		Type:   typ,
		Detail: &types.BlockDetail{Block: &types.Block{Height: height}},
	}
}

func startPull(t *testing.T, src SeqSource, w SeqWriter, dealer BlockDealer, opt PullOptions, run time.Duration) error {
	t.Helper()
	return startPullWithCache(t, src, w, dealer, NewDetailCache(4), opt, run)
}

func startPullWithCache(t *testing.T, src SeqSource, w SeqWriter, dealer BlockDealer, cache *DetailCache, opt PullOptions, run time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), run)
	defer cancel()
	return RunPull(ctx, src, w, dealer, cache, opt)
}

func blocksFor(from, to int64) map[int64]SeqBlock {
	m := map[int64]SeqBlock{}
	for i := from; i <= to; i++ {
		m[i] = seqBlock(i, i+1000, schema.SeqTypeAdd)
	}
	return m
}

func TestRunPullNormal(t *testing.T) {
	src := &fakeSource{last: 12, blocks: blocksFor(10, 12)}
	w := &fakeWriter{}
	d := &fakeDealer{}
	err := startPull(t, src, w, d, PullOptions{Cursor: 9, Batch: 10, PollInterval: 5 * time.Millisecond}, 120*time.Millisecond)
	assert.NoError(t, err, "ctx 超时属于正常退出")
	assert.Equal(t, 3, d.called)
	if assert.Len(t, w.markers, 3) {
		assert.Equal(t, int64(10), w.markers[0].Seq)
		assert.Equal(t, int64(1010), w.markers[0].Height)
		assert.Equal(t, int64(12), w.markers[2].Seq)
	}
	assert.Len(t, w.records, 3)
}

func TestRunPullConvertErrorStopsBeforeMarker(t *testing.T) {
	src := &fakeSource{last: 12, blocks: blocksFor(10, 12)}
	w := &fakeWriter{}
	d := &fakeDealer{errOnSeq: 11}
	err := startPull(t, src, w, d, PullOptions{Cursor: 9, Batch: 10, PollInterval: 5 * time.Millisecond}, time.Second)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "convert seq 11")
	}
	if assert.Len(t, w.markers, 1, "只应写入 seq 10 的 marker") {
		assert.Equal(t, int64(10), w.markers[0].Seq)
	}
}

// 写记录失败不再退出：按 WriteRetryInterval 重试，成功后写 marker 并继续。
func TestRunPullWriteErrorRetriesUntilSuccess(t *testing.T) {
	src := &fakeSource{last: 11, blocks: blocksFor(10, 11)}
	w := &fakeWriter{failUntil: 2} // seq 10 连续失败 2 次
	d := &fakeDealer{}
	err := startPull(t, src, w, d, PullOptions{
		Cursor: 9, Batch: 10, PollInterval: 5 * time.Millisecond,
		WriteRetryInterval: 5 * time.Millisecond,
	}, 300*time.Millisecond)
	assert.NoError(t, err)
	assert.Equal(t, 4, w.calls, "seq10 失败 2 次 + 成功 1 次，seq11 成功 1 次")
	if assert.Len(t, w.markers, 2) {
		assert.Equal(t, int64(10), w.markers[0].Seq)
		assert.Equal(t, int64(11), w.markers[1].Seq)
	}
}

// 一直失败：进程不退出，也不写 marker；ctx 结束后干净退出。
func TestRunPullWriteErrorRetriesUntilCanceled(t *testing.T) {
	src := &fakeSource{last: 10, blocks: blocksFor(10, 10)}
	w := &fakeWriter{failUntil: 1 << 30}
	d := &fakeDealer{}
	err := startPull(t, src, w, d, PullOptions{
		Cursor: 9, Batch: 10, PollInterval: 5 * time.Millisecond,
		WriteRetryInterval: 10 * time.Millisecond,
	}, 80*time.Millisecond)
	assert.NoError(t, err, "ctx 结束属于正常退出")
	assert.Greater(t, w.calls, 2, "应持续重试而不是退出")
	assert.Empty(t, w.markers, "写记录一直失败时不得写 marker")
}

func TestRunPullMarkerErrorStops(t *testing.T) {
	src := &fakeSource{last: 10, blocks: blocksFor(10, 10)}
	w := &fakeWriter{markerFail: true}
	d := &fakeDealer{}
	err := startPull(t, src, w, d, PullOptions{Cursor: 9, Batch: 10, PollInterval: 5 * time.Millisecond}, time.Second)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "marker seq 10")
	}
}

func TestRunPullSeqHoleIsFatal(t *testing.T) {
	src := &fakeSource{last: 12, holeFrom: 11, blocks: blocksFor(10, 12)}
	w := &fakeWriter{}
	d := &fakeDealer{}
	err := startPull(t, src, w, d, PullOptions{Cursor: 9, Batch: 10, PollInterval: 5 * time.Millisecond}, time.Second)
	if assert.Error(t, err) {
		assert.True(t, errors.Is(err, ErrSeqHole), "应保留 ErrSeqHole")
		assert.Contains(t, err.Error(), "seq hole")
	}
	assert.Empty(t, w.markers)
}

func TestRunPullDelSeq(t *testing.T) {
	src := &fakeSource{last: 11, blocks: map[int64]SeqBlock{11: seqBlock(11, 1010, schema.SeqTypeDel)}}
	w := &fakeWriter{}
	d := &fakeDealer{}
	err := startPull(t, src, w, d, PullOptions{Cursor: 10, Batch: 10, PollInterval: 5 * time.Millisecond}, 120*time.Millisecond)
	assert.NoError(t, err)
	if assert.Len(t, w.markers, 1) {
		assert.Equal(t, schema.SeqTypeDel, w.markers[0].Type)
		assert.Equal(t, int64(11), w.markers[0].Seq, "del seq 也推进游标")
	}
}

func TestRunPullRetriesTransientLastSeqError(t *testing.T) {
	src := &fakeSource{last: 10, blocks: blocksFor(10, 10)}
	src.lastErr = errors.New("rpc boom")
	w := &fakeWriter{}
	d := &fakeDealer{}
	err := startPull(t, src, w, d, PullOptions{Cursor: 9, Batch: 10, PollInterval: 5 * time.Millisecond}, 200*time.Millisecond)
	assert.NoError(t, err)
	assert.Len(t, w.markers, 1, "瞬时错误后应重试并处理")
}

func TestRunPullPruneMarkers(t *testing.T) {
	src := &fakeSource{last: 12, blocks: blocksFor(10, 12)}
	w := &fakeWriter{}
	d := &fakeDealer{}
	err := startPull(t, src, w, d, PullOptions{
		Cursor: 9, Batch: 10, PollInterval: 5 * time.Millisecond,
		MarkerKeep: 1, PruneEvery: 2,
	}, 120*time.Millisecond)
	assert.NoError(t, err)
	// 启动时先按当前游标清一次（cursor=9, keep=1 → before=8），推进到 PruneEvery 再清一次（cursor=11 → before=10）
	if assert.NotEmpty(t, w.pruned, "推进达到 PruneEvery 后应滚动删除") {
		assert.Equal(t, int64(8), w.pruned[0], "启动时按游标清理")
		assert.Equal(t, int64(10), w.pruned[len(w.pruned)-1], "删除边界 = cursor - MarkerKeep")
	}
}

// del seq 缺 detail（块体被 prune）：缓存命中即可回滚
func TestRunPullDelDetailFromCache(t *testing.T) {
	b := seqBlock(11, 1010, schema.SeqTypeDel)
	b.Detail = nil
	src := &fakeSource{last: 11, blocks: map[int64]SeqBlock{11: b}}
	cache := NewDetailCache(4)
	cache.Put(b.Hash, &types.BlockDetail{Block: &types.Block{Height: 1010}})
	w := &fakeWriter{}
	d := &fakeDealer{}

	err := startPullWithCache(t, src, w, d, cache, PullOptions{Cursor: 10, Batch: 10, PollInterval: 5 * time.Millisecond}, 120*time.Millisecond)
	assert.NoError(t, err)
	assert.Equal(t, 1, d.called, "缓存命中后应能继续处理")
	if assert.Len(t, w.markers, 1) {
		assert.Equal(t, schema.SeqTypeDel, w.markers[0].Type)
	}
}

// 缓存未命中 → 按 hash 向节点兜底
func TestRunPullDelDetailFromNode(t *testing.T) {
	b := seqBlock(11, 1010, schema.SeqTypeDel)
	b.Detail = nil
	src := &fakeSource{last: 11, blocks: map[int64]SeqBlock{11: b}}
	src.byHash = map[string]*types.BlockDetail{b.Hash: {Block: &types.Block{Height: 1010}}}
	w := &fakeWriter{}
	d := &fakeDealer{}

	err := startPull(t, src, w, d, PullOptions{Cursor: 10, Batch: 10, PollInterval: 5 * time.Millisecond}, 120*time.Millisecond)
	assert.NoError(t, err)
	assert.Equal(t, 1, d.called)
	assert.Len(t, w.markers, 1)
}

// 缓存与节点都没有 → 明确失败、不推进游标
func TestRunPullDelDetailMissing(t *testing.T) {
	b := seqBlock(11, 1010, schema.SeqTypeDel)
	b.Detail = nil
	src := &fakeSource{last: 11, blocks: map[int64]SeqBlock{11: b}}
	w := &fakeWriter{}

	err := startPull(t, src, w, &fakeDealer{}, PullOptions{Cursor: 10, Batch: 10, PollInterval: 5 * time.Millisecond}, time.Second)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "无 block detail")
		assert.Contains(t, err.Error(), "无法回滚")
	}
	assert.Empty(t, w.markers, "拿不到旧块 detail 不得推进游标")
}

// fix 模式：处理到 -to（含）就退出，且不写 marker
func TestRunPullFixModeStopsAtEndSeq(t *testing.T) {
	src := &fakeSource{last: 20, blocks: blocksFor(10, 20)}
	w := &fakeWriter{}
	d := &fakeDealer{}
	err := startPull(t, src, w, d,
		PullOptions{Cursor: 9, Batch: 4, PollInterval: 5 * time.Millisecond, FixMode: true, EndSeq: 14, SkipMarker: true},
		2*time.Second)
	assert.NoError(t, err, "修完区间应正常退出，而不是等 ctx 超时")
	assert.Equal(t, 5, d.called, "应处理 10..14 共 5 个 seq")
	assert.Empty(t, w.markers, "fix 模式不得写 seq_marker")
	assert.Len(t, w.records, 5)
}

// fix 模式：-to 超出节点 seq 日志 → 直接报错，不空转
func TestRunPullFixModeBeyondNodeTip(t *testing.T) {
	src := &fakeSource{last: 12, blocks: blocksFor(10, 12)}
	w := &fakeWriter{}
	err := startPull(t, src, w, &fakeDealer{},
		PullOptions{Cursor: 9, Batch: 10, PollInterval: 5 * time.Millisecond, FixMode: true, EndSeq: 20, SkipMarker: true},
		500*time.Millisecond)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "小于 -to")
	}
	assert.Empty(t, w.markers)
}

// fix 模式：cursor 已在 -to 之后 → 立即成功退出
func TestRunPullFixModeAlreadyDone(t *testing.T) {
	src := &fakeSource{last: 20}
	w := &fakeWriter{}
	d := &fakeDealer{}
	err := startPull(t, src, w, d,
		PullOptions{Cursor: 15, Batch: 10, PollInterval: 5 * time.Millisecond, FixMode: true, EndSeq: 14, SkipMarker: true},
		500*time.Millisecond)
	assert.NoError(t, err)
	assert.Equal(t, 0, d.called)
}
