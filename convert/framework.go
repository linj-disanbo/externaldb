package convert

import (
	"github.com/33cn/chain33/common"
	rpctypes "github.com/33cn/chain33/rpc/types"
	"github.com/33cn/chain33/types"

	"github.com/33cn/externaldb/lib/schema"
)

// TxEnv / HeightIndex 别名（契约在 lib/schema）
type TxEnv = schema.TxEnv

// HeightIndex 见 schema.HeightIndex
var HeightIndex = schema.HeightIndex

// Convert for tx convert
type Convert interface {
	ConvertTx(env *TxEnv, op int) ([]schema.Record, error)
}

// WrapDB/NeedWrapDB 别名（契约在 lib/schema）
type (
	WrapDB     = schema.WrapDB
	NeedWrapDB = schema.NeedWrapDB
)

// 存储层错误别名（定义在 lib/schema）
var (
	ErrDBNotFound         = schema.ErrDBNotFound
	ErrDBBadParam         = schema.ErrDBBadParam
	ErrDBInvalidOperation = schema.ErrDBInvalidOperation
)

// SetupBlock set into to block
func SetupBlock(env *TxEnv, from, txHash string) *schema.Block {
	return &schema.Block{
		Height:      env.Block.Block.Height,
		Ts:          env.Block.Block.BlockTime,
		BlockHash:   env.BlockHash,
		Index:       env.TxIndex,
		Send:        from,
		TxHash:      txHash,
		HeightIndex: HeightIndex(env.Block.Block.Height, env.TxIndex),
	}
}

// NewBlock New
func NewBlock(env *TxEnv) *schema.Block {
	tx := env.Block.Block.Txs[env.TxIndex]
	return SetupBlock(env, tx.From(), common.ToHex(tx.Hash()))
}

// NewBlockByTxDetail New
func NewBlockByTxDetail(txd *rpctypes.TransactionDetail) *schema.Block {
	return &schema.Block{
		Height: txd.Height,
		Ts:     txd.Blocktime,
		//BlockHash:   common.ToHex(txd.Tx.Hash()),
		Index:       txd.Index,
		Send:        txd.Fromaddr,
		TxHash:      txd.Tx.Hash,
		HeightIndex: HeightIndex(txd.Height, txd.Index),
	}
}

// NewBlockByTxDetail2 New
func NewBlockByTxDetail2(txd *types.TransactionDetail) *schema.Block {
	return &schema.Block{
		Height: txd.Height,
		Ts:     txd.Blocktime,
		//BlockHash:   common.ToHex(txd.Tx.Hash()),
		Index:       txd.Index,
		Send:        txd.Fromaddr,
		TxHash:      common.ToHex(txd.Tx.Hash()),
		HeightIndex: HeightIndex(txd.Height, txd.Index),
	}
}
