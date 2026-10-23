package accountrefresh

import (
	"testing"
	"time"
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
