package erc20

import (
	"fmt"

	l "github.com/33cn/chain33/common/log/log15"
	converter "github.com/33cn/externaldb/convert"
	"github.com/33cn/externaldb/convert/evm"
	chaincfg "github.com/33cn/externaldb/lib/chaincfg"
	"github.com/33cn/externaldb/lib/schema"
	"github.com/33cn/go-kit/convert"
)

const ERC20 = "ERC20"

var (
	log = l.New("module", "db.evm.erc20")
)

func init() {
	// Approval 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925 event Approval(address indexed owner, address indexed spender, uint256 value)
	// Transfer 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef event Transfer(address indexed from, address indexed to, uint256 value)
	evm.RegisterEventHandle(ERC20+"Transfer", Transfer)
}

func Transfer(c *evm.Convert, op int, data evm.EVM) ([]schema.Record, error) {
	event, err := data.GetEvent("Transfer")
	if err != nil {
		log.Error("Transfer data.GetEvent(\"TransferBatch\")", "err", err)
		return nil, err
	}
	trans := &evm.Transfer{}
	trans.Amount = convert.ToInt64(event["value"])
	trans.From = chaincfg.AddressConvert(convert.ToString(event["from"]))
	trans.To = chaincfg.AddressConvert(convert.ToString(event["to"]))
	trans.Operator = chaincfg.AddressConvert(convert.ToString(data["from_addr"]))
	trans.TokenType = convert.ToString(ERC20)
	trans.LoadBlockData(data)

	return transferToken(c, op, trans), nil
}

func transferToken(c *evm.Convert, op int, trans *evm.Transfer) []schema.Record {
	records := make([]schema.Record, 0)

	// 转账记录
	if op == schema.SeqTypeAdd {
		records = append(records, evm.NewRecordTransfer(trans, schema.OpAdd))
	} else { // 回滚
		records = append(records, evm.NewRecordTransfer(trans, schema.OpDel))
	}

	// 减少原有token对应数量
	if trans.From != evm.EmptyAddr && trans.From != evm.EmptyAddrEth { // 等于则代表发币，不做处理
		oldT, err := c.EvmTokenDb.GetToken(trans.ContractAddr, trans.From, trans.TokenID)
		switch err {
		case nil:
			if op == schema.SeqTypeAdd {
				oldT.Amount -= trans.Amount
			} else {
				oldT.Amount += trans.Amount
			}
			records = append(records, evm.NewRecordToken(oldT, schema.OpAdd))
		case converter.ErrDBNotFound:
			log.Error("transferToken old token not found", "token", fmt.Sprintf("%s-%s-%v", trans.ContractAddr, trans.From, trans.TokenID))
		default:
			log.Error("transferToken get old token", "", err)
		}
	}

	// 新增token，或者增加对应数量
	newT, err := c.EvmTokenDb.GetToken(trans.ContractAddr, trans.To, trans.TokenID)
	switch err {
	case nil:
		if op == schema.SeqTypeAdd {
			newT.Amount += trans.Amount
		} else {
			newT.Amount -= trans.Amount
		}
		records = append(records, evm.NewRecordToken(newT, schema.OpAdd))
	case converter.ErrDBNotFound:
		token := trans.GetNewToken()
		c.EvmTokenDb.UpdateCache(token)
		records = append(records, evm.NewRecordToken(token, schema.OpAdd))
	default:
		log.Error("transferToken get newT token", "", err)
	}
	return records
}
