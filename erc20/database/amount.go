package database

import "math/big"

// maxDecimal65：MySQL DECIMAL(65,0) 能表示的最大值（10^65 - 1）。
// DECIMAL 的最大精度就是 65 位十进制，而 ERC20 金额是 uint256（最多 78 位），
// 超范围写进去会直接报 "Error 1264 (22003): Out of range value for column ..."，
// 整条记录（events / token_allowances / token_balances / transactions / contracts）都写不进去。
var maxDecimal65 = new(big.Int).Sub(
	new(big.Int).Exp(big.NewInt(10), big.NewInt(65), nil), big.NewInt(1))

// clampAmountForDB 把装不进 DECIMAL(65,0) 的金额归一成 -1，避免 MySQL 严格模式 1264。
// -1 与 Approval "无限授权"的哨兵值一致（见 engine.normalizeApprovalAmount）：
// 1e65 以上的额度/余额在业务上等价于无限，没必要为它让整条记录写失败。
// nil 原样返回（调用方各自决定写 NULL）。
func clampAmountForDB(v *big.Int) *big.Int {
	if v == nil || v.Cmp(maxDecimal65) <= 0 {
		return v
	}
	return big.NewInt(-1)
}
