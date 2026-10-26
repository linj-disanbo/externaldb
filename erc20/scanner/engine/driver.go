package engine

// 注册 chain33 内置地址驱动（btc id=0/1、eth id=2、btc-utxo id=3）。
//
// chain33 的 types.Transaction.From() 按签名 ty 里的 addressID 调
// address.MustLoadDriver(id)，驱动未注册时直接 panic（"unknown address driver, id=2"）。
// 本仓库只有 lib/chaincfg（经 _ "github.com/33cn/chain33/system"）注册了 eth 驱动，
// erc20 扫块程序不经过 chaincfg，所以在 engine 包里显式引入，保证
// buildSlotEthExpectsFromBlockDetail 对 EVM 兼容链的 eth 签名交易取 From 时不 panic。
import (
	_ "github.com/33cn/chain33/system/address"
)
