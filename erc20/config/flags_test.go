package config

import (
	"flag"
	"strings"
	"testing"
)

func TestDefineCommonFlags(t *testing.T) {
	// 重置 flag 包
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)

	flags := DefineCommonFlags()

	if flags == nil {
		t.Fatal("DefineCommonFlags returned nil")
	}

	// 验证所有字段都已初始化
	if flags.ConfigFile == nil {
		t.Error("ConfigFile should not be nil")
	}
	if flags.NodeURL == nil {
		t.Error("NodeURL should not be nil")
	}
	if flags.DBEnabled == nil {
		t.Error("DBEnabled should not be nil")
	}
	if flags.DBDSN == nil {
		t.Error("DBDSN should not be nil")
	}
	if flags.ESEnabled == nil {
		t.Error("ESEnabled should not be nil")
	}
	if flags.ESHost == nil {
		t.Error("ESHost should not be nil")
	}
	if flags.ESPrefix == nil {
		t.Error("ESPrefix should not be nil")
	}
	if flags.ESVersion == nil {
		t.Error("ESVersion should not be nil")
	}
	if flags.ESUser == nil {
		t.Error("ESUser should not be nil")
	}
	if flags.ESPassword == nil {
		t.Error("ESPassword should not be nil")
	}
	if flags.ChainGRPC == nil {
		t.Error("ChainGRPC should not be nil")
	}
	if flags.ChainSymbol == nil {
		t.Error("ChainSymbol should not be nil")
	}
}

func TestDefineScannerFlags(t *testing.T) {
	// 重置 flag 包
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)

	flags := DefineScannerFlags()

	if flags == nil {
		t.Fatal("DefineScannerFlags returned nil")
	}

	// 验证 CommonFlags 已初始化
	if flags.CommonFlags.ConfigFile == nil {
		t.Error("CommonFlags.ConfigFile should not be nil")
	}

	// 验证 Scanner 专用字段
	if flags.StartBlock == nil {
		t.Error("StartBlock should not be nil")
	}
	if flags.EndBlock == nil {
		t.Error("EndBlock should not be nil")
	}
}

func TestDefineRPCFlags(t *testing.T) {
	// 重置 flag 包
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)

	flags := DefineRPCFlags()

	if flags == nil {
		t.Fatal("DefineRPCFlags returned nil")
	}

	// 验证 CommonFlags 已初始化
	if flags.CommonFlags.ConfigFile == nil {
		t.Error("CommonFlags.ConfigFile should not be nil")
	}

	// 验证 RPC 专用字段
	if flags.Port == nil {
		t.Error("Port should not be nil")
	}
}

func TestFlags_DefaultValues(t *testing.T) {
	// 注意：由于 flag 包使用全局变量，我们不能在同一个测试中多次调用 DefineCommonFlags
	// 这个测试只验证一次调用的默认值
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)

	commonFlags := DefineCommonFlags()

	// 验证默认值
	if *commonFlags.ConfigFile != "externaldb.toml" {
		t.Errorf("Expected ConfigFile default to be 'externaldb.toml', got '%s'", *commonFlags.ConfigFile)
	}
	if *commonFlags.NodeURL != "" {
		t.Errorf("Expected NodeURL default to be empty, got '%s'", *commonFlags.NodeURL)
	}
	if *commonFlags.DBEnabled != false {
		t.Errorf("Expected DBEnabled default to be false, got %v", *commonFlags.DBEnabled)
	}
}

func TestFlags_ScannerDefaultValues(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)

	scannerFlags := DefineScannerFlags()

	if *scannerFlags.StartBlock != 0 {
		t.Errorf("Expected StartBlock default to be 0, got %d", *scannerFlags.StartBlock)
	}
	if *scannerFlags.EndBlock != 0 {
		t.Errorf("Expected EndBlock default to be 0, got %d", *scannerFlags.EndBlock)
	}
}

func TestFlags_RPCDefaultValues(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)

	rpcFlags := DefineRPCFlags()

	if *rpcFlags.Port != "8080" {
		t.Errorf("Expected Port default to be '8080', got '%s'", *rpcFlags.Port)
	}
}

// newTestScannerFlags 重置全局 flag 集并返回新的 ScannerFlags，供 -mode 校验用例使用
func newTestScannerFlags(t *testing.T) *ScannerFlags {
	t.Helper()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	return DefineScannerFlags()
}

func TestScannerFlags_FixMode(t *testing.T) {
	tests := []struct {
		name    string
		set     map[string]string
		wantFix bool
		wantErr string
	}{
		{
			name:    "默认 run 模式",
			set:     nil,
			wantFix: false,
		},
		{
			name:    "run 模式带 -s/-e 仍不是修复",
			set:     map[string]string{"mode": "run", "s": "10", "e": "20"},
			wantFix: false,
		},
		{
			name:    "fix 模式区间合法",
			set:     map[string]string{"mode": "fix", "s": "10", "e": "20"},
			wantFix: true,
		},
		{
			name:    "fix 模式允许单块区间",
			set:     map[string]string{"mode": "fix", "s": "7", "e": "7"},
			wantFix: true,
		},
		{
			name:    "fix 模式缺 -e",
			set:     map[string]string{"mode": "fix", "s": "10"},
			wantErr: "必须显式指定 -s 与 -e",
		},
		{
			name:    "fix 模式缺 -s",
			set:     map[string]string{"mode": "fix", "e": "10"},
			wantErr: "必须显式指定 -s 与 -e",
		},
		{
			name:    "fix 模式 e 小于 s",
			set:     map[string]string{"mode": "fix", "s": "20", "e": "10"},
			wantErr: "-e >= -s",
		},
		{
			name:    "fix 模式 e 为 0（engine 哨兵值）",
			set:     map[string]string{"mode": "fix", "s": "0", "e": "0"},
			wantErr: "-e >= 1",
		},
		{
			name:    "非法 mode",
			set:     map[string]string{"mode": "repair", "s": "1", "e": "2"},
			wantErr: "-mode 只支持",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := newTestScannerFlags(t)
			for k, v := range tt.set {
				if err := flag.Set(k, v); err != nil {
					t.Fatalf("set flag %s=%s: %v", k, v, err)
				}
			}

			fix, err := flags.FixMode()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (fix=%v)", tt.wantErr, fix)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fix != tt.wantFix {
				t.Fatalf("fix mode = %v, want %v", fix, tt.wantFix)
			}
		})
	}
}
