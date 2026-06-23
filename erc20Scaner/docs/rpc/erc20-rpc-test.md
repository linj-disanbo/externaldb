# erc20相关接口功能测试

正式: https://mainnet.bityuan.com/scanner/

测试服务器为: http://localhost:8080/


- 1. ok-List ERC20 Contracts
- 2. ok-List ERC20 Tokens
- 3. ok-Get info of the ERC20-Token/Contract (现在返回结果是一样的)
- 4. ok-List Transfers of the ERC20-Token - 发布erc20的合约地址代表erc20, 因为 symbol 会重复
- 5. ok-List transactions of the ERC20-Contract
- 6. more info for evm Tx -- 代替调用 Evm.ParseTx. (本来是要升级原来的接口的, 但原来的工程依赖老的chain33, 依赖go1.18, 现在对以太坊的解析依赖比较新的库, 一直冲突, 弄了好几天, 没有解决, 暂时只能麻烦你调用新的接口, 老的接口不变, 新接口增加了数据Events)
- 7. ok-按用户地址查 ERC20 持仓列表 `GET /evmapi/accounts/{address}/erc20-balances?page=1&size=20`（可选 `min_balance`）



```
"Events" : [
         {
            "Args" : {
               "amount" : "268000000000000000000",
               "contract_address" : "0xf466035fe7b7c045456834a923c9611465467558",
               "from" : "0x244a61b9c44f1b116e444a298b939df0b5482c90",
               "to" : "0x2c300d93d6f40d44f786968929c55acb2d1d47b7",
               "token_decimals" : 18,
               "token_symbol" : "HPO",
               "value" : "268000000000000000000"
            },
            "Name" : "Transfer"
         }
      ],

```

## 1 查询合约列表

**GET** `/evmapi/contracts?page=1&size=20`

curl -X GET http://localhost:8080/evmapi/contracts?page=1&size=20


```
{
   "code" : 0,
   "data" : {
      "contracts" : [
         {
            "address" : "0xa72b4d8a070a04131ef31f0f0e963c7599d8fb25",
            "decimals" : 18,
            "deploy_time" : "2025-11-25T14:59:13+08:00",
            "name" : "HashFun Token",
            "symbol" : "HFO",
            "total_supply" : "1000000000000000000000000000",
            "total_supply_formatted" : "1000000000",
            "type" : "ERC20"
         },
         {
            "address" : "0xf466035fe7b7c045456834a923c9611465467558",
            "decimals" : 18,
            "deploy_time" : "2025-11-25T14:59:05+08:00",
            "name" : "HashPool Token",
            "symbol" : "HPO",
            "total_supply" : "1000000000000000000000000000",
            "total_supply_formatted" : "1000000000",
            "type" : "ERC20"
         }
      ],
      "page" : 1,
      "size" : 20,
      "total" : 2
   },
   "message" : "Success"
}



```

## 2 查询ERC20 Token列表

**GET** `/evmapi/tokens?page=1&size=20`

查询ERC20代币列表，专门用于查询ERC20类型的代币 

curl -X GET http://localhost:8080/evmapi/tokens?page=1&size=20
 

```
{
   "code" : 0,
   "data" : {
      "page" : 1,
      "size" : 20,
      "tokens" : [
         {
            "address" : "0xa72b4d8a070a04131ef31f0f0e963c7599d8fb25",
            "decimals" : 18,
            "deploy_time" : "2025-11-25T14:59:13+08:00",
            "name" : "HashFun Token",
            "symbol" : "HFO",
            "total_supply" : "1000000000000000000000000000",
            "total_supply_formatted" : "1000000000",
            "type" : "ERC20"
         },
         {
            "address" : "0xf466035fe7b7c045456834a923c9611465467558",
            "decimals" : 18,
            "deploy_time" : "2025-11-25T14:59:05+08:00",
            "name" : "HashPool Token",
            "symbol" : "HPO",
            "total_supply" : "1000000000000000000000000000",
            "total_supply_formatted" : "1000000000",
            "type" : "ERC20"
         }
      ],
      "total" : 2
   },
   "message" : "Success"
}
```

 

## 3 查询ERC20代币详情

**GET** `/evmapi/tokens/{address}`

查询指定ERC20代币的详细信息。此接口专门用于查询ERC20类型的代币，如果地址不是ERC20代币，将返回错误。

curl -X GET http://localhost:8080/evmapi/tokens/0xf466035fe7b7c045456834a923c9611465467558

```
{
   "code" : 0,
   "data" : {
      "address" : "0xf466035fe7b7c045456834a923c9611465467558",
      "decimals" : 18,
      "deploy_block_number" : 0,
      "deploy_time" : "2025-11-25T14:59:05+08:00",
      "deploy_tx_hash" : "",
      "deployer" : "",
      "name" : "HashPool Token",
      "symbol" : "HPO",
      "total_supply" : "1000000000000000000000000000",
      "total_supply_formatted" : "1000000000",
      "type" : "ERC20",
      "verification_status" : 1
   },
   "message" : "Success"
}
```

**与查询合约详情的区别：**
- `/evmapi/contracts/{address}` - 查询任何类型的合约（ERC20, ERC721等）, 现在只支持 erc20
- `/evmapi/tokens/{address}` - 只查询ERC20类型的代币，如果不是ERC20会返回错误


## 4 查询erc20的Transfer记录 可指定地址



**GET** `/evmapi/tokens/{address}/transfers?page=1&size=20&from=0x...&to=0x...`

curl -X GET http://localhost:8080/evmapi/tokens/0xf466035fe7b7c045456834a923c9611465467558/transfers?to=0x8b8a3fa686354788522802baf34bc9d2a5a66699

```
 {
   "code" : 0,
   "data" : {
      "page" : 1,
      "size" : 20,
      "total" : 2,
      "transfers" : [
         {
            "block_number" : 42331369,
            "block_time" : "2025-11-25T15:04:21+08:00",
            "from" : "0x244a61b9c44f1b116e444a298b939df0b5482c90",
            "to" : "0x8b8a3fa686354788522802baf34bc9d2a5a66699",
            "token_decimals" : 18,
            "token_symbol" : "HPO",
            "tx_hash" : "0x619a44ce0f69fbe9c194ace2b279b4abb02e0c8a486cf214bb7793692d28aaa7",
            "value" : "55000000000000000000",
            "value_formatted" : "55"
         },
         {
            "block_number" : 42331349,
            "block_time" : "2025-11-25T15:02:10+08:00",
            "from" : "0x244a61b9c44f1b116e444a298b939df0b5482c90",
            "to" : "0x8b8a3fa686354788522802baf34bc9d2a5a66699",
            "token_decimals" : 18,
            "token_symbol" : "HPO",
            "tx_hash" : "0x2a621fb0293cd7dd14addef4d978bcdd3eb682575a77505065046d78af034480",
            "value" : "55000000000000000000",
            "value_formatted" : "55"
         }
      ]
   },
   "message" : "Success"
}


```

## 5 查询指定合约的交易记录

**GET** `/evmapi/contracts/{address}/transactions?page=1&size=20`

查询指定合约的交易记录。

curl -X GET http://localhost:8080/evmapi/contracts/0xf466035fe7b7c045456834a923c9611465467558/transactions?size=2 

```
 {
   "code" : 0,
   "data" : {
      "page" : 1,
      "size" : 2,
      "total" : 8,
      "transactions" : [
         {
            "block_number" : 42331382,
            "block_time" : "2025-11-25T15:06:36+08:00",
            "from" : "0x244a61b9c44f1b116e444a298b939df0b5482c90",
            "func_name" : "transfer",
            "gas_used" : 35460,
            "status" : 1,
            "to" : "0xf466035fe7b7c045456834a923c9611465467558",
            "token_decimals" : 18,
            "token_symbol" : "HPO",
            "tx_hash" : "0x9945a009994ecfae977b888f4b957d907ac1e3ec755a02bb1773a2e518fc1329",
            "value" : "0",
            "value_formatted" : "0"
         },
         {
            "block_number" : 42331377,
            "block_time" : "2025-11-25T15:05:26+08:00",
            "from" : "0x244a61b9c44f1b116e444a298b939df0b5482c90",
            "func_name" : "transfer",
            "gas_used" : 35544,
            "status" : 1,
            "to" : "0xf466035fe7b7c045456834a923c9611465467558",
            "token_decimals" : 18,
            "token_symbol" : "HPO",
            "tx_hash" : "0x1d6b6611e21fa3552a79443582ed4a2e7b57d30e9184d030d8a8bbec1e123368",
            "value" : "268000000000000000000",
            "value_formatted" : "268"
         }
      ]
   },
   "message" : "Success"
}


```
## 6 交易解析

- `GET /evmapi/transactions?contract=0x..&from=0x..&start_block=N&end_block=N&page=1&page_size=20` → 通用交易列表查询
- `GET /evmapi/transactions/{tx_hash}/analysis` → 解析并获取单笔 EVM 交易的详细动作

# 通用交易列表查询
curl -X GET "http://localhost:8080/evmapi/transactions?contract=0xabcdef0123456789abcdef0123456789abcdef01&page=1&page_size=20"

# 按 from 地址查询
curl -X GET "http://localhost:8080/evmapi/transactions?from=0x1234567890abcdef1234567890abcdef12345678&start_block=10000000&page=1&page_size=20"

curl -X GET http://localhost:8080/evmapi/transactions/0x1d6b6611e21fa3552a79443582ed4a2e7b57d30e9184d030d8a8bbec1e123368/analysis

```
{
   "code" : 0,
   "data" : {
      "Amount" : 0,
      "Asset" : {
         "amount" : 0,
         "exec" : "evm",
         "symbol" : ""
      },
      "CallAddress" : "0x244a61b9c44f1b116e444a298b939df0b5482c90",
      "Chain33TxId" : "0x6b7ac3f000364d7dde69896c2f7fa3875b49dbc17b0d591ac51a81f5cb596e1d",
      "ContractAddress" : "0xf466035fe7b7c045456834a923c9611465467558",
      "Error" : "",
      "Events" : [
         {
            "Args" : {
               "amount" : "268000000000000000000",
               "contract_address" : "0xf466035fe7b7c045456834a923c9611465467558",
               "from" : "0x244a61b9c44f1b116e444a298b939df0b5482c90",
               "to" : "0x2c300d93d6f40d44f786968929c55acb2d1d47b7",
               "token_decimals" : 18,
               "token_symbol" : "HPO",
               "value" : "268000000000000000000"
            },
            "Name" : "Transfer"
         }
      ],
      "EvmTxId" : "0x1d6b6611e21fa3552a79443582ed4a2e7b57d30e9184d030d8a8bbec1e123368",
      "ExecSuccess" : true,
      "Func" : {
         "Args" : "args",
         "FuncName" : "0xa9059cbb",
         "MethodID" : "0xa9059cbb"
      },
      "GasLimit" : 400000,
      "GasUsed" : 0,
      "IsCreateContract" : false,
      "IsEvmTx" : true,
      "ParseSuccess" : true
   },
   "message" : "Success"
}

```

## 7. 按用户地址查 ERC20 持仓列表

**GET** `/evmapi/accounts/{address}/erc20-balances?page=1&size=20&min_balance=`

```
curl -s "http://localhost:8080/evmapi/accounts/0xe2c9b42e96e78f5de84406f2c1f4f2bc315e98ad/erc20-balances?page=1&size=20&min_balance="


0xe2c9b42e96e78f5de84406f2c1f4f2bc315e98ad 随便找的地址
``` 





```
{
   "code" : 0,
   "data" : {
      "address" : "0xe2c9b42e96e78f5de84406f2c1f4f2bc315e98ad",
      "page" : 1,
      "size" : 20,
      "tokens" : [
         {
            "balance" : "65415151614586977912853",
            "balance_formatted" : "65415.151614586977912853",
            "contract_address" : "0xb669041cbc498a94ea76419f06e905cc22937f19",
            "decimals" : 18,
            "last_tx_block" : 44840535,
            "last_tx_hash" : "0xdea1e3b3fff874efebe4fdfc01d990234e7179ba08ebd378c7a4ea19f9696768",
            "last_updated" : "2026-04-21T06:47:11+08:00",
            "name" : "PMM",
            "symbol" : "PMM"
         },
         {
            "balance" : "21839021809921217222784",
            "balance_formatted" : "21839.021809921217222784",
            "contract_address" : "0x5e1daf239e3e09fcce016fa4e4c70e20e01118df",
            "decimals" : 18,
            "last_tx_block" : 44840478,
            "last_tx_hash" : "0xb708c2feb29d281e209d78f88980a2c7eea3626ff4d919b4a32dc810ac4c086e",
            "last_updated" : "2026-04-21T06:43:09+08:00",
            "name" : "Bityuan LPs",
            "symbol" : "BTY-LP"
         },
         {
            "balance" : "0",
            "balance_formatted" : "0",
            "contract_address" : "0xe09f5bdca143f6e4ad9d43516a1d1289a3dd6dfc",
            "decimals" : 18,
            "last_tx_block" : 44840478,
            "last_tx_hash" : "0xb708c2feb29d281e209d78f88980a2c7eea3626ff4d919b4a32dc810ac4c086e",
            "last_updated" : "2026-04-21T06:43:09+08:00",
            "name" : "Wrapped BTY",
            "symbol" : "WBTY"
         }
      ],
      "total" : 3
   },
   "message" : "Success"
}

```

```
curl 'http://localhost:8080/evmapi/accounts/0xe2c9b42e96e78f5de84406f2c1f4f2bc315e98ad/erc20-balances?page=1&size=20&min_balance=0.01'


```

```
{
   "code" : 0,
   "data" : {
      "address" : "0xe2c9b42e96e78f5de84406f2c1f4f2bc315e98ad",
      "page" : 1,
      "size" : 20,
      "tokens" : [
         {
            "balance" : "670869697440432907481952",
            "balance_formatted" : "670869.697440432907481952",
            "contract_address" : "0xb669041cbc498a94ea76419f06e905cc22937f19",
            "decimals" : 18,
            "last_tx_block" : 44840687,
            "last_tx_hash" : "0x794fb2bb69c2867729b089d97e67ebb427f13977092b1fe6ebf53a64367062d3",
            "last_updated" : "2026-04-21T06:59:16+08:00",
            "name" : "PMM",
            "symbol" : "PMM"
         },
         {
            "balance" : "21839021809921217222784",
            "balance_formatted" : "21839.021809921217222784",
            "contract_address" : "0x5e1daf239e3e09fcce016fa4e4c70e20e01118df",
            "decimals" : 18,
            "last_tx_block" : 44840478,
            "last_tx_hash" : "0xb708c2feb29d281e209d78f88980a2c7eea3626ff4d919b4a32dc810ac4c086e",
            "last_updated" : "2026-04-21T06:43:09+08:00",
            "name" : "Bityuan LPs",
            "symbol" : "BTY-LP"
         }
      ],
      "total" : 2
   },
   "message" : "Success"
}

```
