package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/33cn/externaldb/lib/esdoc/blockinfo"
	"github.com/33cn/externaldb/lib/esdoc/seqmark"
	"github.com/33cn/externaldb/lib/schema"
)

func TestCheckContinuity(t *testing.T) {
	// 连续
	assert.Empty(t, checkContinuity([]int64{1, 2, 3}, -1))
	// 首页不从 0 开始不算缺口（-1 表示还没有上一页）
	assert.Empty(t, checkContinuity([]int64{10, 11}, -1))
	// 中间缺号
	gaps := checkContinuity([]int64{1, 2, 5, 6, 9}, -1)
	assert.Equal(t, []seqRange{{From: 3, To: 4}, {From: 7, To: 8}}, gaps)
	// 跨页：上一页最后是 5，本页从 7 开始
	gaps = checkContinuity([]int64{7, 8}, 5)
	assert.Equal(t, []seqRange{{From: 6, To: 6}}, gaps)
	// 空页
	assert.Empty(t, checkContinuity(nil, 5))
}

func TestDiffBlockInfo(t *testing.T) {
	m := func(seq, height int64, hash string) *seqmark.Marker {
		return &seqmark.Marker{Seq: seq, Height: height, Hash: hash, Type: schema.SeqTypeAdd}
	}
	ms := []*seqmark.Marker{
		m(1, 101, "h1"),
		m(2, 102, "h2"),
		m(3, 103, "h3"),
	}
	bi := map[string]*blockinfo.BlockInfo{
		"h1": {Height: 101, Hash: "h1"},
		// h2 缺失
		"h3": {Height: 999, Hash: "h3"}, // 高度不一致
	}
	d := diffBlockInfo(ms, bi)
	if assert.Len(t, d.Missing, 1) {
		assert.Equal(t, int64(2), d.Missing[0].Seq)
	}
	if assert.Len(t, d.Mismatch, 1) {
		assert.Equal(t, int64(3), d.Mismatch[0].Seq)
	}

	// 全部一致
	bi["h2"] = &blockinfo.BlockInfo{Height: 102, Hash: "h2"}
	bi["h3"] = &blockinfo.BlockInfo{Height: 103, Hash: "h3"}
	d = diffBlockInfo(ms, bi)
	assert.Empty(t, d.Missing)
	assert.Empty(t, d.Mismatch)
}

func TestPreview(t *testing.T) {
	assert.Equal(t, []int{1, 2}, preview([]int{1, 2}, 5))
	assert.Equal(t, []int{1, 2}, preview([]int{1, 2, 3, 4}, 2))
	assert.Empty(t, preview([]int{}, 3))
}
