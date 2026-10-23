package account

import (
	"github.com/33cn/chain33/types"
	esmaint "github.com/33cn/externaldb/lib/esmaint"
	"github.com/33cn/externaldb/lib/schema"
)

// InitDB init account db
func InitDB(cli schema.DBCreator) error {
	err := esmaint.InitIndex(cli, AccountRecordDBX, AccountRecordTableX, AccountMapping)
	if err != nil {
		return err
	}

	return esmaint.InitIndex(cli, DBX, TableX, AccountMapping)
}

// RecordHelper 把一条 receipt 资产日志（fee/transfer/exec-transfer 等）转成一对文档：
//
//	account/<addr>-<exec|assetExec>-<assetExec>:<symbol>
//	    账户当前态；id 不含高度，Op 恒为 OpAdd（upsert，后写覆盖前写）。
//	account_record/<addr>-<exec|assetExec>-<assetExec>-<height>:<symbol>
//	    按高度的历史快照（供按高度查余额）；Op 跟随 seq 类型，即这里的 op 参数。
//
// 回滚（op = schema.SeqTypeDel）时 detail 取日志里的 Prev：account 文档被写回交易前的状态，
// account_record 则以 OpDel 按 id 里的高度删掉该高度快照。
// 同一笔交易里同一地址可能产生多条日志（如 coins 先扣手续费 TyLogFee、再转出 TyLogTransfer），
// 两条日志的记录 id 相同，最终只保留最后一条日志的态（见 db/coins Test_ConvertTx 的 1/2 与 3/4）。
func RecordHelper(log *types.ReceiptLog, op int, acc Account) ([]schema.Record, error) {
	var records []schema.Record

	accountDetail, err := AssetLogConvert(log.Ty, log.Log, op)
	if err != nil {
		return nil, err
	}
	acc.Detall = accountDetail
	record := &Record{Acc: acc, IKey: NewAccountKey(acc.Key()), Op: schema.NewOp(schema.OpAdd)}
	records = append(records, record)

	rrecord := &Record{Acc: acc, IKey: NewAccountRecordKey(acc.RecordKey()), Op: schema.NewOp(op)}
	records = append(records, rrecord)
	return records, nil
}
