# ERC20 Scanner 配置管理开发指南

> **2026-10 布局变更**：程序已并入主 module —— `cmd/erc20scan`（scanner）、`cmd/erc20api`（原 rpc 服务）、
> `cmd/tools/erc20-*`（离线工具），配置统一读 `externaldb.toml` 的 `[mysql]` / `[erc20scan]`（yaml 仅兼容）。
> 下文出现的 `erc20Scaner/`、`./scanner`、`./rpc`、`-c config.yaml` 属旧布局，按新路径理解。


## 概述

本指南说明如何在 erc20Scaner 项目中添加新的配置项和命令行参数，以及如何正确使用统一的配置管理系统。

## 代码结构

```
erc20Scaner/
├── config/
│   ├── config.go          # 配置结构定义
│   ├── flags.go           # 通用 flag 定义
│   └── loader.go          # 统一配置加载和合并逻辑
│
├── scanner/
│   └── main.go            # Scanner 程序入口（使用通用配置）
│
└── rpc/
    └── main.go            # RPC 程序入口（使用通用配置）
```

## 配置优先级

配置的优先级从高到低为：
1. **命令行参数**（最高优先级）
2. **配置文件**（config.yaml）
3. **默认值**（代码中定义）

## 段名格式

保持 flag 名称和配置文件字段名的一致性（flag 使用 kebab-case，配置文件使用 snake_case）

## 添加新配置项的步骤

### 1. 在 `config/config.go` 中添加配置结构

```go
// Config 配置结构
type Config struct {
    Node     NodeConfig     `yaml:"node"`
    Scanner  ScannerConfig  `yaml:"scanner"`
    RPC      RPCConfig      `yaml:"rpc"`
    Database DatabaseConfig `yaml:"database"`
    ES       ESConfig       `yaml:"es"`
    Log      LogConfig      `yaml:"log"`
    // 新增配置项
    NewFeature NewFeatureConfig `yaml:"new_feature"`
}

// NewFeatureConfig 新功能配置
type NewFeatureConfig struct {
    Enabled bool   `yaml:"enabled"`
    Host    string `yaml:"host"`
    Port    int    `yaml:"port"`
}
```

### 2. 在 `config/config.go` 的 `GetDefaultConfig()` 中添加默认值

```go
func GetDefaultConfig() *Config {
    return &Config{
        // ... 其他配置
        NewFeature: NewFeatureConfig{
            Enabled: false,
            Host:    "localhost",
            Port:    9000,
        },
    }
}
```

### 3. 在 `config/flags.go` 中添加命令行参数

#### 如果是通用参数（scanner 和 rpc 都使用）

在 `CommonFlags` 结构体中添加：

```go
type CommonFlags struct {
    // ... 现有字段
    NewFeatureEnabled *bool   // 新功能的启用标志
    NewFeatureHost    *string // 新功能的主机地址
    NewFeaturePort    *int    // 新功能的端口
}
```

在 `DefineCommonFlags()` 函数中添加 flag 定义：

```go
func DefineCommonFlags() *CommonFlags {
    return &CommonFlags{
        // ... 现有字段
        NewFeatureEnabled: flag.Bool("new-feature", false, "enable new feature (overrides config)"),
        NewFeatureHost:    flag.String("new-feature-host", "", "new feature host (overrides config)"),
        NewFeaturePort:    flag.Int("new-feature-port", 0, "new feature port (overrides config)"),
    }
}
```

#### 如果是 Scanner 专用参数

在 `ScannerFlags` 结构体中添加：

```go
type ScannerFlags struct {
    CommonFlags
    StartBlock *int64
    EndBlock   *int64
    // 新增 Scanner 专用参数
    ScanInterval *int64 // 扫描间隔
}
```

在 `DefineScannerFlags()` 函数中添加：

```go
func DefineScannerFlags() *ScannerFlags {
    common := DefineCommonFlags()
    return &ScannerFlags{
        CommonFlags: *common,
        StartBlock:  flag.Int64("s", 0, "start block (overrides config)"),
        EndBlock:    flag.Int64("e", 0, "end block (overrides config, use -1 for unlimited)"),
        ScanInterval: flag.Int64("scan-interval", 0, "scan interval in seconds (overrides config)"),
    }
}
```

#### 如果是 RPC 专用参数

在 `RPCFlags` 结构体中添加：

```go
type RPCFlags struct {
    CommonFlags
    Port *string
    // 新增 RPC 专用参数
    MaxConcurrentRequests *int // 最大并发请求数
}
```

在 `DefineRPCFlags()` 函数中添加：

```go
func DefineRPCFlags() *RPCFlags {
    common := DefineCommonFlags()
    return &RPCFlags{
        CommonFlags: *common,
        Port: flag.String("port", "8080", "HTTP server port"),
        MaxConcurrentRequests: flag.Int("max-requests", 0, "max concurrent requests (overrides config)"),
    }
}
```

### 4. 在 `config/loader.go` 中添加 flag 检测逻辑

在 `DetectFlagSet()` 函数中添加：

```go
func DetectFlagSet() *FlagSetInfo {
    info := &FlagSetInfo{}
    flag.Visit(func(f *flag.Flag) {
        switch f.Name {
        // ... 现有 case
        case "new-feature":
            info.NewFeatureEnabled = true
        case "new-feature-host":
            info.NewFeatureHost = true
        case "new-feature-port":
            info.NewFeaturePort = true
        case "scan-interval":
            info.ScanInterval = true
        case "max-requests":
            info.MaxConcurrentRequests = true
        }
    })
    return info
}
```

在 `FlagSetInfo` 结构体中添加字段：

```go
type FlagSetInfo struct {
    // ... 现有字段
    NewFeatureEnabled bool
    NewFeatureHost    bool
    NewFeaturePort    bool
    ScanInterval      bool
    MaxConcurrentRequests bool
}
```

### 5. 在 `config/loader.go` 中添加配置合并逻辑

#### 通用配置合并

在 `MergeConfigWithFlagsFromStruct()` 函数中添加：

```go
func MergeConfigWithFlagsFromStruct(cfg *Config, flags *CommonFlags, flagInfo *FlagSetInfo) {
    // ... 现有合并逻辑
    
    if flagInfo.NewFeatureEnabled && flags.NewFeatureEnabled != nil {
        cfg.NewFeature.Enabled = *flags.NewFeatureEnabled
    }
    if flagInfo.NewFeatureHost && flags.NewFeatureHost != nil && *flags.NewFeatureHost != "" {
        cfg.NewFeature.Host = *flags.NewFeatureHost
    }
    if flagInfo.NewFeaturePort && flags.NewFeaturePort != nil && *flags.NewFeaturePort != 0 {
        cfg.NewFeature.Port = *flags.NewFeaturePort
    }
}
```

#### Scanner 专用配置合并

在 `LoadAndMergeForScanner()` 函数中添加：

```go
func LoadAndMergeForScanner(flags *ScannerFlags) (*Config, error) {
    // ... 现有逻辑
    
    // 合并 Scanner 专用配置
    if flagInfo.ScanInterval && flags.ScanInterval != nil {
        // 如果配置结构中有对应字段，在这里合并
        // cfg.Scanner.ScanInterval = *flags.ScanInterval
    }
    
    return cfg, nil
}
```

#### RPC 专用配置合并

在 `LoadAndMergeForRPC()` 函数中添加：

```go
func LoadAndMergeForRPC(flags *RPCFlags) (*Config, error) {
    // ... 现有逻辑
    
    // 合并 RPC 专用配置
    if flagInfo.MaxConcurrentRequests && flags.MaxConcurrentRequests != nil {
        // 如果配置结构中有对应字段，在这里合并
        // cfg.RPC.MaxConcurrentRequests = *flags.MaxConcurrentRequests
    }
    
    return cfg, nil
}
```

### 6. 在程序中使用配置

#### Scanner 中使用

在 `scanner/main.go` 中：

```go
func main() {
    flags := config.DefineScannerFlags()
    flag.Parse()
    
    cfg, err := config.LoadAndMergeForScanner(flags)
    if err != nil {
        // 处理错误
    }
    
    // 使用配置
    if cfg.NewFeature.Enabled {
        // 使用新功能
        host := cfg.NewFeature.Host
        port := cfg.NewFeature.Port
    }
}
```

#### RPC 中使用

在 `rpc/main.go` 中：

```go
func main() {
    flags := config.DefineRPCFlags()
    flag.Parse()
    
    cfg, err := config.LoadAndMergeForRPC(flags)
    if err != nil {
        // 处理错误
    }
    
    // 获取最终配置值（flag 优先，否则使用配置文件）
    flagInfo := config.DetectFlagSet()
    newFeatureHost := config.GetFinalValue(
        flags.NewFeatureHost, 
        flagInfo.NewFeatureHost, 
        cfg.NewFeature.Host, 
        "localhost", // 默认值
    )
}
```

## 配置文件示例

`config.yaml` 示例：

```yaml
node:
  url: "https://mainnet.bityuan.com/eth"
  grpc: "localhost:8802"
  symbol: "bty"

scanner:
  start_block: 42399544
  end_block: -1

rpc:
  port: "8080"

database:
  enabled: true
  dsn: "root:password@tcp(localhost:3306)/token_scanner?charset=utf8mb4&parseTime=True&loc=Local"

es:
  enabled: false
  host: "http://localhost:9200/"
  prefix: "db01_"
  version: 7
  user: "elastic"
  password: "elastic"

log:
  level: "info"
  file: ""

# 新增配置项
new_feature:
  enabled: false
  host: "localhost"
  port: 9000
```

## 使用命令行参数覆盖配置

### Scanner

```bash
# 使用配置文件
./scanner -c config.yaml

# 使用命令行参数覆盖配置
./scanner -c config.yaml -s 1000000 -e 2000000 -db -dsn "user:pass@tcp(host:3306)/db"

# 使用 ES 模式
./scanner -c config.yaml -es -es-host "http://es:9200/" -es-prefix "db01_"
```

### RPC

```bash
# 使用配置文件
./rpc -c config.yaml

# 使用命令行参数覆盖配置
./rpc -c config.yaml -port 9090 -chain-grpc "localhost:8802"

# 使用 ES 配置
./rpc -c config.yaml -es-host "http://es:9200/" -es-prefix "db01_"
```

## 最佳实践

1. **统一使用配置包**：所有配置相关的代码都应该在 `config` 包中
2. **保持向后兼容**：添加新配置时，确保默认值与现有行为一致
3. **使用类型安全**：使用结构体管理 flag，避免直接使用全局 flag 变量
4. **文档化配置**：在配置结构体和 flag 定义中添加清晰的注释
5. **测试配置加载**：确保配置加载和合并逻辑正确工作

## 常见问题

### Q: 如何添加一个只在 Scanner 中使用的配置项？

A: 
1. 在 `ScannerConfig` 结构体中添加字段
2. 在 `ScannerFlags` 结构体中添加对应的 flag
3. 在 `DefineScannerFlags()` 中定义 flag
4. 在 `LoadAndMergeForScanner()` 中合并配置

### Q: 如何添加一个 scanner 和 rpc 都使用的配置项？

A:
1. 在对应的配置结构体中添加字段（如 `NodeConfig`、`DatabaseConfig` 等）
2. 在 `CommonFlags` 结构体中添加 flag
3. 在 `DefineCommonFlags()` 中定义 flag
4. 在 `MergeConfigWithFlagsFromStruct()` 中合并配置

### Q: 配置文件的字段名和 flag 名称不一致怎么办？

A: 
- 配置文件使用 YAML 标签（如 `yaml:"new_feature"`）
- Flag 名称使用命令行风格（如 `new-feature`）
- 在 `DetectFlagSet()` 中确保 flag 名称匹配正确

### Q: 如何获取最终配置值（考虑 flag 和配置文件）？

A: 使用 `config.GetFinalValue()` 辅助函数：

```go
flagInfo := config.DetectFlagSet()
finalValue := config.GetFinalValue(
    flags.SomeFlag,      // flag 值
    flagInfo.SomeFlag,   // flag 是否被设置
    cfg.SomeConfig,      // 配置文件中的值
    "default",           // 默认值
)
```

## 代码示例

### 完整示例：添加一个新的数据库连接池大小配置

1. **在 `config/config.go` 中添加**：

```go
type DatabaseConfig struct {
    Enabled bool   `yaml:"enabled"`
    DSN     string `yaml:"dsn"`
    MaxOpenConns int `yaml:"max_open_conns"` // 新增
}
```

2. **在 `config/flags.go` 中添加**：

```go
type CommonFlags struct {
    // ...
    DBMaxOpenConns *int
}

func DefineCommonFlags() *CommonFlags {
    return &CommonFlags{
        // ...
        DBMaxOpenConns: flag.Int("db-max-conns", 0, "database max open connections (overrides config)"),
    }
}
```

3. **在 `config/loader.go` 中添加**：

```go
type FlagSetInfo struct {
    // ...
    DBMaxOpenConns bool
}

func DetectFlagSet() *FlagSetInfo {
    // ...
    case "db-max-conns":
        info.DBMaxOpenConns = true
}

func MergeConfigWithFlagsFromStruct(cfg *Config, flags *CommonFlags, flagInfo *FlagSetInfo) {
    // ...
    if flagInfo.DBMaxOpenConns && flags.DBMaxOpenConns != nil && *flags.DBMaxOpenConns != 0 {
        cfg.Database.MaxOpenConns = *flags.DBMaxOpenConns
    }
}
```

4. **在程序中使用**：

```go
cfg, _ := config.LoadAndMergeForScanner(flags)
maxConns := cfg.Database.MaxOpenConns
if maxConns == 0 {
    maxConns = 10 // 默认值
}
```

## 总结

通过统一的配置管理系统，我们可以：
- ✅ 减少代码重复
- ✅ 保持配置加载逻辑一致
- ✅ 简化新配置项的添加
- ✅ 提高代码可维护性

遵循本指南，可以确保新添加的配置项与现有系统保持一致，并且易于维护和扩展。

