package database

import "testing"

func TestEnsureNativeTransferContractSymbolUpper(t *testing.T) {
	// 只校验纯函数部分：symbol 归一化（DB 交互在 MySQL 用例里）
	cases := map[string]string{"": "BTY", "bty": "BTY", " bty ": "BTY", "YCC": "YCC"}
	for in, want := range cases {
		if got := normalizeNativeSymbol(in); got != want {
			t.Errorf("normalizeNativeSymbol(%q) = %q, want %q", in, got, want)
		}
	}
}
