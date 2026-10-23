# txinfo：按交易哈希看 scanner 眼里的一条交易

（原 `erc20Scaner/cmd/txinfo/1.md`，2026-10 随目录整理搬迁，命令已更新到新布局。）

## 命令

```bash
# 仓库根目录
go build -o build/erc20-txinfo ./cmd/tools/erc20-txinfo
./build/erc20-txinfo -rpc 'YOUR_JSON_RPC_URL' 0x...
./build/erc20-txinfo -rpc 'YOUR_JSON_RPC_URL' -json 0x...   # 整份 JSON
```

## 输出内容（相对 scanner「处理到的信息」）

```
类别	内容
基础	区块号/哈希、区块时间（含 Asia/Shanghai）、交易在块内下标
交易	from（EIP155 恢复）、to、nonce、type、chainId、gas、gas price / EIP-1559、value、完整 data、前 4 字节 selector
回执	status、gasUsed、累计 gas、transactionIndex、部署合约地址、全部 logs（index、address、topics、data、是否与 ERC20 Transfer 形状一致）
合约创建	checkERC20BySelector 结果（核心/可选/扩展 selector 计数）、若通过则 name/symbol/decimals/totalSupply，以及 与 saveContractToDB / saveNonERC20ContractToDB 一致的 contracts 表预览
ERC20 转账路径	calldata selector、是否直接 transfer/transferFrom、每个 distinct token 合约的 direct/nes
```

## 样本交易（卖出带卖税的）

```
0x0919c7fe5d18fc014a22de0ff8205b79ab0f62a21ec08da56129814d17757f8f
 -> 0x112129e8c637835b3d1e80f68c2905733df7deb8fa02fae96533f245684ea708

0xacd8af1ab12133b5989639dd593e67b27d36ba6b1e5544730e2b8bdd3865b924
 -> 0x59d2d254e7fb23b4f8414b29184c73bad0b63af8eb7bb5ddb3220bc84f24488b
0x1ad5b5947fa614f018763f88d6442eb71c67da2e4d606a4ca212c4820ce480c6
0x11e0e96ed14a483ab81cc4a69611db0447b9f4b4d42a4c0dfbbf787778ce2dc8

买入： 0x96f6d6fc9a68dbaa13728967c768db0040ed76901e2bdad208eabb282b99036f
 -> 0x96f6d6fc9a68dbaa13728967c768db0040ed76901e2bdad208eabb282b99036f
转账： 0xb38cd0778323db041fe54dcb2bf8624357f234422c3f7dafe4dec6d00fa8dd50
 -> 0x9f402cb5ed45642d750513e850d1320a62845716384dc4f934e8565fb4f5e381
卖出： 0xc3baac605c5a2644b1a6cf3a55c9e266b01c41bfab249e4e799fff37a377f769
 -> 0xc3baac605c5a2644b1a6cf3a55c9e266b01c41bfab249e4e799fff37a377f769
```

## 相关查询接口

`GET /evmapi/accounts/{address}/erc20-balances?page=1&size=20&min_balance=`
