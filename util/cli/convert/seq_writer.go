package convert

import (
	"github.com/pkg/errors"

	"github.com/33cn/externaldb/db"
	"github.com/33cn/externaldb/db/seqmark"
	"github.com/33cn/externaldb/escli"
)

// MaxWriteRetry 单个 seq 的记录写入重试次数（每次只重试失败项）
const MaxWriteRetry = 3

// SeqWriter 直连路径的落库与游标接口（便于单测注入 fake）
type SeqWriter interface {
	// WriteRecords 写一个 seq 的记录；内部只重试失败项，全部成功才返回 nil
	WriteRecords(rs []db.Record) error
	// WriteMarker 写 seq marker（refresh=wait_for），返回即可被 search 到
	WriteMarker(m *seqmark.Marker) error
	// PruneMarkers 滚动删除 before 之前的旧 marker
	PruneMarkers(before int64) error
}

// ESWriter SeqWriter 的 ES 实现
type ESWriter struct {
	cli     escli.ESClient
	retries int
}

// NewESWriter 创建 ESWriter
func NewESWriter(cli escli.ESClient) *ESWriter {
	return &ESWriter{cli: cli, retries: MaxWriteRetry}
}

// WriteRecords 写记录：失败时用 escli.FilterFailed 只重试失败项。
// 只重写失败项是幂等的（Record.Value 是绝对值）；整批重跑转换器会重复计数
// （db/address TxCount、block_stat 是读改写累计）。
func (w *ESWriter) WriteRecords(rs []db.Record) error {
	return saveWithRetry(w.cli.BulkUpdate, func(pending []db.Record, err error) []db.Record {
		return escli.FilterFailed(w.cli, pending, err)
	}, rs, w.retries)
}

// WriteMarker 写游标：必须 refresh=wait_for，否则刚写完就重启会读到未刷新的旧游标
func (w *ESWriter) WriteMarker(m *seqmark.Marker) error {
	if m == nil {
		return nil
	}
	return seqmark.Save(w.cli, []*seqmark.Marker{m})
}

// PruneMarkers 滚动删除旧 marker
func (w *ESWriter) PruneMarkers(before int64) error {
	return seqmark.DeleteBefore(w.cli, before)
}

// saveWithRetry 写 rs；失败时用 filter 挑出失败项重试。
// filter 返回空表示"无法定位失败项"→ 立即失败，交上层（不推进游标、不写 marker）。
func saveWithRetry(
	bulk func([]db.Record) error,
	filter func([]db.Record, error) []db.Record,
	rs []db.Record,
	retries int,
) error {
	if len(rs) == 0 {
		return nil
	}
	pending := rs
	var lastErr error
	for i := 0; i <= retries; i++ {
		lastErr = bulk(pending)
		if lastErr == nil {
			return nil
		}
		failed := filter(pending, lastErr)
		if len(failed) == 0 {
			return lastErr
		}
		pending = failed
	}
	return errors.Wrapf(lastErr, "write records: %d item(s) still failing after %d retries", len(pending), retries)
}
