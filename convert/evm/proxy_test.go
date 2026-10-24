package evm

import (
	"testing"

	"github.com/33cn/chain33/common"
	chaintypes "github.com/33cn/chain33/types"
	"github.com/33cn/externaldb/lib/chaincfg"
	"github.com/33cn/externaldb/lib/esdoc/transaction"
	"github.com/33cn/externaldb/lib/proto"
	"github.com/33cn/externaldb/lib/schema"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
)

// 线上 tx 0xaa11cf31… 的 para：内层 chain33 交易（coins Withdraw，85 BTY，execName=ticket）
const testEvmCallGoPara = "0a05636f696e731236180322321080ea8ed51f22067469636b65742a22313668747663424e53454137665a6841644c4a706844775152514a6148707948547020a08d0630cba5b5fcf890d6d1303a22313668747663424e53454137665a6841644c4a706844775152514a61487079485470"

// 同一笔 tx 的 note：外层 eth 交易 RLP（eth 兼容模式规范）
const testEvmCallGoNote = "f8db823e858502540be400830249f094000000000000000000000000000000000020000580b8710a05636f696e731236180322321080ea8ed51f22067469636b65742a22313668747663424e53454137665a6841644c4a706844775152514a6148707948547020a08d0630cba5b5fcf890d6d1303a22313668747663424e53454137665a6841644c4a706844775152514a61487079485470821792a07f254641e0f1d25e5f882d7e01804d6869359901ac1856af131f473357e58b10a01673098fc8648b2b06827325c18c459eecaad8a3a79834b2a2081b0d43dfdbeb"

func newEvmCallGoEnv(t *testing.T, note string) *schema.TxEnv {
	t.Helper()
	chaincfg.InitChain33("", "bty", "")
	schema.SetVersion(7)

	para, err := common.FromHex(testEvmCallGoPara)
	assert.Nil(t, err)
	payload := &proto.EVMContractAction{
		Para:         para,
		Note:         note,
		ContractAddr: chaincfg.EvmCallGoAddr,
	}
	// From() 需要合法的 eth 公钥（eth 地址驱动会解析 65 字节公钥），签名内容本用例不校验
	key, err := ethcrypto.GenerateKey()
	assert.Nil(t, err)
	tx := &chaintypes.Transaction{
		Execer:  []byte("evm"),
		Payload: chaintypes.Encode(payload),
		To:      chaincfg.EvmCallGoAddr,
		Fee:     100000,
		Signature: &chaintypes.Signature{
			Ty:        chaintypes.EncodeSignID(chaintypes.SECP256K1ETH, chaintypes.EthAddressID),
			Pubkey:    ethcrypto.FromECDSAPub(&key.PublicKey),
			Signature: make([]byte, 65),
		},
	}
	detail := &chaintypes.BlockDetail{
		Block:    &chaintypes.Block{Height: 39856428, BlockTime: 1666515000, Txs: []*chaintypes.Transaction{tx}},
		Receipts: []*chaintypes.ReceiptData{{Ty: chaintypes.ExecOk}},
	}
	return &schema.TxEnv{Block: detail, TxIndex: 0, BlockHash: "0xblock"}
}

// 代理执行：不查 contract 索引（DB 全为 nil 也不会 panic），按内层交易补 options
func Test_ConvertTx_EvmCallGo(t *testing.T) {
	env := newEvmCallGoEnv(t, testEvmCallGoNote)

	convert := NewConvert("", "bty", nil).(*Convert)
	// 故意不 SetDB：代理分支必须在访问 ContractDB 之前返回

	records, err := convert.ConvertTx(env, schema.OpAdd)
	assert.Nil(t, err)
	if !assert.Len(t, records, 1) {
		return
	}

	txRecord, ok := records[0].(*transaction.TxRecord)
	if !assert.True(t, ok) {
		return
	}
	opt, ok := txRecord.Tx.Options.(*TxOption)
	if !assert.True(t, ok) {
		return
	}
	assert.Equal(t, "coins.withdraw", opt.CallFunName)
	assert.Equal(t, chaincfg.EvmCallGoAddr, opt.ContractAddr)
	assert.Equal(t, "", opt.ContractType, "代理地址不是合约，不应伪造 contract_type")
	assert.NotZero(t, txRecord.Tx.Amount, "金额应取内层 coins withdraw 的 amount")
}
