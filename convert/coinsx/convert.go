package coinsx

import (
	"github.com/33cn/chain33/common"
	l "github.com/33cn/chain33/common/log/log15"
	"github.com/33cn/chain33/types"
	converter "github.com/33cn/externaldb/convert"
	chaincfg "github.com/33cn/externaldb/lib/chaincfg"
	"github.com/33cn/externaldb/lib/esdoc/account"
	"github.com/33cn/externaldb/lib/esdoc/transaction"
	esmaint "github.com/33cn/externaldb/lib/esmaint"
	"github.com/33cn/externaldb/lib/schema"
	"github.com/33cn/externaldb/lib/util"
	coinTy "github.com/33cn/plugin/plugin/dapp/coinsx/types"
	"github.com/pkg/errors"
)

type coinsxConvert struct {
	symbol string
	title  string

	env *converter.TxEnv

	tx      *types.Transaction
	receipt *types.ReceiptData
	block   *schema.Block
	account account.Account
	manager Manager
}

var log = l.New("module", "db.coinsx")

func init() {
	converter.RegisterConvert("coinsx", NewConvert)
}

// NewConvert NewConvert
func NewConvert(paraTitle, symbol string, supports []string) converter.ExecConvert {
	e := &coinsxConvert{symbol: symbol, title: paraTitle}

	return e
}

// InitDB init db
func (t *coinsxConvert) InitDB(cli schema.DBCreator) error {
	var err error

	err = account.InitDB(cli)
	if err != nil {
		return err
	}

	err = transaction.InitDB(cli)
	if err != nil {
		return err
	}

	return esmaint.InitIndex(cli, CoinsxManagerDBX, CoinsxManagerTableX, CoinsxManagerMapping)
}

// Convert Convert
func (t *coinsxConvert) ConvertTx(env *converter.TxEnv, op int) ([]schema.Record, error) {
	log.Info("convertTX", "position", t.positionID(env))

	t.env = env
	receipt := env.Block.Receipts[env.TxIndex]
	t.tx = env.Block.Block.Txs[env.TxIndex]
	t.receipt = receipt
	t.block = converter.SetupBlock(env, chaincfg.AddressConvert(t.tx.From()), common.ToHex(t.tx.Hash()))

	var records []schema.Record

	tx := converter.ConvertTransaction(env)
	txRecord := transaction.TxRecord{
		IKey: transaction.NewTransactionKey(tx.Hash),
		Op:   schema.NewOp(op),
		Tx:   tx,
	}
	if t.receipt.Ty != types.ExecOk {
		records = append(records, &txRecord)
		return records, nil
	}

	var action coinTy.CoinsxAction
	err := types.Decode(t.tx.Payload, &action)
	if err != nil {
		return nil, err
	}

	// tx option setup here, if needed
	records = append(records, &txRecord)

	t.account = account.Account{
		AssetSymbol: t.symbol,
		AssetExec:   account.ExecCoinsxX,
		HeightIndex: converter.HeightIndex(t.block.Height, t.block.Index),
		Height:      t.block.Height,
		BlockTime:   t.env.Block.Block.BlockTime,
	}

	for _, l := range receipt.Logs {
		if l.Ty == coinTy.TyCoinsxManagerStatusLog {
			t.manager = Manager{
				HeightIndex: converter.HeightIndex(t.block.Height, t.block.Index),
			}
			record, err := LogManagerStatusConvert(l.Log, op, t.manager)
			if err != nil {
				log.Error("convertTX", "position", t.positionID(env), "ManagerStatusLog err", err)
			}
			records = append(records, record)
		} else {
			rs, err2 := account.RecordHelper(l, op, t.account)
			if err2 != nil {
				log.Info("convertTX", "position", t.positionID(env), "logType", l.Ty, "err", err2)
				continue
			}
			records = append(records, rs...)
		}

	}
	return records, nil
}

func (t *coinsxConvert) positionID(env *converter.TxEnv) string {
	return util.PositionID("coinsx", env.Block.Block.Height, env.TxIndex)
}

func LogManagerStatusConvert(v []byte, op int, manager Manager) (schema.Record, error) {
	var l coinTy.ReceiptManagerStatus
	err := types.Decode(v, &l)
	if err != nil {
		return nil, errors.Wrap(err, "coinsx Decode manager status log")
	}

	if op == schema.SeqTypeDel {
		manager.ManagerStatus = l.Prev
	} else {
		manager.ManagerStatus = l.Curr
	}

	r := &Record{
		IKey:    newCoinsxKey(l.Curr.TransferFlag),
		Op:      schema.NewOp(schema.OpAdd),
		Manager: Manager{manager.ManagerStatus, manager.HeightIndex},
	}

	return r, nil
}
