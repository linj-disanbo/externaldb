package config

import (
	"fmt"
	"path/filepath"
	"strings"

	tml "github.com/BurntSushi/toml"

	"github.com/33cn/externaldb/proto"
)

// LoadConfigFile 按扩展名选择配置格式：
//   - .toml：externaldb.toml（与 convert/jrpc 等程序共用同一份配置）
//   - 其他：老的 yaml 配置（保留兼容）
func LoadConfigFile(path string) (*Config, error) {
	if strings.EqualFold(filepath.Ext(path), ".toml") {
		return LoadTOML(path)
	}
	return LoadConfig(path)
}

// LoadTOML 从 externaldb.toml 载入 [chain]/[convertEs]/[mysql]/[erc20scan]。
//
// 与 jrpc 等程序读的是同一个 proto.ConfigNew，字段按 TOML 键名匹配（大小写不敏感）。
func LoadTOML(path string) (*Config, error) {
	var ex proto.ConfigNew
	if _, err := tml.DecodeFile(path, &ex); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return FromProto(&ex), nil
}

// FromProto 把主配置映射成 erc20 扫描/查询程序用的结构，未配置的项保留 GetDefaultConfig 的默认值。
func FromProto(ex *proto.ConfigNew) *Config {
	cfg := GetDefaultConfig()

	if c := ex.GetChain(); c != nil {
		if c.GrpcHost != "" {
			cfg.Node.GRPC = c.GrpcHost
		}
		if c.Symbol != "" {
			cfg.Node.Symbol = c.Symbol
		}
		// ES.Enabled 是历史字段名，实际含义为"chain33 模式"：按 seq 从节点 gRPC 取块，
		// 不再依赖 sync 程序写进 ES 的 block.Seq。统一配置下 [chain].grpcHost 必填。
		cfg.ES.Enabled = c.GrpcHost != ""
	}
	if es := ex.GetConvertEs(); es != nil {
		if es.Host != "" {
			cfg.ES.Host = es.Host
		}
		if es.Prefix != "" {
			cfg.ES.Prefix = es.Prefix
		}
		if es.User != "" {
			cfg.ES.User = es.User
		}
		if es.Pwd != "" {
			cfg.ES.Password = es.Pwd
		}
	}
	if ex.GetEsVersion() != 0 {
		cfg.ES.Version = ex.GetEsVersion()
	}
	if m := ex.GetMysql(); m != nil {
		cfg.Database.Enabled = m.GetEnabled()
		if m.GetDsn() != "" {
			cfg.Database.DSN = m.GetDsn()
		}
	}

	s := ex.GetErc20Scan()
	if s == nil {
		return cfg
	}
	if s.EthUrl != "" {
		cfg.Node.URL = s.EthUrl
	}
	// startBlock=0 表示从已存进度（无进度则链上起点）开始；endBlock=0 表示跟随链尖
	cfg.Scanner.StartBlock = s.StartBlock
	cfg.Scanner.EndBlock = s.EndBlock
	cfg.Scanner.SkipInlineBalanceUpdate = s.SkipInlineBalanceUpdate
	if s.ApiPort != "" {
		cfg.RPC.Port = s.ApiPort
	}
	if br := s.BalanceRefresher; br != nil {
		cfg.BalanceRefresher.Enabled = br.Enabled
		if br.Interval != "" {
			cfg.BalanceRefresher.Interval = br.Interval
		}
		if br.BatchSize > 0 {
			cfg.BalanceRefresher.BatchSize = int(br.BatchSize)
		}
		if br.Concurrency > 0 {
			cfg.BalanceRefresher.Concurrency = int(br.Concurrency)
		}
		if br.MinAge != "" {
			cfg.BalanceRefresher.MinAge = br.MinAge
		}
	}
	if lg := s.Log; lg != nil {
		if lg.Level != "" {
			cfg.Log.Level = lg.Level
		}
		if lg.File != "" {
			cfg.Log.File = lg.File
		}
		if lg.MaxSize > 0 {
			cfg.Log.MaxSize = int(lg.MaxSize)
		}
		if lg.MaxBackups > 0 {
			cfg.Log.MaxBackups = int(lg.MaxBackups)
		}
		if lg.MaxAge > 0 {
			cfg.Log.MaxAge = int(lg.MaxAge)
		}
	}
	return cfg
}
