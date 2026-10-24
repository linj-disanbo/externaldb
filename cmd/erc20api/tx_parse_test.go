package main

import (
	"testing"

	"github.com/33cn/chain33/common"
	"github.com/33cn/externaldb/lib/chaincfg"
	"github.com/33cn/externaldb/lib/proto"
	"github.com/stretchr/testify/assert"
)

// 线上 tx 0xaa11cf31… 的 para：内层 coins Withdraw（85 BTY，execName=ticket）
const testProxyPara = "0a05636f696e731236180322321080ea8ed51f22067469636b65742a22313668747663424e53454137665a6841644c4a706844775152514a6148707948547020a08d0630cba5b5fcf890d6d1303a22313668747663424e53454137665a6841644c4a706844775152514a61487079485470"

// 同一笔 tx 的 note：外层 eth 交易 RLP
const testProxyNote = "f8db823e858502540be400830249f094000000000000000000000000000000000020000580b8710a05636f696e731236180322321080ea8ed51f22067469636b65742a22313668747663424e53454137665a6841644c4a706844775152514a6148707948547020a08d0630cba5b5fcf890d6d1303a22313668747663424e53454137665a6841644c4a706844775152514a61487079485470821792a07f254641e0f1d25e5f882d7e01804d6869359901ac1856af131f473357e58b10a01673098fc8648b2b06827325c18c459eecaad8a3a79834b2a2081b0d43dfdbeb"

func Test_parseEvmCallGo(t *testing.T) {
	chaincfg.InitChain33("", "bty", "")
	para, err := common.FromHex(testProxyPara)
	assert.Nil(t, err)

	info := &EvmTxInfo{ContractAddress: chaincfg.EvmCallGoAddr}
	parseEvmCallGo(info, &proto.EVMContractAction{
		Para:         para,
		Note:         testProxyNote,
		ContractAddr: chaincfg.EvmCallGoAddr,
	})

	assert.True(t, info.ParseSuccess)
	assert.Equal(t, "", info.Error)
	assert.Equal(t, "coins.withdraw", info.Func.FuncName)
	assert.Equal(t, "coins", info.Asset.Exec)
	assert.EqualValues(t, 8500000000, info.Amount)
	assert.EqualValues(t, 8500000000, info.Asset.Amount)
	assert.Equal(t, "0x92c7592ab9e0262902881d8bb19dbf8d391ecddd99f96ba78a809c8c8aa99a6e", info.EvmTxId)
}
