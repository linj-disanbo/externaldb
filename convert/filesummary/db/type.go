package db

import (
	"encoding/json"

	converter "github.com/33cn/externaldb/convert"
	"github.com/33cn/externaldb/lib/schema"
)

// FileInfo block tx payload
type FileInfo struct {
	FileType  string `json:"file_type"`
	FileHash  string `json:"file_hash"`
	FileSize  int64  `json:"file_size"`
	PartType  int32  `json:"part_type"` // 1:base64
	PartHashs []byte `json:"part_hashs"`
}

// BlacklistInfo block tx payload
type BlacklistInfo struct {
	FileHash string `json:"file_hash"`
	Note     string `json:"note"`
	Operate  int64  `json:"operate"`
}

type Blacklist struct {
	FileBlacklistFlag bool   `json:"file_blacklist_flag"`
	FileBlacklist     string `json:"file_blacklist"`
	FileBlacklistNote string `json:"file_blacklist_note"`
}

// FileSummary 文件汇总
type FileSummary struct {
	FileInfo
	Blacklist
	*schema.Block
}

// Key 主键
func (s *FileSummary) Key() string {
	return AddKeyPrefix(s.FileHash)
}

// RecordKey 文件汇总
func (s *FileSummary) RecordKey() *schema.IKey {
	return schema.NewIKey(TableName, TableName, s.Key())
}

// NewSummary new summary
func NewSummary(p FileInfo, env *converter.TxEnv) *FileSummary {
	var sum FileSummary
	sum.FileInfo = p
	sum.Block = converter.NewBlock(env)
	sum.FileBlacklistFlag = false
	sum.FileBlacklist = ""
	sum.FileBlacklistNote = ""
	return &sum
}

// RecordSummary record
type RecordSummary struct {
	*schema.IKey
	*schema.Op
	value *FileSummary
}

// Value value
func (r *RecordSummary) Value() []byte {
	v, _ := json.Marshal(r.value)
	return v
}

// NewRecordSummary new record
func NewRecordSummary(op int, value *FileSummary) *RecordSummary {
	return &RecordSummary{
		IKey:  value.RecordKey(),
		Op:    schema.NewOp(op),
		value: value,
	}
}
