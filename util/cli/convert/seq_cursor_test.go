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
	got, err := ResolveCursor(src, CursorSources{MarkerSeq: 10, Marker: &seqmark.Marker{Seq: 10, Hash: "nope", Type: db.SeqTypeAdd}, StartSeq: 0}, 7, noWarn)
	assert.NoError(t, err)
	assert.Equal(t, int64(7), got)
}

func TestResolveCursorMarkerConsistent(t *testing.T) {
	src := cursorSource()
	b := src.blocks[11]
	got, err := ResolveCursor(src, CursorSources{
		MarkerSeq: 11,
		Marker:    &seqmark.Marker{Seq: 11, Height: b.Height, Hash: b.Hash, Type: b.Type},
		StartSeq:  0,
	}, -1, noWarn)
	assert.NoError(t, err)
	assert.Equal(t, int64(11), got)
}

func TestResolveCursorMarkerMismatch(t *testing.T) {
	src := cursorSource()
	_, err := ResolveCursor(src, CursorSources{
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
	_, err := ResolveCursor(src, CursorSources{
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
	got, err := ResolveCursor(src, CursorSources{
		MarkerSeq:  -1,
		OldSeq:     10,
		StartSeq:   0,
		SaveMarker: func(m *seqmark.Marker) error { saved = append(saved, m); return nil },
	}, -1, noWarn)
	assert.NoError(t, err)
	assert.Equal(t, int64(10), got)
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
	_, err := ResolveCursor(src, CursorSources{MarkerSeq: -1, OldSeq: 10}, -1, noWarn)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "自举失败")
	}
}

func TestResolveCursorFallbackStartSeq(t *testing.T) {
	src := cursorSource()
	got, err := ResolveCursor(src, CursorSources{MarkerSeq: -1, OldSeq: -1, StartSeq: 5}, -1, noWarn)
	assert.NoError(t, err)
	assert.Equal(t, int64(4), got, "startSeq-1")
}

// 自举用的 seq 是 Add 还是 Del 都要原样写入 marker（回滚 seq 也能当游标）
func TestResolveCursorBootstrapDelSeq(t *testing.T) {
	src := cursorSource()
	src.blocks[11] = seqBlock(11, 1011, db.SeqTypeDel)
	var saved *seqmark.Marker
	_, err := ResolveCursor(src, CursorSources{
		MarkerSeq:  -1,
		OldSeq:     11,
		SaveMarker: func(m *seqmark.Marker) error { saved = m; return nil },
	}, -1, noWarn)
	assert.NoError(t, err)
	if assert.NotNil(t, saved) {
		assert.Equal(t, db.SeqTypeDel, saved.Type)
	}
}
