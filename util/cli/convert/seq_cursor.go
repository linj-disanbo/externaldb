package convert

import (
	"github.com/pkg/errors"

	"github.com/33cn/externaldb/db/seqmark"
)

// CursorSources 决定起点所需的各路信息
type CursorSources struct {
	// MarkerSeq >=0 表示 ES 里已有 seq_marker；Marker 为其内容（用于与节点比对）
	MarkerSeq int64
	Marker    *seqmark.Marker
	// OldSeq 旧 convert 的 db05_last_seq（doc id = [convert].appName），<0 表示没有
	OldSeq int64
	// StartSeq 配置 [convert].startSeq
	StartSeq int64
	// SaveMarker 自举时写入 marker（refresh=wait_for）
	SaveMarker func(*seqmark.Marker) error
}

// ResolveCursor 计算起始游标（已处理完成的最后一个 seq）。
//
// 优先级：显式 fromSeq → 已有 marker（与节点比对一致） → 旧 last_seq 自举 → startSeq-1。
// marker 一致性与自举都要读节点同 seq 的记录，不一致一律显式失败（不猜）。
func ResolveCursor(src SeqSource, s CursorSources, fromSeq int64, warn func(msg string, ctx ...interface{})) (int64, int64, error) {
	if warn == nil {
		warn = func(string, ...interface{}) {}
	}

	if fromSeq >= 0 {
		warn("忽略 marker/旧进度，使用 -from 指定的起点", "from", fromSeq)
		if bs, err := src.Batch(fromSeq, fromSeq); err == nil && len(bs) == 1 {
			return fromSeq, bs[0].Height, nil
		}
		return fromSeq, -1, nil // 高度未知 → 统计只能按全局最新恢复
	}

	if s.MarkerSeq >= 0 && s.Marker != nil {
		bs, err := src.Batch(s.MarkerSeq, s.MarkerSeq)
		if err != nil {
			return 0, 0, errors.Wrapf(err, "校验游标失败：读取 seq(%d) 出错", s.MarkerSeq)
		}
		if len(bs) != 1 {
			return 0, 0, errors.Errorf("校验游标失败：marker 的 seq(%d) 在节点 seq 日志中不存在", s.MarkerSeq)
		}
		if bs[0].Hash != s.Marker.Hash || bs[0].Type != s.Marker.Type {
			return 0, 0, errors.Errorf(
				"游标与节点不一致：seq=%d marker(hash=%s,type=%d) node(hash=%s,type=%d)。"+
					"可能换了节点或节点 seq 日志被重建，请人工确认后再处理",
				s.MarkerSeq, s.Marker.Hash, s.Marker.Type, bs[0].Hash, bs[0].Type)
		}
		return s.MarkerSeq, bs[0].Height, nil
	}

	// 兼容切换：没有 marker 但旧 convert 有进度 → 按旧进度自举一条 marker，
	// 避免切到 pull_convert 时从 startSeq 重跑整条链。
	if s.OldSeq >= 0 {
		bs, err := src.Batch(s.OldSeq, s.OldSeq)
		if err != nil {
			return 0, 0, errors.Wrapf(err, "自举失败：读取旧进度 seq(%d) 出错", s.OldSeq)
		}
		if len(bs) != 1 {
			return 0, 0, errors.Errorf("自举失败：旧进度 seq(%d) 在节点 seq 日志中不存在，请人工确认（或用 -from 指定起点）", s.OldSeq)
		}
		m := &seqmark.Marker{Seq: s.OldSeq, Height: bs[0].Height, Hash: bs[0].Hash, Type: bs[0].Type}
		if s.SaveMarker != nil {
			if err := s.SaveMarker(m); err != nil {
				return 0, 0, errors.Wrapf(err, "自举失败：写 marker seq(%d)", s.OldSeq)
			}
		}
		warn("未找到 seq_marker，已按旧 db05_last_seq 自举", "seq", s.OldSeq, "height", bs[0].Height)
		return s.OldSeq, bs[0].Height, nil
	}

	cursor := s.StartSeq - 1
	warn("无 seq_marker、无旧进度，按配置 startSeq 起跑", "startSeq", s.StartSeq, "cursor", cursor)
	return cursor, -1, nil // 高度未知
}
