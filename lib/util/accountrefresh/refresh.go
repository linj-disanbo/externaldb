// Package accountrefresh 定时把链上真实的 BTY 余额校正回 ES 的 account 文档。
//
// 目标文档（与 jrpc Account.TopAsset 的排名口径一致）：
//   - coins 账户：exec=""（人账户），asset_exec=coins, asset_symbol=<symbol>
//   - ticket 账户：exec=<ticket 执行器地址>，asset_exec=coins（票冻结的 bty 记在 frozen）
//
// 三档（间隔可配，<=0 表示该档不跑）：
//   - top   ：按 total 排序的前 N 个地址（同 TopAsset 的聚合口径），默认 1h
//   - active：transaction 索引里近 activeWindow 出现过的地址（from/to），默认 24h
//   - full  ：全量 account 文档按 balance_refresh_at 升序（缺失优先）每次取一页，默认 168h
//
// 写入用 OpUpdate 部分更新：只写 balance/frozen/total/balance_refresh_at/balance_refresh_height，
// 不动 height/height_index/block_time/type/addr_type（那些是日志解析维护的）。
package accountrefresh

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	chain33types "github.com/33cn/chain33/types"
	"github.com/pkg/errors"

	"github.com/33cn/externaldb/lib/chaincfg"
	"github.com/33cn/externaldb/lib/escli"
	"github.com/33cn/externaldb/lib/escli/querypara"
	"github.com/33cn/externaldb/lib/esdoc/account"
	"github.com/33cn/externaldb/lib/schema"
)

// AccountBalance 链上查到一个地址的余额
type AccountBalance struct {
	Addr    string
	Balance int64
	Frozen  int64
}

// BalanceSource 链上余额/高度来源（gRPC 实现见 GRPCSource；测试可替换）
type BalanceSource interface {
	Balances(ctx context.Context, execer, assetExec, symbol string, addrs []string) ([]AccountBalance, error)
	Height(ctx context.Context) (int64, error)
}

// GRPCSource 用 chain33 gRPC 查余额
type GRPCSource struct {
	client chain33types.Chain33Client
}

// NewGRPCSource ...
func NewGRPCSource(c chain33types.Chain33Client) *GRPCSource { return &GRPCSource{client: c} }

// Balances 批量查余额（一次调用带多个地址）
func (s *GRPCSource) Balances(ctx context.Context, execer, assetExec, symbol string, addrs []string) ([]AccountBalance, error) {
	if len(addrs) == 0 {
		return nil, nil
	}
	rep, err := s.client.GetBalance(ctx, &chain33types.ReqBalance{
		Addresses:   addrs,
		Execer:      execer,
		AssetExec:   assetExec,
		AssetSymbol: symbol,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "GetBalance execer=%s addrs=%d", execer, len(addrs))
	}
	out := make([]AccountBalance, 0, len(rep.GetAcc()))
	for _, a := range rep.GetAcc() {
		out = append(out, AccountBalance{Addr: a.GetAddr(), Balance: a.GetBalance(), Frozen: a.GetFrozen()})
	}
	return out, nil
}

// Height 当前链高
func (s *GRPCSource) Height(ctx context.Context) (int64, error) {
	h, err := s.client.GetLastHeader(ctx, &chain33types.ReqNil{})
	if err != nil {
		return 0, err
	}
	return h.GetHeight(), nil
}

// Options 刷新参数
type Options struct {
	ES     escli.ESClient
	Chain  BalanceSource
	Symbol string // 资产符号（= [chain].symbol）
	Title  string // 平行链 title（含末尾点），主链为空

	TopN      int // top 档地址数，默认 1000
	PageSize  int // 每轮从 ES 取多少文档，默认 1000
	BatchSize int // 一次 GetBalance 的地址数，默认 200

	TopInterval    time.Duration // <=0 不跑
	ActiveInterval time.Duration
	FullInterval   time.Duration
	ActiveWindow   time.Duration

	TicketExec string // ticket 执行器地址；空则用 chaincfg.ExecAddress(title+"ticket")

	DryRun bool
	Now    func() time.Time
	Log    func(msg string, kv ...interface{})
}

// Stats 一轮统计
type Stats struct {
	Top, Active, Full int // 各档实际处理的文档数
	Changed           int // balance/frozen 发生变化的文档数
	Dropped           int // 链上查不到结果而跳过的文档数
	Failed            int // 写 ES 失败条数
}

// Refresher 余额校正器
type Refresher struct {
	opt        Options
	ticketExec string
	execCoins  string
	execTicket string

	lastTop, lastActive, lastFull time.Time
}

// New 构造（补默认值）
func New(opt Options) *Refresher {
	if opt.TopN <= 0 {
		opt.TopN = 1000
	}
	if opt.PageSize <= 0 {
		opt.PageSize = 1000
	}
	if opt.BatchSize <= 0 {
		opt.BatchSize = 200
	}
	if opt.ActiveWindow <= 0 {
		opt.ActiveWindow = 24 * time.Hour
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Log == nil {
		opt.Log = func(string, ...interface{}) {}
	}
	r := &Refresher{opt: opt}
	r.execCoins = "coins"
	r.execTicket = "ticket"
	if opt.Title != "" {
		r.execCoins = opt.Title + "coins"
		r.execTicket = opt.Title + "ticket"
	}
	r.ticketExec = opt.TicketExec
	if r.ticketExec == "" {
		r.ticketExec = chaincfg.ExecAddress(r.execTicket)
	}
	if r.ticketExec == "" {
		r.opt.Log("ticket 执行器地址为空，ticket 档将被跳过（可用 TicketExec 指定）")
	}
	return r
}

// Loop 常驻：启动先跑一轮到期的，然后每 tick 检查一次。
func (r *Refresher) Loop(ctx context.Context, tick time.Duration) error {
	if tick <= 0 {
		tick = time.Minute
	}
	for {
		st, err := r.RunOnce(ctx)
		if err != nil {
			r.opt.Log("account_refresh round failed", "err", err)
		} else {
			r.opt.Log("account_refresh round done",
				"top", st.Top, "active", st.Active, "full", st.Full,
				"changed", st.Changed, "dropped", st.Dropped, "failed", st.Failed)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(tick):
		}
	}
}

// RunOnce 跑所有"到期"的档
func (r *Refresher) RunOnce(ctx context.Context) (Stats, error) {
	now := r.opt.Now()
	var st Stats
	if r.due(now, r.lastTop, r.opt.TopInterval) {
		n, ch, dropped, err := r.refreshTop(ctx)
		if err != nil {
			return st, errors.Wrap(err, "top tier")
		}
		st.Top, st.Changed, st.Dropped = n, ch, dropped
		r.lastTop = now
	}
	if r.due(now, r.lastActive, r.opt.ActiveInterval) {
		n, ch, dropped, err := r.refreshActive(ctx)
		if err != nil {
			return st, errors.Wrap(err, "active tier")
		}
		st.Active, st.Changed, st.Dropped = n, st.Changed+ch, st.Dropped+dropped
		r.lastActive = now
	}
	if r.due(now, r.lastFull, r.opt.FullInterval) {
		n, ch, dropped, err := r.refreshFull(ctx)
		if err != nil {
			return st, errors.Wrap(err, "full tier")
		}
		st.Full, st.Changed, st.Dropped = n, st.Changed+ch, st.Dropped+dropped
		r.lastFull = now
	}
	return st, nil
}

func (r *Refresher) due(now, last time.Time, interval time.Duration) bool {
	if interval <= 0 {
		return false
	}
	return last.IsZero() || now.Sub(last) >= interval
}

// refreshTop 按 TopAsset 的口径取前 N 个地址，直接刷（不做年龄过滤）
func (r *Refresher) refreshTop(ctx context.Context) (docs, changed, dropped int, err error) {
	q := &querypara.Query{
		Range:    []*querypara.QRange{{Key: "total", GT: 0}},
		MatchOne: []*querypara.QMatch{{Key: "exec", Value: ""}, {Key: "exec", Value: r.ticketExec}},
		Not:      []*querypara.QMatch{{Key: "type", Value: "contract"}},
		Filter:   []*querypara.QMatch{{Key: "asset_symbol", Value: r.opt.Symbol}, {Key: "asset_exec", Value: "coins"}},
	}
	agg := &querypara.Agg{
		Name:  "total",
		Size:  &querypara.ASize{Size: r.opt.TopN},
		Order: &querypara.AOrder{Key: "total", Asc: false},
		Term:  &querypara.AAgg{Key: "address"},
		Subs:  &querypara.ASub{Sum: []*querypara.AAgg{{Name: "total", Key: "total"}}},
	}
	res, err := r.opt.ES.Agg(account.DBX, account.DBX, &querypara.Search{Query: q, Agg: agg})
	if err != nil {
		return 0, 0, 0, errors.Wrap(err, "agg top addresses")
	}
	buckets, _ := res.Term("total")
	addrs := make([]string, 0, len(buckets))
	for _, b := range buckets {
		if s, ok := b.Key.(string); ok && s != "" {
			addrs = append(addrs, s)
		}
	}
	r.opt.Log("top tier addresses", "n", len(addrs), "ticketExec", r.ticketExec)
	return r.refreshAddrs(ctx, addrs)
}

// refreshActive 近 activeWindow 内在 transaction 里出现过的地址（from/to）
func (r *Refresher) refreshActive(ctx context.Context) (docs, changed, dropped int, err error) {
	cut := r.opt.Now().Add(-r.opt.ActiveWindow).Unix()
	set := map[string]bool{}
	for _, key := range []string{"from", "to"} {
		q := &querypara.Query{
			Range:  []*querypara.QRange{{Key: "block_time", GT: cut}},
			Filter: []*querypara.QMatch{{Key: "asset_symbol", Value: r.opt.Symbol}},
		}
		agg := &querypara.Agg{
			Name: "addr",
			Size: &querypara.ASize{Size: r.opt.PageSize},
			Term: &querypara.AAgg{Key: key},
		}
		res, aggErr := r.opt.ES.Agg("transaction", "transaction", &querypara.Search{Query: q, Agg: agg})
		if aggErr != nil {
			return 0, 0, 0, errors.Wrapf(aggErr, "agg active %s", key)
		}
		buckets, _ := res.Term("addr")
		for _, b := range buckets {
			if s, ok := b.Key.(string); ok && s != "" {
				set[s] = true
			}
		}
	}
	addrs := make([]string, 0, len(set))
	for a := range set {
		addrs = append(addrs, a)
	}
	sort.Strings(addrs)
	r.opt.Log("active tier addresses", "n", len(addrs), "window", r.opt.ActiveWindow)
	return r.refreshAddrs(ctx, addrs)
}

// fullDoc 全量档从 ES 读到的 account 文档
type fullDoc struct {
	Address     string `json:"address"`
	Exec        string `json:"exec"`
	Balance     int64  `json:"balance"`
	Frozen      int64  `json:"frozen"`
	Total       int64  `json:"total"`
	AssetExec   string `json:"asset_exec"`
	AssetSymbol string `json:"asset_symbol"`
}

// refreshFull 按 balance_refresh_at 升序（缺失优先）取一页，形成自推进队列：
// 刷完的文档 refresh_at 变新，自然排到后面，下一轮取到的是下一批。
func (r *Refresher) refreshFull(ctx context.Context) (docs, changed, dropped int, err error) {
	q := &querypara.Query{
		Page:   &querypara.QPage{Size: r.opt.PageSize, Number: 1},
		Sort:   []*querypara.QSort{{Key: "balance_refresh_at", Ascending: true, Missing: "_first"}},
		Filter: []*querypara.QMatch{{Key: "asset_symbol", Value: r.opt.Symbol}, {Key: "asset_exec", Value: "coins"}},
	}
	rs, err := r.opt.ES.Search(account.DBX, account.DBX, q, func(x *json.RawMessage) (interface{}, error) {
		d := &fullDoc{}
		return d, json.Unmarshal(*x, d)
	})
	if err != nil {
		return 0, 0, 0, errors.Wrap(err, "search full page")
	}
	addrs := make([]string, 0, len(rs))
	seen := map[string]bool{}
	for _, it := range rs {
		d, ok := it.(*fullDoc)
		if !ok || d == nil || d.Address == "" || seen[d.Address] {
			continue
		}
		seen[d.Address] = true
		addrs = append(addrs, d.Address)
	}
	r.opt.Log("full tier page", "docs", len(rs), "addresses", len(addrs))
	return r.refreshAddrs(ctx, addrs)
}

// refreshAddrs 按地址刷新（分批查链、只写变化或需要更新 refresh_at 的文档）
func (r *Refresher) refreshAddrs(ctx context.Context, addrs []string) (docs, changed, dropped int, err error) {
	if len(addrs) == 0 {
		return 0, 0, 0, nil
	}
	height, hErr := r.opt.Chain.Height(ctx)
	if hErr != nil {
		// 高度只是观测字段，拿不到不影响余额校正
		r.opt.Log("get chain height failed, refresh_height will be 0", "err", hErr)
		height = 0
	}
	now := r.opt.Now().Unix()

	for start := 0; start < len(addrs); start += r.opt.BatchSize {
		end := start + r.opt.BatchSize
		if end > len(addrs) {
			end = len(addrs)
		}
		batch := addrs[start:end]
		existing, mErr := r.loadDocs(batch)
		if mErr != nil {
			return docs, changed, dropped, mErr
		}
		if len(existing) == 0 {
			continue
		}

		// 按 exec 分组查链：coins 查全批（doc 里 exec="" 或历史 "coins"）；ticket 只查有 ticket 文档的地址
		var ticketAddrs []string
		for _, d := range existing {
			if d.Exec == r.ticketExec {
				ticketAddrs = append(ticketAddrs, d.Address)
			}
		}
		coinsBal, cErr := r.opt.Chain.Balances(ctx, r.execCoins, "coins", r.opt.Symbol, batch)
		if cErr != nil {
			return docs, changed, dropped, cErr
		}
		coinsMap := map[string]AccountBalance{}
		for _, b := range coinsBal {
			coinsMap[b.Addr] = b
		}
		ticketMap := map[string]AccountBalance{}
		if len(ticketAddrs) > 0 && r.ticketExec != "" {
			tb, tErr := r.opt.Chain.Balances(ctx, r.execTicket, "coins", r.opt.Symbol, ticketAddrs)
			if tErr != nil {
				return docs, changed, dropped, tErr
			}
			for _, b := range tb {
				ticketMap[b.Addr] = b
			}
		}

		var records []schema.Record
		for _, d := range existing {
			docs++
			var bal AccountBalance
			var ok bool
			if d.Exec == r.ticketExec {
				bal, ok = ticketMap[d.Address]
			} else {
				bal, ok = coinsMap[d.Address]
			}
			if !ok {
				dropped++
				continue
			}
			total := bal.Balance + bal.Frozen
			if bal.Balance != d.Balance || bal.Frozen != d.Frozen {
				changed++
			}
			// 每次都写 refresh_at/height：full 档靠它自推进；余额没变时写同样的值也无副作用
			rec := &patchRecord{
				IKey: schema.NewIKey(account.DBX, account.DBX, d.id),
				Op:   schema.NewOp(schema.OpUpdate),
				value: map[string]interface{}{
					"balance":                bal.Balance,
					"frozen":                 bal.Frozen,
					"total":                  total,
					"balance_refresh_at":     now,
					"balance_refresh_height": height,
				},
			}
			records = append(records, rec)
		}
		if len(records) == 0 {
			continue
		}
		if r.opt.DryRun {
			r.opt.Log("dry-run: would update", "count", len(records))
			continue
		}
		if uErr := r.opt.ES.BulkUpdate(records); uErr != nil {
			// 单条失败已在 BulkUpdate 内部记录，这里只累计
			r.opt.Log("bulk update failed", "err", uErr)
			return docs, changed, dropped, uErr
		}
	}
	return docs, changed, dropped, nil
}

// docRef 命中的 account 文档（带 _id）
type docRef struct {
	id          string
	Address     string `json:"address"`
	Exec        string `json:"exec"`
	Balance     int64  `json:"balance"`
	Frozen      int64  `json:"frozen"`
	Total       int64  `json:"total"`
	AssetExec   string `json:"asset_exec"`
	AssetSymbol string `json:"asset_symbol"`
}

// loadDocs 用 MGet 取候选文档（coins 兼容两种历史 id：exec="" 与 exec="coins"）
func (r *Refresher) loadDocs(addrs []string) ([]*docRef, error) {
	ids := make([]string, 0, len(addrs)*3)
	seen := map[string]bool{}
	for _, a := range addrs {
		for _, exec := range []string{"", "coins", r.ticketExec} {
			id := fmt.Sprintf("%s-%s-coins:%s", a, exec, r.opt.Symbol)
			if seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	rs, err := r.opt.ES.MGet(account.DBX, account.DBX, ids, func(x *json.RawMessage) (interface{}, error) {
		d := &docRef{}
		return d, json.Unmarshal(*x, d)
	})
	if err != nil {
		return nil, errors.Wrap(err, "mget account docs")
	}
	out := make([]*docRef, 0, len(rs))
	for i, it := range rs {
		if it == nil {
			continue
		}
		d, ok := it.(*docRef)
		if !ok || d == nil {
			continue
		}
		d.id = ids[i]
		out = append(out, d)
	}
	return out, nil
}

// patchRecord OpUpdate 部分更新记录
type patchRecord struct {
	*schema.IKey
	*schema.Op
	value interface{}
}

// Value impl
func (r *patchRecord) Value() []byte {
	v, _ := json.Marshal(r.value)
	return v
}
