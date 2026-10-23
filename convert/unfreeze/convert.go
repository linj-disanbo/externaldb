package unfreeze

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
	pty "github.com/33cn/plugin/plugin/dapp/unfreeze/types"
	"github.com/pkg/errors"
)

var log = l.New("module", "db.unfreeze")

// Convert tx
type Convert struct {
	title  string
	symbol string

	env *converter.TxEnv

	block   *schema.Block
	tx      *types.Transaction
	receipt *types.ReceiptData
}

func init() {
	converter.RegisterConvert("unfreeze", NewConvert)
}

// NewConvert NewConvert
func NewConvert(paraTitle, symbol string, supports []string) converter.ExecConvert {
	e := &Convert{symbol: symbol, title: paraTitle}
	return e
}

// InitDB init db
func (e *Convert) InitDB(cli schema.DBCreator) error {
	err := account.InitDB(cli)
	if err != nil {
		return err
	}

	err = transaction.InitDB(cli)
	if err != nil {
		return err
	}

	err = esmaint.InitIndex(cli, UnfreezeTxDBX, UnfreezeTxTableX, TxRecordMapping)
	if err != nil {
		return err
	}

	return nil
}

// ConvertTx impl
func (e *Convert) ConvertTx(env *converter.TxEnv, op int) ([]schema.Record, error) {
	e.env = env
	tx := env.Block.Block.Txs[env.TxIndex]
	var action pty.UnfreezeAction
	err := types.Decode(tx.Payload, &action)
	if err != nil {
		return nil, errors.Wrap(err, "decode to unfreeze action")
	}
	block := converter.SetupBlock(env, chaincfg.AddressConvert(tx.From()), common.ToHex(tx.Hash()))
	receipt := e.env.Block.Receipts[e.env.TxIndex]
	e.block = block
	e.tx = tx
	e.receipt = receipt

	var records []schema.Record
	switch action.Ty {
	case pty.UnfreezeActionCreate:
		log.Debug("Convert", "action", "create")
		records, err = e.convertCreate(&action, op)
	case pty.UnfreezeActionWithdraw:
		log.Debug("Convert", "action", "withdraw")
		records, err = e.convertWithdraw(&action, op)
	case pty.UnfreezeActionTerminate:
		log.Debug("Convert", "action", "terminate")
		records, err = e.convertTerminate(&action, op)
	}

	tx1 := converter.ConvertTransaction(env)
	txRecord := transaction.TxRecord{
		IKey: transaction.NewTransactionKey(tx1.Hash),
		Op:   schema.NewOp(op),
		Tx:   tx1,
	}
	records = append(records, &txRecord)

	return records, err
}

func (e *Convert) convertCreate(action *pty.UnfreezeAction, op int) ([]schema.Record, error) {
	var items []schema.Record

	unfreezeTx := &Tx{
		BlockInfo: e.block,
	}

	txRecord := &TxRecord{
		IKey: schema.NewIKey(UnfreezeTxDBX, UnfreezeTxTableX, e.block.TxHash),
		Op:   schema.NewOp(op),
		tx:   unfreezeTx,
	}

	if e.receipt.Ty != types.ExecOk {
		txRecord.tx.Success = false
		txRecord.tx.ActionType = ActionTypeCreate
		items = append(items, txRecord)
		return items, nil
	}

	logLen := len(e.receipt.Logs)
	unfreezeStatusLog := e.receipt.Logs[logLen-1]

	log.Info("create log", "len", logLen)
	for _, l := range e.receipt.Logs {
		log.Info("create log", "ty", l.Ty)
	}
	var u pty.ReceiptUnfreeze
	err := types.Decode(unfreezeStatusLog.Log, &u)
	if err != nil {
		return items, errors.Wrap(err, "decode create ReceiptUnfreeze")
	}
	txRecord.tx = setUnfreezeCreate(txRecord.tx, &u)
	items = append(items, txRecord)
	return items, nil
}

func (e *Convert) convertWithdraw(action *pty.UnfreezeAction, op int) ([]schema.Record, error) {
	var items []schema.Record

	unfreezeTx := &Tx{
		BlockInfo: e.block,
	}

	txRecord := &TxRecord{
		IKey: schema.NewIKey(UnfreezeTxDBX, UnfreezeTxTableX, e.block.TxHash),
		Op:   schema.NewOp(op),
		tx:   unfreezeTx,
	}
	if e.receipt.Ty != types.ExecOk {
		txRecord.tx.Success = false
		txRecord.tx.ActionType = ActionTypeWithdraw
		items = append(items, txRecord)
		return items, nil
	}

	logLen := len(e.receipt.Logs)
	unfreezeStatusLog := e.receipt.Logs[logLen-1]

	var u pty.ReceiptUnfreeze
	err := types.Decode(unfreezeStatusLog.Log, &u)
	if err != nil {
		return items, errors.Wrap(err, "decode withdraw ReceiptUnfreeze")
	}
	txRecord.tx = setUnfreezeWithdraw(txRecord.tx, &u)
	items = append(items, txRecord)
	return items, nil
}

func (e *Convert) convertTerminate(action *pty.UnfreezeAction, op int) ([]schema.Record, error) {
	var items []schema.Record

	unfreezeTx := &Tx{
		BlockInfo: e.block,
	}

	txRecord := &TxRecord{
		IKey: schema.NewIKey(UnfreezeTxDBX, UnfreezeTxTableX, e.block.TxHash),
		Op:   schema.NewOp(op),
		tx:   unfreezeTx,
	}
	if e.receipt.Ty != types.ExecOk {
		txRecord.tx.Success = false
		txRecord.tx.ActionType = ActionTypeTerminate
		items = append(items, txRecord)
		return items, nil
	}

	logLen := len(e.receipt.Logs)
	unfreezeStatusLog := e.receipt.Logs[logLen-1]

	var u pty.ReceiptUnfreeze
	err := types.Decode(unfreezeStatusLog.Log, &u)
	if err != nil {
		return items, errors.Wrap(err, "decode terminate ReceiptUnfreeze")
	}
	txRecord.tx = setUnfreezeTerminate(txRecord.tx, &u)
	items = append(items, txRecord)
	return items, nil
}

func setUnfreezeCreate(unfreezeTx *Tx, u *pty.ReceiptUnfreeze) *Tx {
	unfreezeTx.ActionType = ActionTypeCreate
	unfreezeTx.Creator = u.Current.Initiator
	unfreezeTx.Beneficiary = u.Current.Beneficiary
	unfreezeTx.UnfreezeID = u.Current.UnfreezeID
	unfreezeTx.Success = true

	unfreezeTx.Create = &ActionCreate{
		StartTime:   u.Current.StartTime,
		AssetExec:   u.Current.AssetExec,
		AssetSymbol: u.Current.AssetSymbol,
		TotalCount:  u.Current.TotalCount,
		Means:       u.Current.Means,
	}
	if u.Current.Means == pty.FixAmountX {
		unfreezeTx.Create.FixAmountOption = &FixAmount{
			Period: u.Current.GetFixAmount().Period,
			Amount: u.Current.GetFixAmount().Amount,
		}
	} else {
		unfreezeTx.Create.LeftProportionOptioin = &LeftProportion{
			Period:        u.Current.GetLeftProportion().Period,
			TenThousandth: u.Current.GetLeftProportion().TenThousandth,
		}
	}
	return unfreezeTx
}

func setUnfreezeWithdraw(unfreezeTx *Tx, u *pty.ReceiptUnfreeze) *Tx {
	unfreezeTx.ActionType = ActionTypeWithdraw
	unfreezeTx.Creator = u.Current.Initiator
	unfreezeTx.Beneficiary = u.Current.Beneficiary
	unfreezeTx.UnfreezeID = u.Current.UnfreezeID
	unfreezeTx.Success = true

	unfreezeTx.Create = &ActionCreate{
		StartTime:   u.Current.StartTime,
		AssetExec:   u.Current.AssetExec,
		AssetSymbol: u.Current.AssetSymbol,
		TotalCount:  u.Current.TotalCount,
		Means:       u.Current.Means,
	}
	unfreezeTx.Withdraw = &ActionWithdraw{
		Amount: u.Prev.Remaining - u.Current.Remaining,
	}
	return unfreezeTx
}

func setUnfreezeTerminate(unfreezeTx *Tx, u *pty.ReceiptUnfreeze) *Tx {
	unfreezeTx.ActionType = ActionTypeTerminate
	unfreezeTx.Creator = u.Current.Initiator
	unfreezeTx.Beneficiary = u.Current.Beneficiary
	unfreezeTx.UnfreezeID = u.Current.UnfreezeID
	unfreezeTx.Success = true

	unfreezeTx.Create = &ActionCreate{
		StartTime:   u.Current.StartTime,
		AssetExec:   u.Current.AssetExec,
		AssetSymbol: u.Current.AssetSymbol,
		TotalCount:  u.Current.TotalCount,
		Means:       u.Current.Means,
	}
	unfreezeTx.Terminate = &ActionTerminate{
		AmountBack: u.Prev.Remaining - u.Current.Remaining,
		AmountLeft: u.Current.Remaining,
	}
	return unfreezeTx
}
