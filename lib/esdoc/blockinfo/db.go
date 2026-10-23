package blockinfo

import (
	"encoding/json"

	"github.com/33cn/externaldb/lib/schema"
)

const (
	TableName = "block_info"
)

type BlockInfo struct {
	Height    int64  `json:"height"`
	TxCount   int    `json:"tx_count"`
	Hash      string `json:"hash"`
	BlockTime int64  `json:"block_time"`
	From      string `json:"from"`
}

// BlockRecord 用于db记录区块基本信息
type BlockRecord struct {
	*schema.IKey
	*schema.Op
	Block *BlockInfo
}

// Value impl
func (r *BlockRecord) Value() []byte {
	v, _ := json.Marshal(r.Block)
	return v
}

// NewBlockRecord create BlockRecord
func NewBlockRecord(block *BlockInfo, op int) *BlockRecord {
	return &BlockRecord{
		IKey:  schema.NewIKey(TableName, TableName, block.Hash),
		Op:    schema.NewOp(op),
		Block: block,
	}
}
