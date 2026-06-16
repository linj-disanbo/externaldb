package syncseq

import (
	"encoding/json"
	"path/filepath"

	"github.com/33cn/externaldb/db"
	"github.com/33cn/externaldb/db/block"
	"github.com/33cn/externaldb/store"
	"github.com/33cn/externaldb/util/localfile"
)

// DefaultSyncProgressPath 默认的 sync 进度文件路径（相对于工作目录）。
// 最终路径为 <workDir>/data/last_sync。
const DefaultSyncProgressFile = "last_sync"

// DefaultConvertProgressFile 默认的 convert 进度文件名。
const DefaultConvertProgressFile = "last_convert"

// DefaultDataDir 默认的进度文件存放目录。
const DefaultDataDir = "data"

// ProgressFilePath 根据 workDir 和文件名返回完整的进度文件路径。
func ProgressFilePath(workDir, fileName string) string {
	return filepath.Join(workDir, DefaultDataDir, fileName)
}

// fileSeqNumStore 基于本地文件的进度存储，实现 store.SeqNumStore 接口。
// 替代原来基于 ES 的 last_seq 文档读写。
type fileSeqNumStore struct {
	fp *localfile.FileProgress
}

// NewFileSeqNumStore 创建基于本地文件的 SeqNumStore。
// path 为进度文件路径，如 /path/to/data/last_sync。
func NewFileSeqNumStore(path string) (store.SeqNumStore, error) {
	fp, err := localfile.NewFileProgress(path)
	if err != nil {
		return nil, err
	}
	return &fileSeqNumStore{fp: fp}, nil
}

// LastSeq 从本地文件读取最后一个同步的 seq。
func (s *fileSeqNumStore) LastSeq() (*store.SeqNum, error) {
	n, err := s.fp.Load()
	if err != nil {
		return nil, err
	}
	return &store.SeqNum{Number: n}, nil
}

// UpdateLastSeq 更新同步进度到本地文件。
func (s *fileSeqNumStore) UpdateLastSeq(seq db.Record) error {
	if seq == nil {
		return s.fp.Save(0)
	}
	v := seq.Value()
	var lastSeq block.LastSyncSeq
	if err := json.Unmarshal(v, &lastSeq); err != nil {
		log.Error("fileSeqNumStore.UpdateLastSeq decode", "err", err, "value", string(v))
		return s.fp.Save(0)
	}
	return s.fp.Save(lastSeq.SyncSeq)
}
