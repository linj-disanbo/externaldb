package db

import (
	"encoding/json"
	"time"

	"github.com/33cn/externaldb/lib/schema"
)

// FilePart 文件分片
type FilePart struct {
	Data       string `json:"data"`
	TxHash     string `json:"tx_hash"`
	InsertTime int64  `json:"insert_time"`
}

// Key 主键
func (f *FilePart) Key() string {
	return AddKeyPrefix(f.TxHash)
}

// RecordKey record key
func (f *FilePart) RecordKey() *schema.IKey {
	return schema.NewIKey(TableName, TableName, f.Key())
}

// RecordFilePart record
type RecordFilePart struct {
	*schema.IKey
	*schema.Op
	value *FilePart
}

// Value value
func (r *RecordFilePart) Value() []byte {
	if r.value.InsertTime == 0 {
		r.value.InsertTime = time.Now().Unix()
	}
	v, _ := json.Marshal(r.value)
	return v
}

// NewRecordFilePart new RecordFilePart
func NewRecordFilePart(op int, value *FilePart) *RecordFilePart {
	return &RecordFilePart{
		IKey:  value.RecordKey(),
		Op:    schema.NewOp(op),
		value: value,
	}
}
