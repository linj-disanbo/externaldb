package evm

import (
	"encoding/json"

	"github.com/33cn/externaldb/lib/schema"
)

// db
const (
	EVMX         = "evm"
	EVMTokenX    = "evm_token"
	EVMTransferX = "evm_transfer"
)

// RecordEVM db evm
type RecordEVM struct {
	*schema.IKey
	*schema.Op
	value EVM
}

// Value impl
func (r *RecordEVM) Value() []byte {
	v, _ := r.value.ToJSON()
	return v
}

func NewEVMKey(id string) *schema.IKey {
	return schema.NewIKey(EVMX, EVMX, id)
}

func NewRecordToken(token *Token, op int) *RecordToken {
	return &RecordToken{
		IKey:  schema.NewIKey(EVMTokenX, EVMTokenX, token.Key()),
		Op:    schema.NewOp(op),
		value: token,
	}
}

type RecordToken struct {
	*schema.IKey
	*schema.Op
	value *Token
}

// Value impl
func (r *RecordToken) Value() []byte {
	v, _ := json.Marshal(r.value)
	return v
}

type RecordTransfer struct {
	*schema.IKey
	*schema.Op
	value *Transfer
}

// Value impl
func (r *RecordTransfer) Value() []byte {
	v, _ := json.Marshal(r.value)
	return v
}

func NewRecordTransfer(token *Transfer, op int) *RecordTransfer {
	return &RecordTransfer{
		IKey:  schema.NewIKey(EVMTransferX, EVMTransferX, token.Key()),
		Op:    schema.NewOp(op),
		value: token,
	}
}

func (r *RecordTransfer) Source() interface{} {
	return r.value
}
