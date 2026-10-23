package pipeline

import (
	"testing"
	"time"

	"github.com/33cn/externaldb/lib/schema"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

// ---------- 分片 ----------

// sizeRecord 指定 Value() 长度的假记录，便于精确控制分片
type sizeRecord struct {
	*fakeRecord
	n int
}

func (r *sizeRecord) Value() []byte { return make([]byte, r.n) }

func sized(id string, n int) schema.Record {
	return &sizeRecord{fakeRecord: newFakeRecord("idx", id), n: n}
}

func TestChunkRecords(t *testing.T) {
	// 空输入
	assert.Empty(t, chunkRecords(nil, 100, 10))

	// 按字节切：每条 10 字节、上限 25 → 每片 2 条
	rs := []schema.Record{sized("a", 10), sized("b", 10), sized("c", 10), sized("d", 10), sized("e", 10)}
	chunks := chunkRecords(rs, 25, 100)
	assert.Len(t, chunks, 3)
	assert.Len(t, chunks[0], 2)
	assert.Len(t, chunks[1], 2)
	assert.Len(t, chunks[2], 1)

	// 顺序与完整性
	var got []string
	for _, c := range chunks {
		for _, r := range c {
			got = append(got, r.ID())
		}
	}
	assert.Equal(t, []string{"a", "b", "c", "d", "e"}, got)

	// 按条数切
	chunks = chunkRecords(rs, 1<<20, 2)
	assert.Len(t, chunks, 3)
	assert.Len(t, chunks[0], 2)
	assert.Len(t, chunks[2], 1)

	// 单条超过上限时自己成一片，不报错也不丢
	chunks = chunkRecords([]schema.Record{sized("big", 100), sized("x", 1)}, 10, 10)
	assert.Len(t, chunks, 2)
	assert.Equal(t, "big", chunks[0][0].ID())
}

// ---------- 分片写入 + 失败重试 ----------

// callResult 模拟某一次 _bulk 的行为：
//   - err 非 nil：本次返回错误；written 为该次"实际写入成功"的 id（部分失败时给成功的那些）
//   - err 为 nil：本次成功，written 为空表示全部写入
type callResult struct {
	err     error
	written []string
}

type fakeChunkBulk struct {
	results   []callResult // 第 i 次调用的行为；超出则成功且全部写入
	filterIDs []string     // 模拟 escli.FilterFailed：错误时挑出的失败 id
	calls     int
	sizes     []int
	ids       []string
}

func (f *fakeChunkBulk) bulk(rs []schema.Record) error {
	f.calls++
	f.sizes = append(f.sizes, len(rs))
	var res callResult
	if f.calls <= len(f.results) {
		res = f.results[f.calls-1]
	}
	switch {
	case res.err == nil && res.written == nil:
		// 成功且未指定子集：全部写入
		for _, r := range rs {
			f.ids = append(f.ids, r.ID())
		}
	case res.written != nil:
		// 部分失败：只有 written 里的写进去了
		f.ids = append(f.ids, res.written...)
	default:
		// 请求被拒/整体失败：什么都没写
	}
	return res.err
}

func (f *fakeChunkBulk) filter(rs []schema.Record, err error) []schema.Record {
	if err == nil || len(f.filterIDs) == 0 {
		return nil
	}
	want := map[string]bool{}
	for _, id := range f.filterIDs {
		want[id] = true
	}
	var out []schema.Record
	for _, r := range rs {
		if want[r.ID()] {
			out = append(out, r)
		}
	}
	return out
}

var err429 = errors.New("elastic: Error 429 (Too Many Requests): es_rejected_execution_exception")

func newTestWriter() *ESWriter {
	w := NewESWriter(nil)
	w.bulkBytes = 25
	w.bulkCount = 100
	w.retrySleep = time.Millisecond
	return w
}

func TestWriteRecordsChunked(t *testing.T) {
	w := newTestWriter()
	f := &fakeChunkBulk{}
	rs := []schema.Record{sized("a", 10), sized("b", 10), sized("c", 10), sized("d", 10), sized("e", 10)}

	assert.NoError(t, w.write(rs, f.bulk, f.filter))
	assert.Equal(t, 3, f.calls, "5 条 × 10 字节 / 每片 25 字节 = 3 片")
	assert.Equal(t, []int{2, 2, 1}, f.sizes)
	assert.Equal(t, []string{"a", "b", "c", "d", "e"}, f.ids)
}

func TestWriteRecordsRetriesRejectedChunk(t *testing.T) {
	w := newTestWriter()
	w.bulkBytes = 1 << 20 // 单分片
	// 前两次被 ES 拒绝（429，请求未执行），第三次成功 → 整块退避重试
	f := &fakeChunkBulk{results: []callResult{{err: err429}, {err: err429}}}
	rs := []schema.Record{sized("a", 10), sized("b", 10), sized("c", 10)}

	assert.NoError(t, w.write(rs, f.bulk, f.filter))
	assert.Equal(t, 3, f.calls, "1 次 + 2 次整块重试")
	assert.Equal(t, []string{"a", "b", "c"}, f.ids, "被拒的两次不应写入任何记录")
}

func TestWriteRecordsRejectedGivesUp(t *testing.T) {
	w := newTestWriter()
	w.bulkBytes = 1 << 20 // 单分片
	w.transportRetries = 2
	f := &fakeChunkBulk{results: []callResult{{err: err429}, {err: err429}, {err: err429}}}
	rs := []schema.Record{sized("a", 10), sized("b", 10), sized("c", 10)}

	err := w.write(rs, f.bulk, f.filter)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "still rejected after 2 retries")
	}
	assert.Equal(t, 3, f.calls, "初次 + 2 次重试后放弃")
	assert.Empty(t, f.ids, "被拒的请求不应写入任何记录")
}

func TestWriteRecordsNonRetryableErrorFailsFast(t *testing.T) {
	w := newTestWriter()
	w.bulkBytes = 1 << 20 // 单分片
	f := &fakeChunkBulk{results: []callResult{{err: errors.New("mapper_parsing_exception: failed to parse")}}}
	rs := []schema.Record{sized("a", 10), sized("b", 10), sized("c", 10)}

	err := w.write(rs, f.bulk, f.filter)
	if assert.Error(t, err) {
		assert.NotContains(t, err.Error(), "still rejected")
	}
	assert.Equal(t, 1, f.calls, "不可重试错误立即失败")
}

// 片内部分失败：只重试失败项（不整块重发）
func TestWriteRecordsPartialFailureRetriesOnlyFailed(t *testing.T) {
	w := newTestWriter()
	w.bulkBytes = 1 << 20 // 一片
	f := &fakeChunkBulk{
		results:   []callResult{{err: errors.New("bulk failed: [b]"), written: []string{"a", "c"}}},
		filterIDs: []string{"b"},
	}
	rs := []schema.Record{sized("a", 1), sized("b", 1), sized("c", 1)}

	assert.NoError(t, w.write(rs, f.bulk, f.filter))
	assert.Equal(t, 2, f.calls, "整批 1 次 + 只重试 b 1 次")
	assert.Equal(t, []string{"a", "c", "b"}, f.ids)
}

func TestWriteRecordsChunkFailureStopsLaterChunks(t *testing.T) {
	w := newTestWriter()
	// 第 2 片失败（不可重试）→ 第 3 片不应再写
	f := &fakeChunkBulk{results: []callResult{{}, {err: errors.New("mapper_parsing_exception")}}}
	rs := []schema.Record{sized("a", 10), sized("b", 10), sized("c", 10), sized("d", 10), sized("e", 10)}

	err := w.write(rs, f.bulk, f.filter)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "chunk 2/3")
	}
	assert.Equal(t, 2, f.calls)
	assert.Equal(t, []string{"a", "b"}, f.ids)
}

func TestIsRetryableWriteErr(t *testing.T) {
	assert.True(t, isRetryableWriteErr(err429))
	assert.True(t, isRetryableWriteErr(errors.New("... circuit_breaking_exception ...")))
	assert.True(t, isRetryableWriteErr(errors.New("elastic: Error 503 (Service Unavailable)")))
	assert.True(t, isRetryableWriteErr(errors.New("dial tcp: connect: connection refused")))
	// 可能已部分执行，不能整块重发
	assert.False(t, isRetryableWriteErr(errors.New("net/http: request canceled (Client.Timeout exceeded)")))
	assert.False(t, isRetryableWriteErr(errors.New("unexpected EOF")))
	assert.False(t, isRetryableWriteErr(nil))
}

// ESWriter 必须满足 SeqWriter
var _ SeqWriter = (*ESWriter)(nil)
