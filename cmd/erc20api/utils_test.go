package main

import (
	"math/big"
	"testing"
)

func TestFormatTokenAmountUnlimited(t *testing.T) {
	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	if got := formatTokenAmount(maxUint256, 18); got != "unlimited" {
		t.Fatalf("MaxUint256 = %q, want unlimited", got)
	}
	// 入库哨兵（DECIMAL(65,0) 装不下的金额）也要按 unlimited 展示，不能显示成 -1.x
	if got := formatTokenAmount(big.NewInt(-1), 18); got != "unlimited" {
		t.Fatalf("-1 = %q, want unlimited", got)
	}
	if got := formatTokenAmount(big.NewInt(1500000000000000001), 18); got != "1.500000000000000001" {
		t.Fatalf("正常值格式化错误: %q", got)
	}
	if got := formatTokenAmount(nil, 18); got != "0" {
		t.Fatalf("nil = %q, want 0", got)
	}
}
