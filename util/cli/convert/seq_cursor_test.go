package convert

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/33cn/externaldb/db"
	"github.com/33cn/externaldb/db/seqmark"
)

func cursorSource() *fakeSource {
	return &fakeSource{last: 12, blocks: blocksFor(10, 12)}
}

func noWarn(string, ...interface{}) {}

func TestResolveCursorFromFlag(t *testing.T) {
	src := cursorSource()
	// 显式 -from 优先，不校验 marker
	got, h, err := ResolveCursor(src, CursorSources{MarkerSeq: 10, Marker: &seqmark.Marker{Seq: 10, Hash: "nope", Type: db.SeqTypeAdd}, StartSeq: 0}, 10, noWarn)
	assert.NoError(t, err)
	assert.Equal(t, int64(10), got, "-from 优先，不校验 marker")
	assert.Equal(t, int64(1010), h, "-from 时顺带取该 seq 的高度")

	// -from 指向节点上不存在的 seq → 高度未知（-1），统计只按全局最新恢复
	got, h, err = ResolveCursor(src, CursorSources{StartSeq: 0}, 999, noWarn)
	assert.NoError(t, err)
	assert.Equal(t, int64(999), got)
	assert.Equal(t, int64(-1), h)
}

func TestResolveCursorMarkerConsistent(t *testing.T) {
	src := cursorSource()
	b := src.blocks[11]
	got, h, err := ResolveCursor(src, CursorSources{
		MarkerSeq: 11,
		Marker:    &seqmark.Marker{Seq: 11, Height: b.Height, Hash: b.Hash, Type: b.Type},
		StartSeq:  0,
	}, -1, noWarn)
	assert.NoError(t, err)
	assert.Equal(t, int64(11), got)
	assert.Equal(t, b.Height, h, "marker 的高度即为恢复点")
}

func TestResolveCursorMarkerMismatch(t *testing.T) {
	src := cursorSource()
	_, _, err := ResolveCursor(src, CursorSources{
		MarkerSeq: 11,
		Marker:    &seqmark.Marker{Seq: 11, Hash: "0xother", Type: db.SeqTypeAdd},
	}, -1, noWarn)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "游标与节点不一致")
	}
}

func TestResolveCursorMarkerSeqMissing(t *testing.T) {
	src := cursorSource()
	delete(src.blocks, 11)
	_, _, err := ResolveCursor(src, CursorSources{
		MarkerSeq: 11,
		Marker:    &seqmark.Marker{Seq: 11, Hash: "0xa", Type: db.SeqTypeAdd},
	}, -1, noWarn)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "不存在")
	}
}

func TestResolveCursorBootstrapFromOldSeq(t *testing.T) {
	src := cursorSource()
	b := src.blocks[10]
	var saved []*seqmark.Marker
	got, h, err := ResolveCursor(src, CursorSources{
		MarkerSeq:  -1,
		OldSeq:     10,
		StartSeq:   0,
		SaveMarker: func(m *seqmark.Marker) error { saved = append(saved, m); return nil },
	}, -1, noWarn)
	assert.NoError(t, err)
	assert.Equal(t, int64(10), got)
	assert.Equal(t, src.blocks[10].Height, h)
	if assert.Len(t, saved, 1, "应自举一条 marker") {
		assert.Equal(t, int64(10), saved[0].Seq)
		assert.Equal(t, b.Height, saved[0].Height)
		assert.Equal(t, b.Hash, saved[0].Hash)
		assert.Equal(t, b.Type, saved[0].Type)
	}
}

func TestResolveCursorBootstrapOldSeqMissingInNode(t *testing.T) {
	src := cursorSource()
	delete(src.blocks, 10)
	_, _, err := ResolveCursor(src, CursorSources{MarkerSeq: -1, OldSeq: 10}, -1, noWarn)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "自举失败")
	}
}

func TestResolveCursorFallbackStartSeq(t *testing.T) {
	src := cursorSource()
	got, h, err := ResolveCursor(src, CursorSources{MarkerSeq: -1, OldSeq: -1, StartSeq: 5}, -1, noWarn)
	assert.NoError(t, err)
	assert.Equal(t, int64(4), got, "startSeq-1")
	assert.Equal(t, int64(-1), h, "无任何进度时高度未知")
}

// 自举用的 seq 是 Add 还是 Del 都要原样写入 marker（回滚 seq 也能当游标）
func TestResolveCursorBootstrapDelSeq(t *testing.T) {
	src := cursorSource()
	src.blocks[11] = seqBlock(11, 1011, db.SeqTypeDel)
	var saved *seqmark.Marker
	_, h, err := ResolveCursor(src, CursorSources{
		MarkerSeq:  -1,
		OldSeq:     11,
		SaveMarker: func(m *seqmark.Marker) error { saved = m; return nil },
	}, -1, noWarn)
	assert.NoError(t, err)
	assert.Equal(t, int64(1011), h, "回滚 seq 也能作为恢复点（高度取该 seq 的块）")
	if assert.NotNil(t, saved) {
		assert.Equal(t, db.SeqTypeDel, saved.Type)
	}
}
