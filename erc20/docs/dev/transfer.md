
# transfer 处理原理

```
交易流程示例：
用户 -> 合约A（DEX） -> ERC20合约 -> 发出Transfer事件
```

```
代码会：
1 遍历 receipt.Logs 中的所有日志
2 通过 log.Topics[0] 匹配 Transfer 事件签名
3 通过 log.Address 识别是哪个 ERC20 合约发出的 Transfer 事件
4 为每个不同的 ERC20 合约分别：
   验证是否为 ERC20 合约
   保存合约信息（如果不存在）
   保存交易记录
   保存所有相关的 Transfer 事件
   更新余额
5. 日志输出增强
   记录每个 Transfer 事件的实际 log index
   区分直接调用和嵌套调用
   显示涉及的 ERC20 合约数量
```

### 使用场景示例

场景 1：直接调用

```
用户直接调用 ERC20 合约的 transfer 函数
→ 交易 to = ERC20 合约地址
→ 函数选择器 = a9059cbb (transfer)
→ 保存为直接调用
```

场景 2：嵌套调用（DEX 交易）

```
用户调用 DEX 合约的 swap 函数
→ 交易 to = DEX 合约地址
→ DEX 合约内部调用 ERC20 合约的 transfer
→ 发出 Transfer 事件
→ 保存为嵌套调用（funcSelector = "nested_call"）
```

场景 3：多个 ERC20 合约

```
一个交易中涉及多个 ERC20 代币的转账
→ 识别所有不同的 ERC20 合约地址
→ 为每个合约分别保存交易记录和事件
```
