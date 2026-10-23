package convert

import (
	"strings"

	"github.com/33cn/chain33/common"
	"github.com/33cn/chain33/types"

	chaincfg "github.com/33cn/externaldb/lib/chaincfg"
	"github.com/33cn/externaldb/lib/esdoc/transaction"
	"github.com/33cn/externaldb/lib/schema"
)

// GetTransactionRecord get tx record
func GetTransactionRecord(env *TxEnv, op int) schema.Record {
	tx := ConvertTransaction(env)
	txRecord := transaction.TxRecord{
		IKey: transaction.NewTransactionKey(tx.Hash),
		Op:   schema.NewOp(op),
		Tx:   tx,
	}
	return &txRecord
}

// ConvertTransaction ConvertTransaction
func ConvertTransaction(env *TxEnv) *transaction.Transaction {
	tx := env.Block.Block.Txs[env.TxIndex]
	receipt := env.Block.Receipts[env.TxIndex]
	amount, err := tx.Amount()
	if err != nil {
		amount = 0
	}
	tx2 := transaction.Transaction{
		HeightIndex: HeightIndex(env.Block.Block.Height, env.TxIndex),
		Block: &transaction.Block{
			Height:    env.Block.Block.Height,
			BlockTime: env.Block.Block.BlockTime,
			BlockHash: env.BlockHash,
		},

		Success:    receipt.Ty == types.ExecOk,
		Index:      env.TxIndex,
		Hash:       common.ToHex(tx.Hash()),
		From:       chaincfg.AddressConvert(tx.From()),
		To:         chaincfg.AddressConvert(tx.GetRealToAddr()),
		Amount:     amount,
		Fee:        tx.Fee,
		Execer:     string(tx.Execer),
		ActionName: tx.ActionName(),
		GroupCount: int64(tx.GroupCount),
		IsWithdraw: tx.IsWithdraw(string(tx.GetExecer())),
		Assets:     transaction.GenAssets(tx),
		Next:       common.ToHex(tx.Next),
		IsPara:     strings.HasPrefix(string(tx.Execer), "user.p."),
	}

	if string(tx.Execer) == "none" && receipt.Ty == types.ExecPack {
		tx2.Success = true
	}

	return &tx2
}
