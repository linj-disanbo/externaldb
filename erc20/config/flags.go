package config

import (
	"errors"
	"flag"
	"fmt"
)

// CommonFlags 通用命令行参数（scanner 和 rpc 共用）
type CommonFlags struct {
	ConfigFile  *string
	NodeURL     *string
	DBEnabled   *bool
	DBDSN       *string
	ESEnabled   *bool
	ESHost      *string
	ESPrefix    *string
	ESVersion   *int
	ESUser      *string
	ESPassword  *string
	ChainGRPC   *string
	ChainSymbol *string
}

// Scanner 运行模式
const (
	// ScannerModeRun 跟随链尖（默认）：从进度游标继续扫，读/写 scan_progress 与 scan_seq_marker
	ScannerModeRun = "run"
	// ScannerModeFix 区间修复：只处理 -s..-e（闭区间），不读/写进度，处理完即退出
	ScannerModeFix = "fix"
)

// ScannerFlags Scanner 专用参数
type ScannerFlags struct {
	CommonFlags
	StartBlock *int64
	EndBlock   *int64
	Mode       *string
}

// RPCFlags RPC 专用参数
type RPCFlags struct {
	CommonFlags
	Port *string
}

// DefineCommonFlags 定义通用命令行参数
func DefineCommonFlags() *CommonFlags {
	return &CommonFlags{
		ConfigFile:  flag.String("c", "externaldb.toml", "config file path (.toml = externaldb.toml, 其他按 yaml 解析)"),
		NodeURL:     flag.String("u", "", "node url (overrides config)"),
		DBEnabled:   flag.Bool("db", false, "enable database (overrides config)"),
		DBDSN:       flag.String("dsn", "", "database DSN (overrides config)"),
		ESEnabled:   flag.Bool("es", false, "enable ES mode (overrides config)"),
		ESHost:      flag.String("es-host", "", "ES host (overrides config)"),
		ESPrefix:    flag.String("es-prefix", "", "ES prefix (overrides config)"),
		ESVersion:   flag.Int("es-version", 0, "ES version (6 or 7, overrides config)"),
		ESUser:      flag.String("es-user", "", "ES username (overrides config)"),
		ESPassword:  flag.String("es-pwd", "", "ES password (overrides config)"),
		ChainGRPC:   flag.String("chain-grpc", "", "Chain33 gRPC host (overrides config)"),
		ChainSymbol: flag.String("chain-symbol", "", "Chain33 symbol (overrides config)"),
	}
}

// DefineScannerFlags 定义 Scanner 专用参数
func DefineScannerFlags() *ScannerFlags {
	common := DefineCommonFlags()
	return &ScannerFlags{
		CommonFlags: *common,
		StartBlock:  flag.Int64("s", 0, "start block/seq (overrides config); fix 模式必填（闭区间起点）"),
		EndBlock:    flag.Int64("e", 0, "end block/seq (overrides config, use -1 for unlimited); fix 模式必填（闭区间终点）"),
		Mode:        flag.String("mode", ScannerModeRun, "运行模式：run=跟随链尖；fix=只修复 -s..-e 区间后退出（不读/写进度）"),
	}
}

// FixMode 校验 -mode 并返回是否为修复模式。
// fix 模式要求显式给出 -s/-e（不接受配置里的 start_block/end_block 兜底）且 e >= s，
// 因为修复是显式区间操作，配置里的 end_block 通常是 -1（不限），落到 fix 会变成一直跑。
func (f *ScannerFlags) FixMode() (bool, error) {
	mode := ScannerModeRun
	if f.Mode != nil && *f.Mode != "" {
		mode = *f.Mode
	}

	switch mode {
	case ScannerModeRun:
		return false, nil
	case ScannerModeFix:
	default:
		return false, fmt.Errorf("-mode 只支持 %s/%s（收到 %q）", ScannerModeRun, ScannerModeFix, mode)
	}

	info := DetectFlagSet()
	if !info.StartBlock || !info.EndBlock {
		return false, errors.New("fix 模式必须显式指定 -s 与 -e（闭区间，不接受配置文件里的 start_block/end_block）")
	}

	var start, end int64
	if f.StartBlock != nil {
		start = *f.StartBlock
	}
	if f.EndBlock != nil {
		end = *f.EndBlock
	}
	if start < 0 {
		return false, fmt.Errorf("fix 模式要求 -s >= 0（收到 %d）", start)
	}
	if end < start {
		return false, fmt.Errorf("fix 模式要求 -e >= -s（收到 s=%d e=%d）", start, end)
	}
	if end < 1 {
		// engine 里 EndPoint=0 表示“不限”，fix 模式不能借用这个哨兵值
		return false, errors.New("fix 模式要求 -e >= 1（0 在扫描器里表示不限）")
	}

	return true, nil
}

// DefineRPCFlags 定义 RPC 专用参数
func DefineRPCFlags() *RPCFlags {
	common := DefineCommonFlags()
	return &RPCFlags{
		CommonFlags: *common,
		Port:        flag.String("port", "8080", "HTTP server port"),
	}
}
