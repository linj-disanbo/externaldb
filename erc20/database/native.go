package database

import "strings"

// NativeTransferAddress 普通主币转账（无 calldata 的 ETH/BTY 转账）在 contracts 表里的归属地址。
//
// 为什么需要它：transactions.contract_address 是 NOT NULL + 外键到 contracts，普通转账交易也必须
// 指向一个 contracts 行；如果直接填 tx.To()，每个收款 EOA 都会在 contracts 里留下一条 UNKNOWN
// 占位记录（线上实测 5.8 万条且会持续增长）。
//
// 为什么不用 chain33 的 proxyExecAddress（0x0000000000000000000000000000000000200005）：
// 那是"EVM 代理执行 Go 执行器"的地址，代理交易本身就会以它为 contract_address，
// 用来当普通转账的哨兵会把两类交易混在一起，没法区分。
//
// 采用 ERC-7528 约定的原生币伪地址，语义清晰、地址固定、不会与真实合约冲突（该地址无代码）。
const NativeTransferAddress = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

// EnsureNativeTransferContract 幂等写入"主币转账"记录行（contract_type='NATIVE'）。
// symbol 统一大写（配置里 [chain].symbol 通常是小写 bty），空则用 BTY。
// decimals 固定 18：transactions.value 是 EVM wei，v_transaction_details 用该字段算 value_formatted；
// total_supply 留 NULL（不参与代币列表/余额查询）。
func (db *DB) EnsureNativeTransferContract(name, symbol string) error {
	if name == "" {
		name = "Native"
	}
	symbol = normalizeNativeSymbol(symbol)
	q := `INSERT INTO contracts (
			contract_address, contract_name, contract_symbol, contract_type, decimals,
			verification_status, verified_functions
		) VALUES (?, ?, ?, 'NATIVE', 18, 0, '[]')
		ON DUPLICATE KEY UPDATE
			contract_name = VALUES(contract_name),
			contract_symbol = VALUES(contract_symbol),
			contract_type = 'NATIVE',
			decimals = 18`
	_, err := db.exec(q, NativeTransferAddress, name, symbol)
	return err
}

// normalizeNativeSymbol 主币符号统一大写（配置里通常是 bty），空则 BTY
func normalizeNativeSymbol(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return "BTY"
	}
	return symbol
}
