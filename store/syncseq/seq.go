package syncseq

import (
	"encoding/json"
	"fmt"

	l "github.com/33cn/chain33/common/log/log15"

	"github.com/33cn/externaldb/escli"
	"github.com/33cn/externaldb/lib/esdoc/block"
	"github.com/33cn/externaldb/lib/proto"
	"github.com/33cn/externaldb/lib/schema"
	"github.com/33cn/externaldb/store"
	"github.com/33cn/externaldb/util"
)

var log = l.New("module", "syncSeq")

type seqNumStore struct {
	seqNumClient escli.ESClient
}

type esSeqStore struct {
	esSeqClient escli.ESClient
	bulk        bool
}

// NewEsSeqStore  SeqStore（只支持 ES 存储；Kafka/MQ 存储已移除）
func NewSeqStore(cfg *proto.ConfigNew) (store.SeqNumStore, store.SeqStore, error) {
	client, err := escli.NewESLongConnect(cfg.SyncEs.Host, cfg.SyncEs.Prefix, cfg.EsVersion, cfg.SyncEs.User, cfg.SyncEs.Pwd)
	if err != nil {
		return nil, nil, err
	}
	seqNumClient := &seqNumStore{
		seqNumClient: client,
	}
	err = util.InitIndex(client, block.StatusDB, block.StatusDB, block.Mapping)
	if err != nil {
		return seqNumClient, nil, err
	}
	return seqNumClient, &esSeqStore{
		esSeqClient: client,
		bulk:        cfg.SyncEs.Bulk,
	}, nil
}

// NewSeqStoreWithFileProgress 创建 SeqStore（ES 存储区块数据）+ SeqNumStore（本地文件存储进度）。
// workDir 为二进制所在目录，进度文件将写入 <workDir>/data/last_sync。
func NewSeqStoreWithFileProgress(cfg *proto.ConfigNew, workDir string) (store.SeqNumStore, store.SeqStore, error) {
	client, err := escli.NewESLongConnect(cfg.SyncEs.Host, cfg.SyncEs.Prefix, cfg.EsVersion, cfg.SyncEs.User, cfg.SyncEs.Pwd)
	if err != nil {
		return nil, nil, err
	}

	progressPath := ProgressFilePath(workDir, DefaultSyncProgressFile)
	seqNumClient, err := NewFileSeqNumStore(progressPath)
	if err != nil {
		return nil, nil, err
	}

	err = util.InitIndex(client, block.StatusDB, block.StatusDB, block.Mapping)
	if err != nil {
		return seqNumClient, nil, err
	}
	return seqNumClient, &esSeqStore{
		esSeqClient: client,
		bulk:        cfg.SyncEs.Bulk,
	}, nil
}

// NewEsSeqStore  SeqStore
func NewGetSeq(cfg *proto.ConfigNew) (store.SeqNumStore, store.SeqStore, error) {
	client, err := escli.NewESLongConnect(cfg.SyncEs.Host, cfg.SyncEs.Prefix, cfg.EsVersion, cfg.SyncEs.User, cfg.SyncEs.Pwd)
	if err != nil {
		return nil, nil, err
	}
	seqNumClient := &seqNumStore{
		seqNumClient: client,
	}
	return seqNumClient, &esSeqStore{
		esSeqClient: client,
	}, nil
}

// LastSeq LastSeq
func (s *seqNumStore) LastSeq() (*store.SeqNum, error) {
	num, err := util.LastSyncSeq(s.seqNumClient, block.SyncSeq)
	if err != nil {
		return nil, err
	}
	return &store.SeqNum{Number: num}, nil
}

// UpdateLastSeq 更新最后同步的seq
func (s *seqNumStore) UpdateLastSeq(seq schema.Record) error {
	err := s.seqNumClient.Update(seq.Index(), seq.Type(), seq.ID(), string(seq.Value()))
	log.Debug("save", "ID", seq.Key(), "value", string(seq.Value()))
	if err != nil {
		log.Error("save", "ID", seq.Key(), "err", err)
		return err
	}
	return nil
}

// SaveSeqs 输出同步区块信息到ES
func (s *esSeqStore) SaveSeqs(blockItems []schema.Record) error {
	if len(blockItems) == 0 {
		return nil
	}
	if s.bulk {
		var rs []schema.Record
		for i, v := range blockItems {
			rs = append(rs, v)
			log.Debug("save", "op", v.OpType(), "idx", i, "ID", v.Key(), "v", string(v.Value()))
		}
		return s.esSeqClient.BulkUpdate(rs)
	}

	for i, v := range blockItems {
		err := s.esSeqClient.Update(v.Index(), v.Type(), v.ID(), string(v.Value()))
		log.Debug("save", "idx", i, "ID", v.Key(), "value", string(v.Value()))
		if err != nil {
			log.Error("save", "idx", i, "ID", v.Key(), "err", err)
			return err
		}
	}
	return nil
}

// GetSeq get block from es
func (s *esSeqStore) GetSeq(seqNum int64) (*block.Seq, error) {
	id := fmt.Sprintf("%d", seqNum)
	result, err := s.esSeqClient.Get(block.StatusDB, block.StatusDB, id)
	if err != nil {
		return nil, err
	}
	var seq block.Seq
	err = json.Unmarshal([]byte(*result), &seq)
	if err != nil {
		return nil, err
	}

	return &seq, nil
}

// CommitSeqAck continue next seq
func (s *esSeqStore) CommitSeqAck(seqNum int64) int64 {
	return seqNum + 1
}
