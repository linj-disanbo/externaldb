package address

import (
	"encoding/json"

	l "github.com/33cn/chain33/common/log/log15"
	"github.com/33cn/externaldb/escli"
	"github.com/33cn/externaldb/lib/schema"
	"github.com/33cn/externaldb/util"
	lru "github.com/hashicorp/golang-lru"
)

const (
	TableName = "address"
	KeyPrefix = TableName + "-"
)

var log = l.New("module", "db.address")
var Manager DB

func Init(client escli.ESClient) error {
	Manager = NewEsDB(client)
	return InitESDB(client)
}

type DB interface {
	Get(addr string) (*Address, error)
	AddTxCount(op int, addr string) (*Address, error)
	AddEvmTransferCount(op int, addr string) (*Address, error)
}

// Record es record
type Record struct {
	*schema.IKey
	*schema.Op
	value *Address
}

// Value value
func (r *Record) Value() []byte {
	v, _ := json.Marshal(r.value)
	return v
}

// NewRecord new RecordFilePart
func NewRecord(op int, value *Address) *Record {
	return &Record{
		IKey:  schema.NewIKey(TableName, TableName, value.Key()),
		Op:    schema.NewOp(op),
		value: value,
	}
}

// AddKeyPrefix 添加索引前缀
func AddKeyPrefix(hash string) string {
	return KeyPrefix + hash
}

// InitESDB 创建db (插件满足接口 converter.ExecConvert)
func InitESDB(cli schema.DBCreator) error {
	return util.InitIndex(cli, TableName, TableName, Mapping)
}

// NewEsDB new es DB
func NewEsDB(client schema.WrapDB) DB {
	cd := &esDB{}
	cd.db = client
	cd.cache, _ = lru.New(1024)
	return cd
}

// EsDB elasticsearch
type esDB struct {
	cache *lru.Cache
	db    schema.WrapDB
}

// Get 加载合约，优先从缓存加载
func (d *esDB) Get(addr string) (*Address, error) {
	if c, ok := d.cache.Get(addr); ok {
		if ct, ok := c.(*Address); ok {
			return ct, nil
		}
		log.Error("contract cache error", "cache", c)
		d.cache.Remove(addr)
	}
	var c = &Address{}
	data, err := d.db.Get(TableName, TableName, AddKeyPrefix(addr))
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(*data, c); err != nil {
		return nil, err
	}
	d.cache.Add(addr, c)
	return c, nil
}

func (d *esDB) AddTxCount(op int, addr string) (*Address, error) {
	ad, err := d.Get(addr)
	switch err {
	case schema.ErrDBNotFound:
		ad = &Address{Address: addr, TxCount: 1, AddrType: AccountPersonage}
		d.cache.Add(addr, ad)
		return ad, nil
	case nil:
		if op == schema.OpAdd {
			ad.TxCount++
		} else {
			ad.TxCount--
		}
		d.cache.Add(addr, ad)
		return ad, nil
	default:
		return nil, err
	}
}

func (d *esDB) AddEvmTransferCount(op int, addr string) (*Address, error) {
	ad, err := d.Get(addr)
	switch err {
	case schema.ErrDBNotFound:
		ad = &Address{Address: addr, EvmTransferCount: 1, AddrType: AccountPersonage}
		d.cache.Add(addr, ad)
		return ad, nil
	case nil:
		if op == schema.OpAdd {
			ad.EvmTransferCount++
		} else {
			ad.EvmTransferCount--
		}
		d.cache.Add(addr, ad)
		return ad, nil
	default:
		return nil, err
	}
}
