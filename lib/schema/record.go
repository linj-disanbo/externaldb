package schema

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/33cn/chain33/types"

	"github.com/33cn/externaldb/lib/escli/querypara"
)

// TxEnv 转换上下文：一条交易在区块中的位置
type TxEnv struct {
	Block     *types.BlockDetail
	TxIndex   int64
	BlockHash string
}

// HeightIndex 高度+交易下标合成一个可排序的序号
func HeightIndex(height, index int64) int64 {
	return height*types.MaxTxsPerBlock + index
}

// WrapDB 虚拟合约访问数据库的接口（转换器可用，实现在 escli）
type WrapDB interface {
	Get(k1, k2, id string) (*json.RawMessage, error)
	List(k1, k2 string, keyValue []*ListKV) ([]*json.RawMessage, error)
	Set(k1, k2, id string, r Record) error
	Search(idx, typ string, query *querypara.Query, decode func(x *json.RawMessage) (interface{}, error)) ([]interface{}, error)
}

// NeedWrapDB 看插件是否需要数据库
type NeedWrapDB interface {
	SetDB(w WrapDB) error
}

var version int32 = 7

// Key 获得相关的key
type Key interface {
	Index() string
	Type() string
	ID() string
	Key() string
}

// Operator for db Operator
type Operator interface {
	OpType() int
}

// Record 要保存到外部数据的数据结构都要满足这个接口
type Record interface {
	Key
	Operator
	Value() []byte
}

type SourceAbleRecord interface {
	Record
	Source() interface{}
}

// SeqType
const (
	SeqTypeAdd = 1
	SeqTypeDel = 2
)

// DB operator
const (
	OpAdd    = 1
	OpDel    = 2
	OpUpdate = 3
)

// DB Type
const (
	DatabaseTypeEs        = 1
	DatabaseTypeChainJRPC = 2
	DatabaseTypeChainGRPC = 3
)

// IKey impl Key
type IKey struct {
	index, typ, id string
}

func SetVersion(v int32) {
	version = v
}

// Index impl Key
func (d *IKey) Index() string {
	return d.index
}

// Type impl Key
func (d *IKey) Type() string {
	return d.typ
}

// ID impl Key
func (d *IKey) ID() string {
	return d.id
}

// Key impl Key
func (d *IKey) Key() string {
	return fmt.Sprintf("%s/%s/%s", d.index, d.typ, d.id)
}

// NewIKey create key
// ES7 起文档路径是 index/_doc/id（ES8/9 已无 mapping type，同样如此）。
// 版本差异若出现，在这里按 version 分支。
func NewIKey(index, typ, id string) *IKey {
	switch version {
	case 7:
		return &IKey{index: index, typ: "_doc", id: id}
	default:
		return &IKey{index: index, typ: "_doc", id: id}
	}
}

// Op impl Op
type Op struct {
	opType int
}

// Op impl Op
func (d *Op) OpType() int {
	return d.opType
}

// NewOp create Op
func NewOp(t int) *Op {
	return &Op{opType: t}
}

// Block info
type Block struct {
	Height      int64  `json:"height"`
	Ts          int64  `json:"ts"`
	BlockHash   string `json:"block_hash"`
	Index       int64  `json:"index"`
	Send        string `json:"send"`
	TxHash      string `json:"tx_hash"`
	HeightIndex int64  `json:"height_index"`
}

// ListKV 键值对（WrapDB.List 用）
type ListKV struct {
	Key   string
	Value interface{}
}

// 存储层错误
var (
	ErrDBNotFound         = errors.New("DB Not Found")
	ErrDBBadParam         = errors.New("DB Bad Param")
	ErrDBInvalidOperation = errors.New("DB invalid operation")
)
