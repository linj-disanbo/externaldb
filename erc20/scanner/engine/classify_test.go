package engine

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/33cn/externaldb/erc20/database"
)

// 普通主币转账（无 calldata / 短 data）统一指向哨兵地址，避免给每个收款 EOA 建 contracts 占位行；
// 合约创建、合约调用仍指向真实地址。
func TestClassifyTxForSave(t *testing.T) {
	to := common.HexToAddress("0x1111111111111111111111111111111111111111")
	created := common.HexToAddress("0x2222222222222222222222222222222222222222")
	native := common.HexToAddress(database.NativeTransferAddress)

	t.Run("普通转账走哨兵地址", func(t *testing.T) {
		tx := types.NewTransaction(1, to, nil, 21000, nil, nil)
		addr, sel, name := classifyTxForSave(tx, &types.Receipt{})
		if addr != native {
			t.Fatalf("contractAddr = %s, want 哨兵 %s", addr.Hex(), native.Hex())
		}
		if sel != "0x" || name != "eth_transfer" {
			t.Fatalf("selector/name = %q/%q", sel, name)
		}
	})

	t.Run("短 data 也走哨兵地址", func(t *testing.T) {
		tx := types.NewTransaction(1, to, nil, 21000, nil, []byte{0x01, 0x02})
		addr, _, name := classifyTxForSave(tx, &types.Receipt{})
		if addr != native {
			t.Fatalf("contractAddr = %s, want 哨兵 %s", addr.Hex(), native.Hex())
		}
		if name != "unknown" {
			t.Fatalf("name = %q", name)
		}
	})

	t.Run("合约调用仍指向 to", func(t *testing.T) {
		data := append([]byte{0xa9, 0x05, 0x9c, 0xbb}, make([]byte, 64)...)
		tx := types.NewTransaction(1, to, nil, 100000, nil, data)
		addr, sel, name := classifyTxForSave(tx, &types.Receipt{})
		if addr != to {
			t.Fatalf("contractAddr = %s, want %s", addr.Hex(), to.Hex())
		}
		if sel != "a9059cbb" || name != "unknown" {
			t.Fatalf("selector/name = %q/%q", sel, name)
		}
	})

	t.Run("合约创建走 receipt.ContractAddress", func(t *testing.T) {
		// 合约创建交易的 To 为 nil（geth 用 NewContractCreation 构造）
		tx := types.NewContractCreation(1, nil, 100000, nil, []byte{0x60, 0x00})
		addr, sel, name := classifyTxForSave(tx, &types.Receipt{ContractAddress: created})
		if addr != created {
			t.Fatalf("contractAddr = %s, want %s", addr.Hex(), created.Hex())
		}
		if sel != "0x" || name != "contract_creation" {
			t.Fatalf("selector/name = %q/%q", sel, name)
		}
	})
}

func TestNativeTransferAddressNoOverlapWithProxyExec(t *testing.T) {
	// 哨兵地址不能与 chain33 的 proxyExecAddress（0x…200005）相同，
	// 否则普通转账与"EVM 代理执行 Go 执行器"的交易会混在同一地址下。
	const proxyExecAddr = "0x0000000000000000000000000000000000200005"
	if database.NativeTransferAddress == proxyExecAddr {
		t.Fatal("主币转账哨兵地址不能复用 proxyExecAddress")
	}
	if database.NativeTransferAddress != common.HexToAddress(database.NativeTransferAddress).Hex() {
		// 这里只要求常量本身是小写规范形式（DB 里按小写存）
		t.Logf("哨兵地址: %s", database.NativeTransferAddress)
	}
}

// 回归：普通主币转账的 transactions.value 以前恒为 NULL（只在 len(data)>=4 时赋值）。
func TestTxValue(t *testing.T) {
	// 普通转账：value 取 tx.Value()
	native := big.NewInt(1_234_567)
	if got := txValue(nil, "0x", native); got.Cmp(native) != 0 {
		t.Fatalf("主币转账 value = %s, want %s", got, native)
	}
	// 短 data 也按主币转账
	if got := txValue([]byte{0x01}, "0x", native); got.Cmp(native) != 0 {
		t.Fatalf("短 data value = %s, want %s", got, native)
	}
	// 无 value 的交易（tx.Value() 为 nil）应记 0，而不是 NULL
	if got := txValue(nil, "0x", nil); got == nil || got.Sign() != 0 {
		t.Fatalf("nil value 应记 0, got %v", got)
	}

	// transfer(address,uint256)：取第 4 字节起的 32 字节金额
	data := make([]byte, 68)
	copy(data[:4], []byte{0xa9, 0x05, 0x9c, 0xbb})
	amount := big.NewInt(42)
	amount.FillBytes(data[36:68])
	if got := txValue(data, "a9059cbb", big.NewInt(0)); got.Cmp(amount) != 0 {
		t.Fatalf("transfer value = %s, want %s", got, amount)
	}

	// transferFrom：取最后 32 字节
	data2 := make([]byte, 100)
	copy(data2[:4], []byte{0x23, 0xb8, 0x72, 0xdd})
	amount2 := big.NewInt(99)
	amount2.FillBytes(data2[68:100])
	if got := txValue(data2, "23b872dd", big.NewInt(0)); got.Cmp(amount2) != 0 {
		t.Fatalf("transferFrom value = %s, want %s", got, amount2)
	}

	// 带 value 的合约调用（非 ERC20 方法）：保留 tx.Value()
	call := make([]byte, 36)
	copy(call[:4], []byte{0x12, 0x34, 0x56, 0x78})
	if got := txValue(call, "12345678", native); got.Cmp(native) != 0 {
		t.Fatalf("合约调用 value = %s, want %s", got, native)
	}
}
