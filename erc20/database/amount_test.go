package database

import (
	"math/big"
	"testing"
)

func TestClampAmountForDB(t *testing.T) {
	if got := clampAmountForDB(nil); got != nil {
		t.Fatalf("nil 应原样返回 nil，got %v", got)
	}

	normal := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil) // 10^18
	if got := clampAmountForDB(normal); got.Cmp(normal) != 0 {
		t.Fatalf("正常值不应被改：got %s", got)
	}

	// DECIMAL(65,0) 的上界本身可以存
	if got := clampAmountForDB(new(big.Int).Set(maxDecimal65)); got.Cmp(maxDecimal65) != 0 {
		t.Fatalf("10^65-1 不应被改：got %s", got)
	}

	// 超过上界 → -1（unlimited 哨兵），否则 MySQL 报 1264
	over := new(big.Int).Add(maxDecimal65, big.NewInt(1)) // 10^65
	if got := clampAmountForDB(over); got.Cmp(big.NewInt(-1)) != 0 {
		t.Fatalf("10^65 应归一成 -1，got %s", got)
	}

	// uint256 最大值（78 位）与 max-1 都要被归一
	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	if got := clampAmountForDB(maxUint256); got.Cmp(big.NewInt(-1)) != 0 {
		t.Fatalf("MaxUint256 应归一成 -1，got %s", got)
	}
	if got := clampAmountForDB(new(big.Int).Sub(maxUint256, big.NewInt(1))); got.Cmp(big.NewInt(-1)) != 0 {
		t.Fatalf("MaxUint256-1 应归一成 -1，got %s", got)
	}
}
