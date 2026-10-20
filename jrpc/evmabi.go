package main

import (
	"encoding/json"
	"fmt"

	"github.com/33cn/chain33/common"
	"github.com/33cn/externaldb/escli"

	"github.com/pkg/errors"
)

//   简单的支持解析evm交易
//  1. ABI 注册不在 jrpc 里做：写入 contractabi 索引的能力已移到仓库外的离线工具
//     （local/tools/saveabi，不随开源仓库发布）。jrpc 只读：按合约地址取 ABI。
//  2. 查询交易时, 如果是evm合约的交易会获得额外信息
//  2.1 流程
///   获得交易时 ->
///  发现是 evm合约的交易 -> 获得对应的合约地址 -> 通过合约地址地址寻找abi -> 用abi 解析 -> 解析成功后, 在交易信息中加入额外信息
///  否则获得交易信息和普通交易一致

// Evm Evm
type Evm struct {
	*DBRead
	ChainGrpc string
	Symbol    string
}

// Abi 存 ES contractabi 索引里的 ABI 文档；abi 字段是「ABI 的 JSON 文本」的 hex 编码
// （写入方：仓库外的离线工具 local/tools/saveabi）。
type Abi struct {
	Address string `json:"address"`
	Abi     string `json:"abi"`
}

type ParseEvmTxRequest struct {
	TxHash string `json:"tx_hash"`
}

func (t *Evm) ParseTx(q *ParseEvmTxRequest, out *interface{}) error {
	if q.TxHash == "" {
		err := errors.Wrapf(errors.New(errBadParm), "tx_hash empty")
		log.Error("ParseTx", "errBadParm", err.Error())
		*out = err.Error()
		return err
	}
	host := t.ChainGrpc
	detail, err := getTxDetailFromChain33(host, q.TxHash)
	if err != nil {
		err := errors.Wrapf(errors.New(errBadParm), "tx not exist")
		log.Error("ParseTx", "getTxDetailFromChain33", err.Error())
		*out = err.Error()
		return nil
	}

	parsed := parseEvmTx(detail, t.get, t.Symbol)
	*out = *parsed
	return nil
}

// get get abi
func (t *Evm) get(address string) (string, error) {
	cli, err := escli.NewESShortConnect(t.Host, t.Prefix, t.Version, t.Username, t.Password)
	if err != nil {
		log.Error("ParseTx", "NewESShortConnect", err.Error())
		return "", err
	}
	log.Error("ParseTx", "Get", ContractAbiID(address))
	raw, err := cli.Get(ContractAbiDB, ContractAbiDefaultType, ContractAbiID(address))
	if err != nil {
		log.Error("ParseTx", "Get", err.Error())
		return "", err
	}
	abi, err := decodeAbi2(raw)
	if err != nil {
		log.Error("ParseTx", "decodeAbi2", err.Error())
		return "", err
	}

	abiByte, err := common.FromHex(abi.Abi)
	if err != nil {
		log.Error("ParseTx", "FromHex", err.Error())
		return "", err
	}
	return string(abiByte), err
}

func decodeAbi(x *json.RawMessage) (interface{}, error) {
	abi := Abi{}
	err := json.Unmarshal([]byte(*x), &abi)
	return &abi, err
}

func decodeAbi2(x *json.RawMessage) (*Abi, error) {
	abi := Abi{}
	err := json.Unmarshal([]byte(*x), &abi)
	return &abi, err
}

const (
	ContractAbiDB          = "contractabi"
	ContractAbiDefaultType = "_doc"
)

func ContractAbiID(contractAddress string) string {
	return fmt.Sprintf("contract-abi-%s", contractAddress)
}

type ContractAbi struct {
	Address string `json:"address"`
	Abi     string `json:"abi"`
}
