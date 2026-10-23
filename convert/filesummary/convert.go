package filesummary

import (
	"encoding/json"
	"errors"

	"github.com/33cn/chain33/common"
	l "github.com/33cn/chain33/common/log/log15"
	converter "github.com/33cn/externaldb/convert"
	fsdb "github.com/33cn/externaldb/convert/filesummary/db"
	proofconfig "github.com/33cn/externaldb/convert/proof_config"
	"github.com/33cn/externaldb/lib/schema"
	"github.com/33cn/externaldb/util"
)

var log = l.New("module", "db.file")

const (
	BlacklistTx = "user.file_blacklist"
	OpBlacklist = 0
	OpRecover   = 1
)

// Convert tx convert
type Convert struct {
	symbol string
	title  string

	FileDB   fsdb.DB
	ConfigDB proofconfig.PrivilegeDB
}

// ConvertTx impl
func (c *Convert) convertTx(env *converter.TxEnv, op int) ([]schema.Record, error) {
	tx := env.Block.Block.Txs[env.TxIndex]

	var err error
	records := make([]schema.Record, 0)
	records = append(records, converter.GetTransactionRecord(env, op))

	// 签名地址是否有权限,没有直接返回
	fromAddr := util.AddressConvert(tx.From())
	if !c.ConfigDB.IsHaveProofPermission(fromAddr) {
		log.Error("IsHaveProofPermission", "err", errors.New("ErrNoPermission"))
		return records, nil
	}

	if string(tx.Execer) == c.title+BlacklistTx {
		log.Debug("deal blacklist file ", "execer", string(tx.Execer))
		bfile, berr := c.blacklistFile(env, op)
		err = berr
		if err == nil {
			records = append(records, bfile...)
		} else {
			log.Error("convertTX:blacklistFile", "err", err)
		}
	} else {
		log.Debug("deal add file ", "execer", string(tx.Execer))
		file, aerr := c.addFile(env, op)
		err = aerr
		if err == nil {
			records = append(records, file...)
		} else {
			log.Error("convertTX:addFile", "err", err)
		}
	}

	return records, nil
}

// addFile 文件上链
func (c *Convert) addFile(env *converter.TxEnv, op int) ([]schema.Record, error) {
	tx := env.Block.Block.Txs[env.TxIndex]

	var records []schema.Record

	var file fsdb.FileInfo
	if err := json.Unmarshal(tx.Payload, &file); err != nil {
		log.Error("json.Unmarshal(tx.Payload, &file)", "err", err)
		return nil, err
	}

	sum := fsdb.NewSummary(file, env)
	records = append(records, fsdb.NewRecordSummary(op, sum))
	return records, nil
}

// blacklistFile 对于文件的移入和移除黑名单的操作
func (c *Convert) blacklistFile(env *converter.TxEnv, op int) ([]schema.Record, error) {
	tx := env.Block.Block.Txs[env.TxIndex]

	var records []schema.Record

	var blacklist fsdb.BlacklistInfo
	if err := json.Unmarshal(tx.Payload, &blacklist); err != nil {
		log.Error("json.Unmarshal(tx.Payload, &blacklist)", "err", err)
		return nil, err
	}

	sumFile, err := c.FileDB.Get(blacklist.FileHash)
	if err != nil {
		return nil, err
	}

	file := fsdb.NewBlackFile(sumFile)
	if blacklist.Operate == OpBlacklist {
		if sumFile.FileBlacklistFlag {
			return nil, fsdb.ErrFileBlacklisted
		}
		file.Black(op, common.ToHex(tx.Hash()), blacklist.Note)
	} else if blacklist.Operate == OpRecover {
		if !sumFile.FileBlacklistFlag {
			return nil, fsdb.ErrFileNotBlacklisted
		}
		file.Recover(op, common.ToHex(tx.Hash()), blacklist.Note)
	} else {
		return nil, fsdb.ErrInvalidOperate
	}

	records = append(records, fsdb.NewRecordSummary(op, sumFile))
	return records, nil
}
