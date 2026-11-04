package accountrefresh

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/33cn/externaldb/lib/escli"
	"github.com/33cn/externaldb/lib/escli/aggdecode"
	"github.com/33cn/externaldb/lib/escli/querypara"
)

func TestDue(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	r := &Refresher{}

	if r.due(now, time.Time{}, 0) {
		t.Error("interval<=0 应表示该档不跑")
	}
	if !r.due(now, time.Time{}, time.Hour) {
		t.Error("从未跑过应视为到期")
	}
	if r.due(now, now.Add(-30*time.Minute), time.Hour) {
		t.Error("未到间隔不应到期")
	}
	if !r.due(now, now.Add(-time.Hour), time.Hour) {
		t.Error("正好到间隔应到期")
	}
}

func TestNewDefaults(t *testing.T) {
	r := New(Options{Symbol: "bty", Title: "user.p.ex.", TicketExec: "0xticket"})
	if r.opt.TopN != 1000 || r.opt.PageSize != 1000 || r.opt.BatchSize != 200 {
		t.Errorf("默认值不对: topN=%d page=%d batch=%d", r.opt.TopN, r.opt.PageSize, r.opt.BatchSize)
	}
	if r.opt.ActiveWindow != 24*time.Hour {
		t.Errorf("默认活跃窗口应为 24h, got %v", r.opt.ActiveWindow)
	}
	if r.execCoins != "user.p.ex.coins" || r.execTicket != "user.p.ex.ticket" {
		t.Errorf("平行链 execer 应带 title 前缀: coins=%s ticket=%s", r.execCoins, r.execTicket)
	}
	if r.ticketExec != "0xticket" {
		t.Errorf("TicketExec 应直接使用, got %q", r.ticketExec)
	}

	// 主链：无 title 时不加前缀
	r2 := New(Options{Symbol: "bty", TicketExec: "0xt"})
	if r2.execCoins != "coins" || r2.execTicket != "ticket" {
		t.Errorf("主链 execer 不该带前缀: coins=%s ticket=%s", r2.execCoins, r2.execTicket)
	}
}

// 推算 ticket 执行器：只有 user.p. 开头的 title 才是平行链，主链 title 不参与计算；
// 地址格式随 AddressDriver（btc → base58，eth → 0x）。
func TestTicketExecDerivedFromTitleAndDriver(t *testing.T) {
	cases := []struct {
		name       string
		title      string
		driver     string
		wantExec   string
		wantTicket string
	}{
		{"主链空 title/btc", "", "btc", "ticket", "16htvcBNSEA7fZhAdLJphDwQRQJaHpyHTp"},
		{"主链 title 非 user.p. 不参与计算", "bityuan", "btc", "ticket", "16htvcBNSEA7fZhAdLJphDwQRQJaHpyHTp"},
		{"平行链 btc", "user.p.ex.", "btc", "user.p.ex.ticket", "1GBnPKVSPrKuWBi1jwsHEKmSKTnjgo8ex3"},
		{"主链 eth", "", "eth", "ticket", "0x47e48d946c5db1a18de7357a01d070c0bf328158"},
	}
	for _, c := range cases {
		r := New(Options{Symbol: "bty", Title: c.title, AddressDriver: c.driver})
		if r.execTicket != c.wantExec {
			t.Errorf("%s: execTicket = %q, want %q", c.name, r.execTicket, c.wantExec)
		}
		if r.ticketExec != c.wantTicket {
			t.Errorf("%s: ticketExec = %q, want %q", c.name, r.ticketExec, c.wantTicket)
		}
	}

	// 显式配置优先于推算（不因 AddressDriver 改写）
	r := New(Options{Symbol: "bty", Title: "bityuan", AddressDriver: "eth", TicketExec: "0xabc"})
	if r.ticketExec != "0xabc" {
		t.Errorf("显式 TicketExec 应直接使用, got %q", r.ticketExec)
	}
}

// emptyEnvES 模拟"新环境、索引都还没建"：Agg 返回空结果（ESClientV7 对 404 的新契约），
// Search 返回空列表。其它方法不该被调用，故用嵌入的 nil 接口占位。
type emptyEnvES struct {
	escli.ESClient
	aggCalls int
}

func (e *emptyEnvES) Agg(idx, typ string, _ *querypara.Search) (*aggdecode.AggregationBucketKeyItem, error) {
	e.aggCalls++
	return &aggdecode.AggregationBucketKeyItem{Aggregations: map[string]json.RawMessage{}, Version: 7}, nil
}

func (e *emptyEnvES) Search(idx, typ string, _ *querypara.Query, _ func(*json.RawMessage) (interface{}, error)) ([]interface{}, error) {
	return nil, nil
}

// 回归：索引还不存在的新机器上跑一轮必须不 panic（历史问题是 Agg 返回 nil 导致
// refreshTop 里 res.Term 空指针，进程启动即退出），并且不该产生任何处理量。
func TestRunOnceEmptyEnv(t *testing.T) {
	es := &emptyEnvES{}
	r := New(Options{
		ES:             es,
		Symbol:         "bty",
		TicketExec:     "16htvcBNSEA7fZhAdLJphDwQRQJaHpyHTp",
		TopInterval:    time.Hour,
		ActiveInterval: time.Hour,
		FullInterval:   time.Hour,
		Now:            func() time.Time { return time.Unix(1_700_000_000, 0) },
	})

	st, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("空环境不应报错: %v", err)
	}
	if st != (Stats{}) {
		t.Fatalf("空环境不应有处理量: %+v", st)
	}
	if es.aggCalls != 3 { // top 1 次 + active 的 from/to 各 1 次
		t.Fatalf("agg 调用次数 = %d, want 3", es.aggCalls)
	}
}
