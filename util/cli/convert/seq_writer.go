package convert

import (
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/33cn/externaldb/db"
	"github.com/33cn/externaldb/db/seqmark"
	"github.com/33cn/externaldb/escli"
)

const (
	// MaxWriteRetry 单个 _bulk 的记录写入重试次数（每次只重试失败项）
	MaxWriteRetry = 3
	// DefaultBulkBytes 单个 _bulk 请求的估算字节上限。
	// ES 的 coordination 断路器按"请求要协调的字节数"拒绝：堆 512m 时上限只有 ~51MB，
	// genesis 这种大 seq 一次发 60MB+ 会直接 429（es_rejected_execution_exception）。
	DefaultBulkBytes = 5 << 20
	// DefaultBulkCount 单个 _bulk 的记录条数上限（防止极小的记录堆成超大请求）
	DefaultBulkCount = 2000
	// MaxTransportRetry "请求未被 ES 执行"类错误（429 拒绝/503/circuit breaking）的整块重试次数
	MaxTransportRetry = 5
	// defaultRetrySleep 传输层重试的基础退避
	defaultRetrySleep = 200 * time.Millisecond
	maxRetrySleep     = 5 * time.Second
)

// retryableWriteErrTokens：这些错误说明 ES 在**执行前**就把请求拒了（没写任何文档），
// 因此整块重发是安全的。注意 timeout/EOF 等"可能已部分执行"的错误**不在**这里——
// 重发会让 db/address 的 TxCount、block_stat 这类读改写累计重复计数。
var retryableWriteErrTokens = []string{
	"429",
	"es_rejected_execution_exception",
	"circuit_breaking_exception",
	"503",
	"Unavailable",
	"connection refused",
}

// SeqWriter 直连路径的落库与游标接口（便于单测注入 fake）
type SeqWriter interface {
	// WriteRecords 写一个 seq 的记录；内部按大小分片、只重试失败项，全部成功才返回 nil
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
	// bulkBytes/bulkCount 单个 _bulk 的规模上限（genesis 之类的大 seq 必须分片）
	bulkBytes int
	bulkCount int
	// transportRetries 请求被拒（429/503 等，未执行）时的整块重试次数
	transportRetries int
	retrySleep       time.Duration
}

// NewESWriter 创建 ESWriter（默认分片 5MB / 2000 条）
func NewESWriter(cli escli.ESClient) *ESWriter {
	return &ESWriter{
		cli:              cli,
		retries:          MaxWriteRetry,
		bulkBytes:        DefaultBulkBytes,
		bulkCount:        DefaultBulkCount,
		transportRetries: MaxTransportRetry,
		retrySleep:       defaultRetrySleep,
	}
}

// WriteRecords 写记录：先按大小/条数分片，片内失败只重试失败项；
// 遇到 ES 明确拒绝（429/503，请求未执行）则整块退避重试。
// 只重写失败项是幂等的（Record.Value 是绝对值）；整批重跑转换器会重复计数
// （db/address TxCount、block_stat 是读改写累计）。
func (w *ESWriter) WriteRecords(rs []db.Record) error {
	return w.write(rs, w.cli.BulkUpdate, func(pending []db.Record, err error) []db.Record {
		return escli.FilterFailed(w.cli, pending, err)
	})
}

// write 分片 + 重试的实际实现（bulk/filter 可注入，便于单测）
func (w *ESWriter) write(rs []db.Record,
	bulk func([]db.Record) error,
	filter func([]db.Record, error) []db.Record) error {
	chunks := chunkRecords(rs, w.bulkBytes, w.bulkCount)
	for i, chunk := range chunks {
		if err := w.writeChunk(chunk, bulk, filter); err != nil {
			return errors.Wrapf(err, "chunk %d/%d (%d records)", i+1, len(chunks), len(chunk))
		}
	}
	return nil
}

// writeChunk 写一个分片：片内先只重试失败项，被 ES 拒绝则整块退避重试
func (w *ESWriter) writeChunk(chunk []db.Record,
	bulk func([]db.Record) error,
	filter func([]db.Record, error) []db.Record) error {
	var lastErr error
	sleep := w.retrySleep
	for i := 0; i <= w.transportRetries; i++ {
		err := saveWithRetry(bulk, filter, chunk, w.retries)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryableWriteErr(err) {
			return err
		}
		if i < w.transportRetries {
			if sleep > 0 {
				time.Sleep(sleep)
			}
			if sleep *= 2; sleep > maxRetrySleep {
				sleep = maxRetrySleep
			}
		}
	}
	return errors.Wrapf(lastErr, "still rejected after %d retries", w.transportRetries)
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

// chunkRecords 按估算字节数（len(Value())）与条数把记录切成分片，保持原顺序。
// 单条记录超过 maxBytes 时自己成一片（不拆单条）。
func chunkRecords(rs []db.Record, maxBytes, maxCount int) [][]db.Record {
	if maxBytes <= 0 {
		maxBytes = DefaultBulkBytes
	}
	if maxCount <= 0 {
		maxCount = DefaultBulkCount
	}
	var out [][]db.Record
	var cur []db.Record
	size := 0
	for _, r := range rs {
		n := len(r.Value())
		if len(cur) > 0 && (size+n > maxBytes || len(cur) >= maxCount) {
			out = append(out, cur)
			cur, size = nil, 0
		}
		cur = append(cur, r)
		size += n
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// isRetryableWriteErr 请求是否"未被执行"（可以安全整块重发）
func isRetryableWriteErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, t := range retryableWriteErrTokens {
		if strings.Contains(msg, t) {
			return true
		}
	}
	return false
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
