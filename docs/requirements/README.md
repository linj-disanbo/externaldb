# 需求文档

本目录存放各模块/功能的需求描述文档。

## 目录组织

按功能模块划分子目录，一个模块一个目录：

```
docs/requirements/
├── README.md                      <- 本文件
└── erc20-scanner/                 <- ERC20 扫描/查询需求
    ├── token-approval-query.md    <- 代币授权额度查询
    └── token-holder-ranking.md    <- 代币持币排行
```

新增模块时按需建同名子目录（`docs/requirements/<模块>/`）。

## 模块与代码位置

| 需求目录 | 代码位置 | 说明 |
|---|---|---|
| `erc20-scanner/` | `erc20/`（`scanner`、`txparser`、`database`）+ `cmd/erc20scan`、`cmd/erc20api` | ERC20 扫块与查询，落 MySQL，独立于 ES 管道 |
| （ES 文档管道）| `convert/`（转换器）+ `lib/esdoc`（文档类型）+ `lib/schema`（契约） | 新增合约解析的需求写到 `docs/requirements/<合约>/` |
| （查询接口）| `cmd/jrpc`（JSON-RPC）、`cmd/rpc`（REST/存证） | 接口类需求注明是 jrpc 还是 rpc |

过程性、内部部署细节与排查记录放 `local/docs/`（不入库），本目录只放对外可公开的需求描述。

## 命名规则

- 目录名与模块名一致，使用 `kebab-case`
- 一个需求一个 `.md` 文件，命名简洁描述功能（如 `token-holder-ranking.md`）
- 同一模块多个需求文件放在同一目录下

## 文档模板

```markdown
# 功能名称

## 背景
为什么要做这个功能，解决什么问题

## 需求描述
功能的核心需求，分点列出

## 数据模型
涉及的数据表/字段/索引

## 接口
暴露的 API / RPC 接口

## 非功能需求
性能、可靠性、扩展性等

## 参考资料
相关链接、PR、设计文档
```
