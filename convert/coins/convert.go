package coins

import (
	"github.com/33cn/chain33/common"
	l "github.com/33cn/chain33/common/log/log15"
	pty "github.com/33cn/chain33/system/dapp/coins/types"
	"github.com/33cn/chain33/types"
	converter "github.com/33cn/externaldb/convert"
	"github.com/33cn/externaldb/lib/esdoc/account"
	"github.com/33cn/externaldb/lib/esdoc/transaction"
	"github.com/33cn/externaldb/lib/schema"
	"github.com/33cn/externaldb/util"
)

type coinsConvert struct {
	symbol string
	title  string

	env *converter.TxEnv

	tx         *types.Transaction
	receipt    *types.ReceiptData
	block      *schema.Block
	accountBty account.Account
}

var log = l.New("module", "db.coins")

func init() {
	converter.RegisterConvert("coins", NewConvert)
}

// NewConvert NewConvert
func NewConvert(paraTitle, symbol string, supports []string) converter.ExecConvert {
	e := &coinsConvert{symbol: symbol, title: paraTitle}

	return e
}

// InitDB init db
func (t *coinsConvert) InitDB(cli schema.DBCreator) error {
	var err error

	err = account.InitDB(cli)
	if err != nil {
		return err
	}

	err = transaction.InitDB(cli)
	if err != nil {
		return err
	}

	return nil
}

// Convert Convert
func (t *coinsConvert) ConvertTx(env *converter.TxEnv, op int) ([]schema.Record, error) {
	log.Info("convertTX", "position", t.positionID(env))

	t.env = env
	receipt := env.Block.Receipts[env.TxIndex]
	t.tx = env.Block.Block.Txs[env.TxIndex]
	t.receipt = receipt
	t.block = converter.SetupBlock(env, util.AddressConvert(t.tx.From()), common.ToHex(t.tx.Hash()))

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

	var action pty.CoinsAction
	err := types.Decode(t.tx.Payload, &action)
	if err != nil {
		return nil, err
	}

	// tx option setup here, if needed
	records = append(records, &txRecord)

	t.accountBty = account.Account{
		AssetSymbol: t.symbol,
		AssetExec:   account.ExecCoinsX,
		HeightIndex: converter.HeightIndex(t.block.Height, t.block.Index),
		Height:      t.block.Height,
		BlockTime:   t.env.Block.Block.BlockTime,
	}

	for _, l := range receipt.Logs {
		rs, err2 := account.RecordHelper(l, op, t.accountBty)
		if err2 != nil {
			log.Info("convertTX", "position", t.positionID(env), "logType", l.Ty, "err", err2)
			continue
		}
		records = append(records, rs...)
	}
	return records, nil
}

func (t *coinsConvert) positionID(env *converter.TxEnv) string {
	return util.PositionID("coins", env.Block.Block.Height, env.TxIndex)
}
