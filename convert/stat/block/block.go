package block

import (
	"encoding/json"
	"fmt"

	l "github.com/33cn/chain33/common/log/log15"
	"github.com/33cn/chain33/types"
	"github.com/33cn/externaldb/convert/stat"
	"github.com/33cn/externaldb/lib/escli"
	"github.com/33cn/externaldb/lib/escli/querypara"
	esmaint "github.com/33cn/externaldb/lib/esmaint"
	"github.com/33cn/externaldb/lib/schema"
	"github.com/pkg/errors"
)

// db
const (
	DBStatX     = "block_stat"
	NameX       = "block"
	DefaultType = "_doc"
)

const (
	coin = 100000000
)

var log = l.New("module", "stat.block")

// BStat 统计从0高度的累计值
type BStat struct {
	Height  int64 `json:"height"`
	Time    int64 `json:"time"`
	TxCount int   `json:"tx_count"`
	Fee     int64 `json:"fee"`
	Mine    int64 `json:"mine"`
	Coins   int64 `json:"coins"`
}

// 记录以往的累计
var (
	prev *BStat
)

func init() {
	stat.Register(NameX, NewStat)
}

// ID id
func (s *BStat) ID() string {
	return fmt.Sprintf("stat-%d", s.Height)
}

// NewStat NewStat
func NewStat(title, symbol string, genesis, coin int64) stat.Stat {
	if title != "bityuan" {
		return &blockStat{
			title:   title,
			genesis: genesis,
			coin:    coin,
		}
	}
	return &blockStat{title: title}
}

type blockStat struct {
	title   string
	genesis int64
	coin    int64
}

// InitDB init db
func (s *blockStat) InitDB(cli schema.DBCreator) error {
	err := esmaint.InitIndex(cli, DBStatX, DBStatX, StatMapping)
	return err
}

// Recover 兼容旧调用（只按 seq，无高度信息）：等价于"不设上界"的 RecoverAt。
// 注意：它取的是全局最新一条，**不能**保证与恢复点对齐；能拿到高度的调用方请用 RecoverAt。
func (s *blockStat) Recover(client escli.ESClient, lastSeq int64) error {
	return s.RecoverAt(client, -1)
}

// RecoverAt 恢复累计值 prev：取 height <= 指定高度的最新一条 block_stat。
// height < 0 表示不设上界（取全局最新）。
//
// 语义（见 stat.HeightRecover）：
//   - 正常续跑：游标块高度 H 已处理完 → 取 stat-H（下一次处理的是 H+1，prev 需要"累计到 H"）；
//   - 崩溃重放：游标是 H-1、但 stat-H 已写 → 上界 H-1 会把 prev 拉回"累计到 H-1"，
//     重放 H 时算出同一个 stat-H（幂等），不会重复累加；
//   - 回滚：游标 seq 是高度 H 的 Del（stat-H 已被删掉）→ 取到的必然是 stat-(H-1)，正确。
func (s *blockStat) RecoverAt(client escli.ESClient, height int64) error {
	if prev != nil && (height < 0 || prev.Height == height) {
		return nil
	}
	q := querypara.Query{
		Page: &querypara.QPage{Size: 1, Number: 1},
		Sort: []*querypara.QSort{{Key: "height", Ascending: false}},
	}
	if height >= 0 {
		q.Range = []*querypara.QRange{{Key: "height", REnd: height}}
	}
	rs, err := client.Search(DBStatX, DBStatX, &q, decode)
	if err != nil {
		return errors.Wrapf(err, "block stat recover search")
	}
	if len(rs) == 0 {
		log.Warn("block stat 没有可用的累计记录（<= height）", "height", height)
		return nil
	}
	st, ok := rs[0].(*BStat)
	if !ok {
		return errors.New("block stat recover get value")
	}
	prev = st
	if height >= 0 && st.Height != height {
		// 缺块（例如中途才开统计）会导致累计值基座偏早；这里显式告警，不再静默算错
		log.Warn("block stat 恢复点与游标不一致", "recoveredHeight", st.Height, "expectedHeight", height)
	}
	log.Info("block stat recovered", "height", st.Height, "txCount", st.TxCount, "coins", st.Coins)
	return nil
}

func decode(x *json.RawMessage) (interface{}, error) {
	s := BStat{}
	err := json.Unmarshal([]byte(*x), &s)
	return &s, err
}

// Stat impl
func (s *blockStat) Stat(detail *types.BlockDetail, op int) ([]schema.Record, error) {
	cur := statCur(s.title, detail, s.genesis, s.coin)

	if cur.Height == 0 {
		prev = cur
		return []schema.Record{makeRecord(cur, op)}, nil
	}

	if cur.Height > 0 && prev != nil {
		factor := 1
		if op == schema.SeqTypeDel {
			factor = -1
		}
		ret := BStat{
			Height:  detail.Block.Height,
			Time:    detail.Block.BlockTime,
			TxCount: prev.TxCount + cur.TxCount*factor,
			Fee:     prev.Fee + cur.Fee*int64(factor),
			Mine:    prev.Mine + cur.Mine*int64(factor),
			Coins:   prev.Coins + (cur.Mine-cur.Fee)*int64(factor),
		}
		prev = &ret
		return []schema.Record{makeRecord(&ret, op)}, nil
	}

	// 若是不是从零开始， 或没有从数据库中读到上次的值，累计将出错
	return nil, nil
}

func statCur(title string, detail *types.BlockDetail, genesis, coin int64) *BStat {
	cur := BStat{
		Height:  detail.Block.Height,
		Time:    detail.Block.BlockTime,
		TxCount: len(detail.Block.Txs),
	}

	for _, tx := range detail.Block.Txs {
		cur.Fee += tx.GetFee()
	}

	if title == "bityuan" || title == "" {
		if cur.Height == 0 {
			cur.Coins = bityuanGenesis()
		}
		cur.Mine = bityuanMiner(cur.Height)
	} else {
		if cur.Height == 0 {
			cur.Coins = genesis
		}
		cur.Mine = coin
	}
	return &cur
}

func bityuanMiner(height int64) int64 {
	// ForkChainParamV2
	if height >= 2270000 {
		return 8 * coin
	} else if height > 0 {
		return 30 * coin
	}
	return 0
}

func bityuanGenesis() int64 {
	return 317430000 * coin
}

// StatRecord StatRecord
type StatRecord struct {
	*schema.IKey
	*schema.Op
	s *BStat
}

// Append impl
func (r *StatRecord) Append(r2 schema.Record) schema.Record {
	return r
}

// Value impl
func (r *StatRecord) Value() []byte {
	v, _ := json.Marshal(r.s)
	return v
}

func makeRecord(s *BStat, op int) *StatRecord {
	return &StatRecord{
		IKey: schema.NewIKey(DBStatX, DBStatX, s.ID()),
		Op:   schema.NewOp(op),
		s:    s,
	}
}
