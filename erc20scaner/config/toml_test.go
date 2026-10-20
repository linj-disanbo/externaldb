package config

import (
	"os"
	"path/filepath"
	"testing"
)

// 用仓库里的正式配置模板做一次映射断言：模板改了/字段拼错时这里先红。
func TestLoadTOMLRepoConfig(t *testing.T) {
	cfg, err := LoadTOML(filepath.Join("..", "..", "config", "externaldb.toml"))
	if err != nil {
		t.Fatalf("LoadTOML: %v", err)
	}

	if cfg.Node.GRPC != "localhost:8802" {
		t.Errorf("Node.GRPC = %q, want localhost:8802（来自 [chain].grpcHost）", cfg.Node.GRPC)
	}
	if cfg.Node.Symbol != "bty" {
		t.Errorf("Node.Symbol = %q, want bty", cfg.Node.Symbol)
	}
	if !cfg.ES.Enabled {
		t.Error("ES.Enabled 应为 true（[chain].grpcHost 非空 = chain33 模式）")
	}
	if cfg.ES.Host != "http://localhost:9200/" || cfg.ES.Prefix != "db01_" {
		t.Errorf("ES = %+v, want 来自 [convertEs]", cfg.ES)
	}
	if cfg.Database.Enabled {
		t.Error("Database.Enabled 应为 false（[mysql].enabled=false）")
	}
	if cfg.Database.DSN == "" || cfg.Database.DSN == GetDefaultConfig().Database.DSN {
		t.Errorf("Database.DSN 未取到 [mysql].dsn: %q", cfg.Database.DSN)
	}
	if cfg.Node.URL != "https://mainnet.bityuan.com/eth" {
		t.Errorf("Node.URL = %q, want [erc20scan].ethUrl", cfg.Node.URL)
	}
	if cfg.RPC.Port != "8080" {
		t.Errorf("RPC.Port = %q, want [erc20scan].apiPort", cfg.RPC.Port)
	}
	if cfg.Scanner.EndBlock != 0 {
		t.Errorf("Scanner.EndBlock = %d, want 0（0 = 跟随链尖）", cfg.Scanner.EndBlock)
	}
	if cfg.BalanceRefresher.Interval != "1h" || cfg.BalanceRefresher.BatchSize != 50 {
		t.Errorf("BalanceRefresher = %+v, want [erc20scan.balanceRefresher]", cfg.BalanceRefresher)
	}
	if cfg.Log.File != "./logs/app.log" {
		t.Errorf("Log.File = %q, want [erc20scan.log].file", cfg.Log.File)
	}
}

// 缺段时全部退回到默认值，不 panic。
func TestLoadTOMLMinimal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "externaldb.toml")
	if err := os.WriteFile(p, []byte("[chain]\ngrpcHost = \"127.0.0.1:8802\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadTOML(p)
	if err != nil {
		t.Fatalf("LoadTOML: %v", err)
	}
	def := GetDefaultConfig()
	if !cfg.ES.Enabled || cfg.Node.GRPC != "127.0.0.1:8802" {
		t.Errorf("chain 映射错误: %+v", cfg.Node)
	}
	if cfg.Scanner.EndBlock != def.Scanner.EndBlock {
		t.Errorf("EndBlock = %d, want 默认 %d", cfg.Scanner.EndBlock, def.Scanner.EndBlock)
	}
	if cfg.RPC.Port != def.RPC.Port {
		t.Errorf("Port = %q, want 默认 %q", cfg.RPC.Port, def.RPC.Port)
	}
}

// 扩展名分发：.toml 走 proto 映射，yaml 路径保持老行为。
func TestLoadConfigFileDispatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "c.toml"), []byte("[chain]\nsymbol = \"ycc\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfigFile(filepath.Join(dir, "c.toml"))
	if err != nil || cfg.Node.Symbol != "ycc" {
		t.Fatalf("toml 分发失败: %v %+v", err, cfg)
	}

	if err := os.WriteFile(filepath.Join(dir, "c.yaml"), []byte("node:\n  symbol: bty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfigFile(filepath.Join(dir, "c.yaml"))
	if err != nil || cfg.Node.Symbol != "bty" {
		t.Fatalf("yaml 分发失败: %v %+v", err, cfg)
	}
}
