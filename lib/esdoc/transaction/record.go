package transaction

import (
	"encoding/json"

	"github.com/33cn/externaldb/lib/schema"
)

// unfreeze db
const (
	TransactionX  = "transaction"
	DefaultType   = "_doc"
	RTransactionX = "transaction"
)

// TxRecord 用于db 记录
type TxRecord struct {
	*schema.IKey
	*schema.Op
	Tx *Transaction
}

// Value impl
func (r *TxRecord) Value() []byte {
	v, _ := json.Marshal(r.Tx)
	return v
}

// NewTransactionKey NewTransactionKey
func NewTransactionKey(id string) *schema.IKey {
	return schema.NewIKey(TransactionX, TransactionX, id)
}
