package filepart

import (
	"errors"

	"github.com/33cn/chain33/common"
	l "github.com/33cn/chain33/common/log/log15"
	converter "github.com/33cn/externaldb/convert"
	fpdb "github.com/33cn/externaldb/convert/filepart/db"
	proofconfig "github.com/33cn/externaldb/convert/proof_config"
	"github.com/33cn/externaldb/lib/schema"
	"github.com/33cn/externaldb/util"
)

var log = l.New("module", "db.file_part")

// Convert tx convert
type Convert struct {
	symbol string
	title  string

	ConfigDB proofconfig.PrivilegeDB
}

// ConvertTx impl
func (c *Convert) convertTx(env *converter.TxEnv, op int) ([]schema.Record, error) {
	tx := env.Block.Block.Txs[env.TxIndex]

	records := make([]schema.Record, 0)

	// 签名地址是否有权限,没有直接返回
	if !c.ConfigDB.IsHaveProofPermission(util.AddressConvert(tx.From())) {
		log.Error("IsHaveProofPermission", "err", errors.New("ErrNoPermission"))
		return records, nil
	}

	rdFilePart := fpdb.NewRecordFilePart(op, &fpdb.FilePart{Data: string(tx.Payload), TxHash: common.ToHex(tx.Hash())})
	records = append(records, rdFilePart)
	return records, nil
}
