package main

import (
	"github.com/33cn/externaldb/lib/esdoc/blockinfo"
	"github.com/33cn/externaldb/lib/esdoc/seqmark"
)

// seqRange 缺号区间（闭区间）
type seqRange struct {
	From, To int64
}

// checkContinuity 找出 seq 序列里不连续的区间。
// seqs 必须按 seq 递增（跨页调用时用 last 传入上一页的最后一个 seq）。
func checkContinuity(seqs []int64, last int64) []seqRange {
	var gaps []seqRange
	prev := last
	for _, s := range seqs {
		if prev >= 0 && s != prev+1 {
			gaps = append(gaps, seqRange{From: prev + 1, To: s - 1})
		}
		prev = s
	}
	return gaps
}

// markerDiff 对账结果
type markerDiff struct {
	Missing  []*seqmark.Marker // block_info 文档缺失
	Mismatch []*seqmark.Marker // block_info 存在但 height/hash 不一致
}

// diffBlockInfo 比对 marker 与 block_info（bi 里 nil 表示该 hash 的文档不存在）。
// 正常情况下 marker.Seq 对应块的 hash 就是 block_info 的 doc id，height 也应一致。
func diffBlockInfo(ms []*seqmark.Marker, bi map[string]*blockinfo.BlockInfo) markerDiff {
	var d markerDiff
	for _, m := range ms {
		info := bi[m.Hash]
		if info == nil {
			d.Missing = append(d.Missing, m)
			continue
		}
		if info.Height != m.Height || info.Hash != m.Hash {
			d.Mismatch = append(d.Mismatch, m)
		}
	}
	return d
}

// preview 截断用于日志打印
func preview[T any](s []T, n int) []T {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
