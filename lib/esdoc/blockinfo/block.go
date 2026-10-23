package blockinfo

import (
	"github.com/33cn/chain33/types"
	esmaint "github.com/33cn/externaldb/lib/esmaint"
	"github.com/33cn/externaldb/lib/schema"
)

// InitDB init db
func InitDB(cli schema.DBCreator) error {
	err := esmaint.InitIndex(cli, TableName, TableName, Mapping)
	return err
}

func SaveBlock(block *types.Block, hash string, op int) (schema.Record, error) {
	blockInfo := BlockInfo{
		Height:    block.Height,
		TxCount:   len(block.Txs),
		Hash:      hash,
		BlockTime: block.BlockTime,
		From:      block.Txs[0].From(),
	}

	return NewBlockRecord(&blockInfo, op), nil
}
