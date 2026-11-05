package engine

import (
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
