package engine

import (
	"strings"
	"testing"

	"github.com/33cn/chain33/system/crypto/secp256k1eth"
	chain33types "github.com/33cn/chain33/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// eth 签名的 chain33 交易（ty = EncodeSignID(secp256k1eth.ID, EthAddressID=2)）。
// tx.From() 会按签名里的 addressID 调 address.MustLoadDriver(2)，
// 未注册 eth 地址驱动时会 panic: unknown address driver, id=2。
func TestBuildSlotEthExpectsFromBlockDetail_EthSign(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := crypto.CompressPubkey(&key.PublicKey)

	ethTx := &chain33types.Transaction{
		Execer:  []byte("evm"),
		Payload: []byte("{}"),
		Nonce:   7,
		Signature: &chain33types.Signature{
			Ty:     chain33types.EncodeSignID(secp256k1eth.ID, chain33types.EthAddressID),
			Pubkey: pub,
		},
	}
	// 非 evm 交易不取 From，用于确认槽位标记
	otherTx := &chain33types.Transaction{Execer: []byte("coins")}

	detail := &chain33types.BlockDetail{
		Block: &chain33types.Block{Height: 1, Txs: []*chain33types.Transaction{ethTx, otherTx}},
	}
	got, err := buildSlotEthExpectsFromBlockDetail(detail)
	if err != nil {
		t.Fatalf("buildSlotEthExpectsFromBlockDetail: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 slots, got %d", len(got))
	}
	if !got[0].NeedEth || got[0].Nonce != 7 {
		t.Fatalf("slot0 = %+v, want NeedEth nonce=7", got[0])
	}
	if !strings.HasPrefix(strings.ToLower(got[0].From), "0x") || len(got[0].From) != 42 {
		t.Fatalf("slot0.From = %q, want eth address", got[0].From)
	}
	if got[1].NeedEth {
		t.Fatalf("slot1 = %+v, want NeedEth=false", got[1])
	}
}
