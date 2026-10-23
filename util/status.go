package util

import (
	"encoding/json"

	l "github.com/33cn/chain33/common/log/log15"
	"github.com/33cn/externaldb/escli"
	"github.com/33cn/externaldb/lib/esdoc/block"
	"github.com/33cn/externaldb/lib/schema"
)

var log = l.New("module", "util")

// dbStatus//lastHeader  hash
// dbStatus//Header -> keys[prev,current]

// GetLastSyncSeq 获得最后同步的seq
func LastSyncSeq(client escli.ESClient, id string) (int64, error) {
	result, err := client.Get(schema.LastSeqDB, schema.LastSeqDB, id)
	if err != nil {
		if err == schema.ErrDBNotFound {
			// 第一个连上
			return -1, nil
		}
		return 0, err
	}
	var seq block.LastSyncSeq
	err = json.Unmarshal([]byte(*result), &seq)
	if err != nil {
		return 0, err
	}

	return seq.SyncSeq, nil
}

//func SaveLastSyncSeq(client *escli.ESClient, id string, seq int64) error {
//	record := newLastRecord(schema.LastSeqDB, schema.LastSeqDB, id, seq)
//	return client.Update(record.Index(), record.Type(), record.ID(), string(record.Value()))
//}

// NewLastRecord create LastSyncSeqRecord
func NewLastRecord(id string, seq int64) *block.LastSyncSeqRecord {
	s := &block.LastSyncSeq{SyncSeq: seq}
	return &block.LastSyncSeqRecord{
		IKey: schema.NewIKey(schema.LastSeqDB, schema.LastSeqDB, id),
		Op:   schema.NewOp(schema.OpAdd),
		Seq:  s,
	}
}

// Save 保存convert后的结果到ES
func SaveToES(client escli.ESClient, blockItems []schema.Record) error {
	if len(blockItems) == 0 {
		return nil
	}
	var rs []schema.Record
	for i, v := range blockItems {
		rs = append(rs, v)
		log.Debug("save", "op", v.OpType(), "idx", i, "ID", v.Key(), "v", string(v.Value()))
	}
	return client.BulkUpdate(rs)
}

func SaveToESSelectBulk(client escli.ESClient, blockItems []schema.Record, bulk bool) error {
	if len(blockItems) == 0 {
		return nil
	}
	if bulk {
		var rs []schema.Record
		for i, v := range blockItems {
			rs = append(rs, v)
			log.Info("SaveToESSelectBulk bulk", "op", v.OpType(), "idx", i, "ID", v.Key(), "v", string(v.Value()))
		}
		return client.BulkUpdate(rs)
	}

	for i, v := range blockItems {
		err := client.Update(v.Index(), v.Type(), v.ID(), string(v.Value()))
		log.Info("SaveToESSelectBulk", "idx", i, "ID", v.Key(), "value", string(v.Value()))
		if err != nil {
			log.Error("SaveToESSelectBulk", "idx", i, "ID", v.Key(), "err", err)
			return err
		}
	}
	return nil
}
