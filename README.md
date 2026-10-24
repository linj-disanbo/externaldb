# externaldb

把 chain33 链上数据展开成**可查询的文档**，并通过 RPC / JSON-RPC 对外提供查询能力。
链上查询能力有限（合约 RPC 只覆盖预设场景），这个项目把区块、交易、回执提前解析成 ES 文档（或 MySQL 表），
让“按地址/合约/代币/文件任意组合查询”不必改链上代码、不必重同步节点。

## 两条链路

```
老链路（保留备用）: chain33 node --push--> sync --> ES(原始 seq) --> convert --> ES(解析后的文档) --> jrpc / rpc
直连链路（当前生产）: chain33 node <--gRPC 拉取(seq)-- pull_convert --> ES --> jrpc
```

- **直连链路**：`pull_convert` 按 seq 从节点拉块（`GetBlockSequences` + `GetBlockByHashes`），
  在进程内展开后直接写 ES；进度是 append-only 的 `seq_marker` 索引（游标 = max(seq)），
  不落原始区块。省掉一层 ES 存储，也省掉 sync 这一跳。
- **老链路**：`sync` 用节点的 push 接口把整块 seq 存进 ES（`seq05_seq`），`convert` 再读出来展开。
  保留用于回退与历史环境，同一批索引只能有一个写方。

## 组件

| 程序 | 作用 |
|---|---|
| `cmd/pull_convert` | 直连链路：拉块 → 展开 → 写 ES（`-mode fix -from -to` 可只修一段 seq）|
| `cmd/sync` / `cmd/convert` | 老链路：接收推送 / 读 seq 展开 |
| `cmd/jrpc` | JSON-RPC 2.0 查询端点（账户、交易、区块、EVM 合约事件等）|
| `cmd/rpc` | REST（存证 proof 相关）+ 文件下载 |
| `cmd/erc20scan` / `cmd/erc20api` | ERC20 扫块写 MySQL / `/evmapi/*` 查询（独立的 MySQL 管道）|
| `cmd/account_refresh` | 常驻循环，按三档间隔把主币真实余额校正回 ES `account` 文档 |
| `cmd/convert_check` | 离线对账（seq_marker / 文档条数）|

## 代码结构

```
cmd/        程序入口（每个二进制一个子目录）
convert/    ES 落地管道：转换器框架 + 16 个合约转换器 + pipeline(装配/pull 循环) + receiver(推送接收)
            + stat(累计统计)
erc20/      MySQL 落地管道：scanner / txparser / database / erc20api 等（内部自成一套）
lib/        两个管道共用的底层：schema(文档契约) esdoc(文档类型) escli(ES 客户端) esmaint(索引维护)
            store(seq 存储) util(块处理编排) chaincfg(chain33 初始化/地址驱动) proto version
deploy/     部署物料：pkg(老链路包) pull-convert(直连包) env(ES7 模板) docker ci
config/     配置模板（externaldb.toml / chain33.toml）
docs/       公开文档（部署、需求）
local/      私有过程文档与工具（git 忽略，不随开源仓库发布）
```

依赖方向：`lib` ← `convert`、`lib` ← `erc20`，`cmd` 在最上层；`convert` 与 `erc20` 互不引用。
新增一个合约解析：在 `convert/<name>/` 实现 `ConvertTx` + `InitDB`，`init()` 里 `convert.RegisterConvert`，
再到 `cmd/convert`、`cmd/pull_convert` 加 blank import（细节见 `convert/README.md`）。

## 快速开始

```bash
make build        # 编译全部程序（static, CGO_ENABLED=0）
make pkg          # 老链路部署包 → build/externaldb.tgz
make pkg-pull     # 直连链路部署包 → build/externaldb-pull.tgz
make testq        # 单元测试（ES/外网相关用例用 -run 过滤）
```

部署包解压后改 `.env`（链地址、ES、索引前缀、场景），然后：

```bash
make quickStart   # 起 ES → 写配置 → 初始化索引 → 起服务
# 或分开：make initES / make initConfig / make run
```

`EX_SERVICE_TYPE` 决定起哪些服务：`proof`（存证，起 rpc）、`browser`（浏览器，起 jrpc）、其它（全部）。
只支持 ES 7.x（包内 `env/es7` 为 7.17，`env/es7.4` 备用）。详细步骤见 [docs/deploy.md](docs/deploy.md)。

## 文档

| 文档 | 内容 |
|---|---|
| [docs/deploy.md](docs/deploy.md) | 部署：两个包的区别、`.env` 配置、启动与运维 |
| [convert/README.md](convert/README.md) | 转换器框架、目录、如何新增一个转换器 |
| [lib/README.md](lib/README.md) | 底层库各子目录与依赖方向 |
| [docs/requirements/](docs/requirements/) | 需求文档（按模块分目录）|

## 依赖

- [chain33](https://github.com/33cn/chain33) / [plugin](https://github.com/33cn/plugin)：链、执行器、类型
- Elasticsearch 7.x（`olivere/elastic/v7`）
- MySQL 8（仅 ERC20 扫块/查询与 jrpc 的 `getTransferLogs` 可选）
- Go 1.24+
