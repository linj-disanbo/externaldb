package evm

import (
	"testing"

	"github.com/33cn/chain33/common"
	chaintypes "github.com/33cn/chain33/types"
	converter "github.com/33cn/externaldb/convert"
	dbcom "github.com/33cn/externaldb/convert/common"
	"github.com/33cn/externaldb/lib/chaincfg"
	"github.com/33cn/externaldb/lib/esdoc/contract"
	"github.com/33cn/externaldb/lib/esdoc/transaction"
	"github.com/33cn/externaldb/lib/proto"
	"github.com/33cn/externaldb/lib/schema"
	pabi "github.com/33cn/plugin/plugin/dapp/evm/executor/abi"
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

func Test_parseNote_EthSign(t *testing.T) {
	mapinfo := map[string]interface{}{}
	_, err := parseNote(mapinfo, testEvmCallGoNote, chaincfg.EvmCallGoAddr, true)
	assert.Nil(t, err)
	assert.Equal(t, "rlp", mapinfo["evm_note_type"])
	assert.Equal(t, "0x92c7592ab9e0262902881d8bb19dbf8d391ecddd99f96ba78a809c8c8aa99a6e", mapinfo["evm_eth_tx_hash"])
	assert.EqualValues(t, 16005, mapinfo["evm_nonce"])
	assert.Equal(t, "10000000000", mapinfo["evm_gas_price"])
	assert.EqualValues(t, 150000, mapinfo["evm_gas"])
	// eth 交易不应再写 base64 解出来的 evm_note
	_, ok := mapinfo["evm_note"]
	assert.False(t, ok)
}

func Test_parseNote_NotEthSign(t *testing.T) {
	// 非 eth 签名：base64(JSON) 备注，带 call_func_name
	raw := `{"call_func_name":"transfer"}`
	note := dbcom.EncodeToString([]byte(raw))
	mapinfo := map[string]interface{}{}
	_, err := parseNote(mapinfo, note, "0xABC", false)
	assert.Nil(t, err)
	assert.Equal(t, "transfer", mapinfo["call_func_name"])
	assert.Equal(t, "0xabc", mapinfo["contract_addr"])

	// 非法 base64 只记 Debug，不报错
	mapinfo = map[string]interface{}{}
	_, err = parseNote(mapinfo, "not-base64!!", "0xabc", false)
	assert.Nil(t, err)
	assert.Nil(t, mapinfo["evm_note"])
}

// 未录 ABI 的普通合约：只保留交易记录，不 panic
type notFoundContractDB struct{}

func (notFoundContractDB) Get(string) (*contract.Contract, error) {
	return nil, converter.ErrDBNotFound
}
func (notFoundContractDB) Set(*contract.Record) error                     { return nil }
func (notFoundContractDB) GetABI(string) (*pabi.ABI, error)               { return nil, nil }
func (notFoundContractDB) GetContract(string) (*contract.Contract, error) { return nil, nil }
func (notFoundContractDB) UpdateCache(*contract.Contract)                 {}

func Test_ConvertTx_UnknownContract(t *testing.T) {
	env := newEvmCallGoEnv(t, "")
	env.Block.Block.Txs[0].To = "0x00000000000000000000000000000000000000aa"
	payload := &proto.EVMContractAction{
		ContractAddr: "0x00000000000000000000000000000000000000aa",
	}
	env.Block.Block.Txs[0].Payload = chaintypes.Encode(payload)

	converter := NewConvert("", "bty", nil).(*Convert)
	converter.ContractDB = notFoundContractDB{}

	records, err := converter.ConvertTx(env, schema.OpAdd)
	assert.Nil(t, err)
	assert.Len(t, records, 1)
}

// eth 纯转账（data 为空）：ContractAddr 是 evm 执行器地址，但 Code 为空 → 不能当部署合约
func Test_ConvertTx_TransferOnly(t *testing.T) {
	env := newEvmCallGoEnv(t, "")
	execAddr := chaincfg.ExecAddress("evm")
	to := "0x00000000000000000000000000000000000000bb"
	payload := &proto.EVMContractAction{
		Amount:       8500000000,
		ContractAddr: execAddr,   // chain33 ethrpc：coins 转账时填执行器地址
		Code:         nil,        // 纯转账没有 code
		Para:         []byte(to), // 目的地址 bytes
	}
	env.Block.Block.Txs[0].To = to
	env.Block.Block.Txs[0].Payload = chaintypes.Encode(payload)

	convert := NewConvert("", "bty", nil).(*Convert)
	records, err := convert.ConvertTx(env, schema.OpAdd)
	assert.Nil(t, err)
	// 只有交易记录，不产生 contract 文档
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
	assert.Equal(t, "coins.transfer", opt.CallFunName)
	assert.Equal(t, "", opt.ContractType)
	assert.EqualValues(t, 8500000000, txRecord.Tx.Amount)
}

// 非 eth 签名：即使 to 是代理地址，也不按内层交易解析（chain33 checkProxyExecTx 要求 IsEthSignID）
func Test_ConvertTx_EvmCallGo_NonEthSign(t *testing.T) {
	env := newEvmCallGoEnv(t, testEvmCallGoNote)
	// 改成普通 chain33 签名类型
	env.Block.Block.Txs[0].Signature.Ty = chaintypes.SECP256K1

	convert := NewConvert("", "bty", nil).(*Convert)
	convert.ContractDB = notFoundContractDB{}

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
	assert.Equal(t, "", opt.CallFunName, "非 eth 签名不应按代理执行解析内层交易")
}
