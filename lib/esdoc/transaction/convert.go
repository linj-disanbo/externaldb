package transaction

import (
	"github.com/33cn/chain33/types"
	"github.com/33cn/externaldb/lib/schema"
	"github.com/33cn/externaldb/util"
)

// InitDB init db
func InitDB(cli schema.DBCreator) error {
	return util.InitIndex(cli, TransactionX, TransactionX, TxRecordMapping)
}

// GenAssets 交易资产列表
func GenAssets(tx *types.Transaction) []Asset {
	assets, err := tx.Assets()
	if err != nil {
		return nil
	}
	output := make([]Asset, 0)
	for _, a := range assets {
		output = append(output, Asset{Exec: a.Exec, Symbol: a.Symbol, Amount: a.Amount})
	}
	return output
}
