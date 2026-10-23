// Package seqmark 存 append-only 的 seq marker：convert 直连（pull + seq 驱动）的进度游标。
//
// 设计要点：
//   - 每个 seq 一条，doc id = seq（十进制），只写不删（可滚动删除旧窗口）；
//   - 游标 = max(seq)；marker 存在即代表该 seq 的数据已写入 → 重启不需要"重做最后一块"；
//   - del seq 也写 marker → 回滚后游标不回退；
//   - 缺号即空洞，可用于对账；
//   - 写入必须带 refresh=wait_for：游标是 search 出来的，否则刚写完就崩会读到旧值并重放。
package seqmark

import (
	"encoding/json"
	"strconv"

	"github.com/33cn/externaldb/escli"
	"github.com/33cn/externaldb/escli/querypara"
	"github.com/33cn/externaldb/lib/schema"
	"github.com/33cn/externaldb/util"
	"github.com/pkg/errors"
)

const (
	// TableName 逻辑索引名；物理名 = [convertEs] prefix + 这个名字
	TableName = "seq_marker"
)

// Marker 一个 seq 的处理标记
type Marker struct {
	Seq    int64  `json:"seq"`
	Height int64  `json:"height"`
	Hash   string `json:"hash"`
	Type   int    `json:"type"` // 1=Add, 2=Del（chain33 AddBlock/DelBlock，同 schema.SeqTypeAdd/Del）
}

// MarkerRecord Marker 的 ES 记录
type MarkerRecord struct {
	*schema.IKey
	*schema.Op
	Marker *Marker
}

// Value impl
func (r *MarkerRecord) Value() []byte {
	v, _ := json.Marshal(r.Marker)
	return v
}

// Key 索引 id = seq
func (m *Marker) Key() string {
	return strconv.FormatInt(m.Seq, 10)
}

// NewMarkerRecord create MarkerRecord
func NewMarkerRecord(m *Marker) *MarkerRecord {
	return &MarkerRecord{
		IKey:   schema.NewIKey(TableName, TableName, m.Key()),
		Op:     schema.NewOp(schema.OpAdd),
		Marker: m,
	}
}

// InitDB 建索引（mapping：seq 用 long 才能 sort/range）
func InitDB(cli schema.DBCreator) error {
	return util.InitIndex(cli, TableName, TableName, Mapping)
}

// Save 追加 marker，用普通 bulk（**不** 逐次 refresh=wait_for）。
//
// 为什么不 wait_for：wait_for 每次要等一个 refresh 周期（实测 ~1s），逐 seq 写会把
// 追块速度压到 ~1 块/秒。marker 只要保证"重启时能被 search 读到"——
// 启动时调用一次 Refresh() 即可（translog durability=request 下，ACK 即已落盘，重启不丢）。
func Save(cli escli.ESClient, ms []*Marker) error {
	if len(ms) == 0 {
		return nil
	}
	rs := make([]schema.Record, 0, len(ms))
	for _, m := range ms {
		rs = append(rs, NewMarkerRecord(m))
	}
	return cli.BulkUpdate(rs)
}

// Refresh 显式 refresh marker 索引。启动时、读游标（Max）之前调用一次，
// 保证刚写入但还未被 ES 自动 refresh 的 marker 对 search 可见。
func Refresh(cli escli.ESClient) error {
	return cli.RefreshIndex(TableName)
}

// Max 读取游标：返回最大 seq 及其 marker；没有任何 marker 时返回 (-1, nil, nil)。
func Max(cli escli.ESClient) (int64, *Marker, error) {
	q := &querypara.Query{
		Sort: []*querypara.QSort{{Key: "seq", Ascending: false}},
		Size: &querypara.QSize{Size: 1},
	}
	decode := func(x *json.RawMessage) (interface{}, error) {
		var m Marker
		if err := json.Unmarshal(*x, &m); err != nil {
			return nil, err
		}
		return &m, nil
	}
	rs, err := cli.Search(TableName, TableName, q, decode)
	if err != nil {
		return -1, nil, errors.Wrap(err, "seqmark: query max seq")
	}
	if len(rs) == 0 {
		return -1, nil, nil
	}
	m, ok := rs[0].(*Marker)
	if !ok || m == nil {
		return -1, nil, errors.New("seqmark: unexpected decode result")
	}
	return m.Seq, m, nil
}

// DeleteBefore 滚动删除 seq < before 的旧 marker（保留窗口要覆盖最长停机时间）。
func DeleteBefore(cli escli.ESClient, before int64) error {
	if before <= 0 {
		return nil
	}
	q := &querypara.Query{
		Range: []*querypara.QRange{{Key: "seq", LT: before}},
	}
	return cli.DeleteByQuery(TableName, TableName, q)
}
