package db

import (
	"encoding/json"

	"github.com/33cn/externaldb/lib/schema"
)

// token db
const (
	TokenX    = "nft"
	TransferX = "nft_transfer"
	AccountX  = "nft_account"
)

// RecordToken db token
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

// RecordTransfer db token
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

// RecordAccount db account
type RecordAccount struct {
	*schema.IKey
	*schema.Op
	value *Account
}

// Value impl
func (r *RecordAccount) Value() []byte {
	v, _ := json.Marshal(r.value)
	return v
}

// NewTokenKey NewTokenKey
func NewTokenKey(id string) *schema.IKey {
	return schema.NewIKey(TokenX, TokenX, id)
}

func NewRecordToken(token *Token, op int) *RecordToken {
	return &RecordToken{
		IKey:  NewTokenKey(token.Key()),
		Op:    schema.NewOp(op),
		value: token,
	}
}

func NewTransferKey(id string) *schema.IKey {
	return schema.NewIKey(TransferX, TransferX, id)
}

func NewRecordTransfer(transfer *Transfer, op int) *RecordTransfer {
	return &RecordTransfer{
		IKey:  NewTransferKey(transfer.Key()),
		Op:    schema.NewOp(op),
		value: transfer,
	}
}

func NewAccountKey(id string) *schema.IKey {
	return schema.NewIKey(AccountX, AccountX, id)
}

func NewRecordAccount(acc *Account, op int) *RecordAccount {
	return &RecordAccount{
		IKey:  NewAccountKey(acc.Key()),
		Op:    schema.NewOp(op),
		value: acc,
	}
}
