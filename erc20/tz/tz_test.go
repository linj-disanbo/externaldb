package tz

import (
	"testing"
	"time"
)

func TestCST(t *testing.T) {
	loc := CST()
	if loc == nil {
		t.Fatal("CST() 不得返回 nil（否则 t.In(nil) 会 panic）")
	}
	// 2026-01-01 00:00:00 UTC 在 +08:00 应为 08:00
	utc := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got, want := utc.In(loc).Format("2006-01-02 15:04:05"), "2026-01-01 08:00:00"; got != want {
		t.Fatalf("CST 偏移不对：got %s, want %s", got, want)
	}
	// 东八区无夏令时，7 月同样 +8
	if _, off := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).In(loc).Zone(); off != 8*60*60 {
		t.Fatalf("offset = %d, want 28800", off)
	}
}
