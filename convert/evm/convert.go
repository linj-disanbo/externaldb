package evm

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/33cn/chain33/common"
	l "github.com/33cn/chain33/common/log/log15"
	coinsTypes "github.com/33cn/chain33/system/dapp/coins/types"
	"github.com/33cn/chain33/types"
	ndb "github.com/33cn/externaldb/convert/evm/nft/db"
	"github.com/33cn/externaldb/lib/chaincfg"
	"github.com/33cn/externaldb/lib/esdoc/contract"
	"github.com/33cn/externaldb/lib/esdoc/contractverify"
	"github.com/33cn/externaldb/lib/schema"
	"github.com/33cn/go-kit/convert"
	pabi "github.com/33cn/plugin/plugin/dapp/evm/executor/abi"
	pcom "github.com/33cn/plugin/plugin/dapp/evm/executor/vm/common"
	evmtypes "github.com/33cn/plugin/plugin/dapp/evm/types"
	etypes "github.com/ethereum/go-ethereum/core/types"

	converter "github.com/33cn/externaldb/convert"
	dbcom "github.com/33cn/externaldb/convert/common"
	proofconfig "github.com/33cn/externaldb/convert/proof_config"
	"github.com/33cn/externaldb/lib/esdoc/transaction"
	esmaint "github.com/33cn/externaldb/lib/esmaint"
	"github.com/33cn/externaldb/lib/proto"
)

var log = l.New("module", "db.evm")

// Convert tx convert
type Convert struct {
	symbol string
	title  string

	env     *converter.TxEnv
	tx      *types.Transaction
	receipt *types.ReceiptData

	ConfigDB      proofconfig.ConfigDB
	ContractDB    contract.DB
	ContractVrfDB contractverify.DB
	NftTokenDb    ndb.TokenDB
	EvmTokenDb    TokenDB
}

func init() {
	converter.RegisterConvert("evm", NewConvert)
}

// NewConvert new convert
func NewConvert(paraTitle, symbol string, _ []string) converter.ExecConvert {
	e := &Convert{symbol: symbol, title: paraTitle}
	return e
}

// InitDB init db
func (c *Convert) InitDB(cli schema.DBCreator) error {
	var err error
	err = contract.InitESDB(cli)
	if err != nil {
		return err
	}
	err = contractverify.InitESDB(cli)
	if err != nil {
		return err
	}
	err = esmaint.InitIndex(cli, EVMX, EVMX, EVMMapping)
	if err != nil {
		return err
	}
	err = esmaint.InitIndex(cli, EVMTokenX, EVMTokenX, TokenMapping)
	if err != nil {
		return err
	}
	err = esmaint.InitIndex(cli, EVMTransferX, EVMTransferX, EVMTransferMapping)
	if err != nil {
		return err
	}
	for _, initDB := range initDBs {
		if err := initDB(cli); err != nil {
			return err
		}
	}
	return nil
}

func (c *Convert) positionID() string {
	return fmt.Sprintf("%s:%d.%d", "evm.token", c.env.Block.Block.Height, c.env.TxIndex)
}

// SetDB 访问权限控制index
func (c *Convert) SetDB(db converter.WrapDB) error {
	c.ConfigDB = proofconfig.NewConfigDB(db)
	c.ContractDB = contract.NewEsDB(db)
	c.ContractVrfDB = contractverify.NewEsDB(db)
	c.NftTokenDb = ndb.NewTokenDB(db)
	c.EvmTokenDb = NewTokenDB(db)
	return nil
}

// ConvertTx impl
func (c *Convert) ConvertTx(env *converter.TxEnv, op int) ([]schema.Record, error) {
	var err error
	c.env = env
	tx := env.Block.Block.Txs[env.TxIndex]
	c.tx = tx
	c.receipt = env.Block.Receipts[env.TxIndex]
	height := env.Block.Block.Height

	var records []schema.Record
	var payload proto.EVMContractAction
	trans := converter.ConvertTransaction(env)
	txOption := TxOption{}
	trans.Options = &txOption
	records = append(records, &transaction.TxRecord{
		IKey: transaction.NewTransactionKey(trans.Hash),
		Op:   schema.NewOp(op),
		Tx:   trans,
	})
	log := log.New("tx_hash", trans.Hash)

	err = types.Decode(tx.Payload, &payload)
	if err != nil || c.receipt.Ty != types.ExecOk {
		//解析payload出错，打印错误日志。跳过解析交易详情并把交易的基本信息存到数据库
		if c.receipt.Ty == types.ExecOk {
			//交易执行成功但解析payload出错，说明代码存在bug
			log.Error(c.positionID(), "info", "decode payload error and skip convert tx", "error", err)
		}
		log.Error(c.positionID(), "info", "decode payload error convert tx", "error", err)
		return records, nil
	}
	trans.Amount = int64(payload.Amount)
	txOption.ContractAddr = payload.ContractAddr

	fromAddr := chaincfg.AddressConvert(tx.From())
	// 签名地址是否有权限,没有直接返回
	isProof := false
	if isProof {
		if !c.ConfigDB.IsHaveProofPermission(fromAddr) {
			log.Error("ConvertTx.evm:IsHaveProofPermission", "err", errors.New("ErrNoPermission"))
			return records, nil
		}

		_, err = c.ConfigDB.GetOrganizationName(fromAddr)
		if err != nil {
			log.Error("ConvertTx.evm:GetOrganizationName", "err", err, "addr", fromAddr)
			return records, nil
		}
	}

	mapinfo := make(map[string]interface{})
	mapinfo, err = parseNote(mapinfo, payload.Note, payload.ContractAddr, types.IsEthSignID(tx.GetSignature().GetTy()))
	if err != nil {
		log.Debug("parseNote", "err", err)
	}
	eventLogs := c.parseLog(mapinfo)
	mapinfo["to_addr"] = chaincfg.AddressConvert(tx.To)
	mapinfo["from_addr"] = fromAddr
	mapinfo["evm_tx_hash"] = common.ToHex(tx.Hash())
	mapinfo["evm_height_index"] = converter.HeightIndex(height, env.TxIndex)
	mapinfo["evm_height"] = height
	mapinfo["evm_block_hash"] = env.BlockHash
	mapinfo["evm_block_time"] = env.Block.Block.BlockTime

	// 合约处理
	log = log.New("contract_addr", payload.ContractAddr)
	// 合约部署 / 纯转账：两者在 chain33 里 ContractAddr 都是 evm 执行器地址，
	// 区别是 payload.Code 是否为空（chain33 executor: isCreate = to==execAddr && len(data)>0，
	// isTransferOnly = to==execAddr && len(data)==0；eth 的纯币转账 data 为空）。
	execAddr := chaincfg.ExecAddress(string(tx.Execer))
	if execAddr == payload.ContractAddr && len(payload.Code) == 0 {
		// 纯 coins 转账（eth 发币交易）：没有合约文档可写，只补 options
		txOption.CallFunName = "coins.transfer"
		mapinfo["evm_transfer_only"] = true
		log.Debug("ConvertTx.evm: transfer only", "to", tx.To, "amount", payload.Amount)
		return records, nil
	}
	// 合约部署
	if execAddr == payload.ContractAddr {
		mapinfo["contract_bin"] = hex.EncodeToString(payload.Code)
		ct := contract.NewContractByMap(mapinfo)

		vrf, err := c.ContractVrfDB.Get(ct.GetContractBinHash())
		switch err {
		case nil:
			ct.ContractAbi = vrf.ContractAbi
			ct.ContractType = vrf.ContractType
		case converter.ErrDBNotFound:
			if ct.ContractVerify.ContractAbi != "" {
				records = append(records, contractverify.NewRecord(op, ct.ContractVerify))
			}
		default:
			log.Error("ContractVrfDB.Get", "err", err, "bin-hash", ct.GetContractBinHash())
		}
		if ct.ContractType == "" {
			contract.Detector.Detect(ct)
		}
		txOption.ContractAddr = ct.Address
		txOption.ContractType = ct.ContractType
		txOption.CallFunName = "DeployContract"
		ct.TxCount++
		c.ContractDB.UpdateCache(ct)
		records = append(records, contract.NewRecord(op, ct))
		return records, nil
	}
	//records = append(records, &RecordEVM{
	//	IKey:  NewEVMKey(EVM(mapinfo).Key()),
	//	Op:    schema.NewOp(op),
	//	value: mapinfo,
	//})

	// 代理执行（EVM -> Go 执行器）：contractAddr 是 chain33 的 proxyExecAddress（如 0x…200005），
	// 链上没有对应合约文档，payload.Para 是内层 chain33 交易（见 chain33 executor/execenv.go proxyGetRealTx）。
	// 这里不能查 contract 索引，否则每笔都会 ErrDBNotFound 并丢掉后续解析。
	// chain33 代理执行的前提是 eth 签名（execenv.checkProxyExecTx: tx.To==proxy && IsEthSignID），
	// 非 eth 签名即使 to 是代理地址也只是一次普通 EVM 调用，不能按内层交易解析
	if chaincfg.IsEvmCallGoAddr(payload.ContractAddr) && types.IsEthSignID(tx.GetSignature().GetTy()) {
		c.convertEvmCallGo(records, trans, &txOption, mapinfo, &payload, op)
		return records, nil
	}

	// 合约执行
	ct, err := c.ContractDB.Get(payload.ContractAddr)
	if err != nil {
		// 合约没录 ABI（或根本不是合约）：没有可解析的 ABI/事件，保留交易记录即可，不算错误
		if errors.Is(err, converter.ErrDBNotFound) {
			log.Debug("ConvertTx.evm: contract not found", "addr", payload.ContractAddr)
		} else {
			log.Error("ConvertTx.evm:ContractDB.Get", "err", err)
		}
		return records, nil
	}
	mapinfo["contract_type"] = ct.ContractType
	if ct.ContractAbi == "" {
		if ct.ContractType != "" {
			ct.ParsedAbi = contract.DefaultABIs[ct.ContractType]
		} else {
			return records, nil
		}
	}

	if _, err := c.parseParam(ct.ParsedAbi, payload.Para, mapinfo); err != nil {
		log.Warn("ConvertTx.evm:parseParam", "err", err)
	}
	// txOption.Params 不写入 options.params，避免大数值溢出

	events := c.parseEvent(ct.ParsedAbi, mapinfo, eventLogs)
	// 不写入 txOption.Events/Transfers/Params，避免 uint256 数值超过 ES long 范围
	// 事件/Transfer 详情通过 event handle（如 erc20.Transfer）写入独立 evm_transfer 索引

	// 指定合约调用处理
	callFuncName := convert.ToString(mapinfo["call_func_name"])
	if callFuncName == "" {
		callFuncName = hex.EncodeToString(payload.Para[:4])
	}
	if fcv, ok := GetFuncConvert(callFuncName); ok {
		rd, err := fcv(c, op, mapinfo)
		if err == nil {
			records = append(records, rd...)
		} else {
			log.Error("evm func convert", "err", err, "func_name", callFuncName)
		}
	} else {
		log.Warn("no evm func convert", "func_name", callFuncName)
	}
	// 指定event处理
	for key := range events {
		if handle, ok := GetEventHandle(ct.ContractType + key); ok {
			rd, err := handle(c, op, mapinfo)
			if err == nil {
				records = append(records, rd...)
			} else {
				log.Error("evm event handle", "err", err, "event_name", key)
			}
		}
	}
	// 数据收集
	transfers := make([]*Transfer, 0)
	tokenID := make([]string, 0)
	contractType := ct.ContractType
	mintCount := int64(0)
	for _, record := range records {
		if record.Index() == EVMTransferX {
			if r, ok := record.(schema.SourceAbleRecord); ok {
				if t, ok := r.Source().(*Transfer); ok {
					transfers = append(transfers, t)
					tokenID = append(tokenID, t.TokenID)
					contractType = t.TokenType
					if t.IsMint() {
						mintCount += t.Amount
					}
				}
			}
		}
	}

	txOption.ContractType = contractType
	// txOption.TokenID / txOption.Transfers 不写入 options，避免 Transfer.Amount int64 溢出
	txOption.CallFunName = callFuncName

	// 合约数量统计
	if op == schema.OpAdd {
		ct.PublishCount += mintCount
		ct.TxCount++
	} else {
		ct.PublishCount -= mintCount
		ct.TxCount--
	}
	records = append(records, contract.NewRecord(schema.OpUpdate, ct))
	return records, nil
}

func (c *Convert) parseLog(mapinfo map[string]interface{}) []*types.EVMLog {
	eLogs := make([]*types.EVMLog, 0)
	for _, lg := range c.receipt.Logs {
		switch lg.Ty {
		case evmtypes.TyLogContractData:
			var cd evmtypes.EVMContractData
			if err := types.Decode(lg.Log, &cd); err != nil {
				log.Error("parseLog: Decode EVMContractData", "err", err)
				continue
			}
			mapinfo["contract_addr"] = chaincfg.AddressConvert(cd.Addr)
			mapinfo["contract_name"] = cd.Name
			mapinfo["contract_creator"] = cd.Creator
		case evmtypes.TyLogCallContract:
			var ec evmtypes.ReceiptEVMContract
			if err := types.Decode(lg.Log, &ec); err != nil {
				log.Error("parseLog: Decode ReceiptEVMContract", "err", err)
				continue
			}
			mapinfo["contract_addr"] = chaincfg.AddressConvert(ec.ContractAddr)
			mapinfo["contract_used_gas"] = ec.UsedGas
		case evmtypes.TyLogEVMEventData:
			var el types.EVMLog
			if err := types.Decode(lg.Log, &el); err != nil {
				log.Error("parseLog: Decode EVMLog", "err", err)
				continue
			}
			if len(el.Topic) <= 0 {
				log.Error("parseLog: no topic", "len(topic)", len(el.Topic))
				continue
			}
			eLogs = append(eLogs, &el)
		}
	}
	return eLogs
}

func (c *Convert) parseEvent(abi *pabi.ABI, mapinfo map[string]interface{}, eLogs []*types.EVMLog) (events map[string]interface{}) {
	// parse event
	if len(eLogs) <= 0 {
		return
	}
	events = make(map[string]interface{})
	errEvents := make([]interface{}, 0)
	for _, el := range eLogs {
		var topics []pcom.Hash
		for _, topic := range el.Topic {
			topics = append(topics, pcom.BytesToHash(topic))
		}
		name, m, err := UnpackEvent(el.Data, topics, abi)
		if err != nil {
			log.Error("parseLog: UnpackEventLog", "err", err)
			m = make(map[string]interface{})
			m["data"] = el.Data
			m["topic"] = el.Topic
			m["err"] = err.Error()
			errEvents = append(errEvents, m)
			continue
		}
		events[name] = m
	}
	if len(events) > 0 {
		buf, err := json.Marshal(events)
		if err != nil {
			log.Error("parseLog json.Marshal(events)", "err", err)
			return
		}
		mapinfo["evm_events"] = string(buf)
	}
	if len(errEvents) > 0 {
		buf, err := json.Marshal(errEvents)
		if err != nil {
			log.Error("parseLog json.Marshal(errEvents)", "err", err)
			return
		}
		mapinfo["evm_err_parse_events"] = string(buf)
	}
	return
}

func (c *Convert) parseParam(abi *pabi.ABI, data []byte, m map[string]interface{}) (map[string]interface{}, error) {
	if m == nil {
		return nil, fmt.Errorf("parseParam: map is nil")
	}
	pm, err := UnpackParam(data, abi)
	if err != nil {
		log.Error("parseParam: UnpackParam", "err", err)
		return pm, err
	}
	buf, err := json.Marshal(pm)
	if err != nil {
		log.Error("parseParam: json.Marshal(pm)", "err", err)
		return pm, err
	}
	if m["call_func_name"] == nil || m["call_func_name"] == "" {
		m["call_func_name"] = pm["call_func_name"]
	}
	m["evm_param"] = string(buf)
	return pm, nil
}

// convertEvmCallGo 处理代理执行交易（EVM 合约经 proxyExecAddress 调用 Go 执行器）。
//
// chain33 规范（execenv.go proxyGetRealTx）：payload.Para 是内层 chain33 交易的 protobuf，
// 内层交易才是真正的业务（本例：coins 的 withdraw，85 BTY）。note 属于外层 eth 交易，
// 不参与代理执行，也不参与这里的解析。
func (c *Convert) convertEvmCallGo(records []schema.Record, trans *transaction.Transaction,
	txOption *TxOption, mapinfo map[string]interface{}, payload *proto.EVMContractAction, op int) {

	log := log.New("tx_hash", trans.Hash)

	var inner types.Transaction
	if len(payload.Para) == 0 {
		log.Debug("convertEvmCallGo: empty para")
		return
	}
	if err := types.Decode(payload.Para, &inner); err != nil {
		log.Debug("convertEvmCallGo: decode inner tx", "err", err)
		return
	}

	execName := string(types.GetRealExecName(inner.Execer))
	actionName := innerActionName(execName, &inner)
	txOption.CallFunName = execName + "." + actionName
	mapinfo["evm_call_go"] = true
	mapinfo["evm_inner_exec"] = execName
	if inner.To != "" {
		mapinfo["to_addr"] = chaincfg.AddressConvert(inner.To)
	}
	if amount, ok := innerActionAmount(execName, &inner); ok && amount != 0 {
		// trans 已被放进 transaction 记录（指针），这里改的是同一份数据
		trans.Amount = amount
	}
	log.Debug("convertEvmCallGo", "exec", execName, "action", actionName)
	_ = op
}

// innerActionName 取内层交易的 action 名。老执行器的 Transaction.ActionName() 可能返回 "unknown"，
// 所以 coins 按 action 字段判断（比按 Ty 更稳，兼容历史交易）。
func innerActionName(execName string, tx *types.Transaction) string {
	if execName == "coins" {
		var action coinsTypes.CoinsAction
		if err := types.Decode(tx.Payload, &action); err == nil {
			switch {
			case action.GetTransfer() != nil:
				return "transfer"
			case action.GetWithdraw() != nil:
				return "withdraw"
			case action.GetTransferToExec() != nil:
				return "transferToExec"
			case action.GetGenesis() != nil:
				return "genesis"
			}
		}
	}
	if name := tx.ActionName(); name != "" && name != "unknown" {
		return name
	}
	return "call"
}

// innerActionAmount 取内层交易的金额（coins 的金额在 action 里，不在 tx.Amount）。
func innerActionAmount(execName string, tx *types.Transaction) (int64, bool) {
	if execName != "coins" {
		amount, err := tx.Amount()
		return amount, err == nil
	}
	var action coinsTypes.CoinsAction
	if err := types.Decode(tx.Payload, &action); err != nil {
		return 0, false
	}
	switch {
	case action.GetTransfer() != nil:
		return action.GetTransfer().GetAmount(), true
	case action.GetWithdraw() != nil:
		return action.GetWithdraw().GetAmount(), true
	case action.GetTransferToExec() != nil:
		return action.GetTransferToExec().GetAmount(), true
	case action.GetGenesis() != nil:
		return action.GetGenesis().GetAmount(), true
	}
	return 0, false
}

// parseNote 解析 payload.Note。note 的语义取决于签名类型（chain33 eth 兼容规范）：
//
//   - eth 签名交易（IsEthSignID）：note 必须是 hex(RLP raw tx)，是节点验签与 eth 交易哈希的依据
//     （chain33 system/crypto/secp256k1eth.VerifyBytes / types.Transaction.GetEthTxHash）。
//     这里只提取展示用信息，不做 base64/JSON 尝试。
//   - 非 eth 签名：note 是自由备注，本项目约定为 base64(JSON)，可带 abi 或 call_func_name。
//
// 两种情况解析失败都只记为 Debug，不影响交易记录落库。
func parseNote(mapinfo map[string]interface{}, note, addr string, isEthSign bool) (map[string]interface{}, error) {
	if note == "" {
		return mapinfo, nil
	}

	if isEthSign {
		raw, err := common.FromHex(note)
		if err != nil {
			log.Debug("parseNote: eth note is not hex", "err", err)
			return mapinfo, nil
		}
		var etx etypes.Transaction
		if err := etx.UnmarshalBinary(raw); err != nil {
			log.Debug("parseNote: eth note is not rlp tx", "err", err)
			return mapinfo, nil
		}
		mapinfo["evm_note_type"] = "rlp"
		mapinfo["evm_eth_tx_hash"] = common.ToHex(etx.Hash().Bytes())
		mapinfo["evm_nonce"] = etx.Nonce()
		mapinfo["evm_gas_price"] = etx.GasPrice().String()
		mapinfo["evm_gas"] = etx.Gas()
		return mapinfo, nil
	}

	mapNote := make(map[string]interface{})
	nb, err := dbcom.DecodeString(note)
	if err != nil {
		log.Debug("parseNote:DecodeString payload.Note", "err", err)
		return mapinfo, nil
	}
	mapinfo["evm_note"] = nb

	if err := json.Unmarshal(nb, &mapNote); err != nil {
		log.Debug("Unmarshal note failed", "err", err)
		return mapinfo, nil
	}
	if abi, ok := mapNote["abi"]; ok {
		mapinfo["contract_abi"] = abi
	} else {
		mapinfo["call_func_name"] = mapNote["call_func_name"]
		mapinfo["contract_addr"] = chaincfg.AddressConvert(addr)
	}

	return mapinfo, nil
}
