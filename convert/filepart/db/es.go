package db

import (
	"encoding/json"
	"time"

	converter "github.com/33cn/externaldb/convert"
	"github.com/33cn/externaldb/lib/escli"
	"github.com/33cn/externaldb/lib/escli/querypara"
	esmaint "github.com/33cn/externaldb/lib/esmaint"
	"github.com/33cn/externaldb/lib/schema"
)

const (
	TableName         = "file_part"
	KeyPrefix         = TableName + "-"
	FilePartCacheTime = 5 // 5 day
)

// AddKeyPrefix 添加索引前缀
func AddKeyPrefix(hash string) string {
	return KeyPrefix + hash
}

// InitESDB 创建db (插件满足接口 converter.ExecConvert)
func InitESDB(cli schema.DBCreator) error {
	go func() {
		d := NewEsDB(cli.(escli.ESClient))
		t := time.NewTicker(FilePartCacheTime * time.Hour * 24)
		for range t.C {
			if err := d.Clean(); err != nil {
				log.Error("file part cache clean", "err", err)
			}
		}
	}()
	return esmaint.InitIndex(cli, TableName, TableName, FilePartMapping)
}

// NewEsDB new es DB
func NewEsDB(client converter.WrapDB) DB {
	return &esDB{client: client}
}

// EsDB elasticsearch
type esDB struct {
	client converter.WrapDB
}

// Get FilePart
func (d *esDB) Get(hash string) (*FilePart, error) {
	buf, err := d.client.Get(TableName, TableName, AddKeyPrefix(hash))
	if err != nil {
		return nil, err
	}
	var ans FilePart
	err = json.Unmarshal(*buf, &ans)
	return &ans, err
}

// Set a file record
func (d *esDB) Set(r *RecordFilePart) error {
	return d.client.Set(TableName, TableName, r.value.Key(), r)
}

// Clean file part cache in es
func (d *esDB) Clean() error {
	q := querypara.Query{
		Range: []*querypara.QRange{
			{
				Key: "insert_time",
				LT:  time.Now().Unix() - FilePartCacheTime*3600*24,
			},
		},
	}
	return d.client.(escli.ESClient).DeleteByQuery(TableName, TableName, &q)
}
