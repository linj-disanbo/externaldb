package esmaint

import (
	"testing"

	"github.com/33cn/externaldb/lib/escli"
	"github.com/33cn/externaldb/lib/proto"
	tml "github.com/BurntSushi/toml"
)

// esClientForTest 读仓库根的配置连 ES；连不上就跳过（这两个用例依赖真实 ES）。
//
// 注意：不要用 os.Exit —— 在 go test 里会变成 "unexpected call to os.Exit" panic，
// 让整个包的测试看起来是崩了，而不是跳过。
func esClientForTest(t *testing.T) (*proto.ConfigNew, escli.ESClient) {
	t.Helper()
	var cfg proto.ConfigNew
	if _, err := tml.DecodeFile("../../config/externaldb.toml", &cfg); err != nil {
		t.Skipf("跳过：读不到 config/externaldb.toml（%v）", err)
	}

	cli, err := escli.NewESLongConnect(cfg.ConvertEs.Host, cfg.ConvertEs.Prefix, cfg.EsVersion, cfg.ConvertEs.User, cfg.ConvertEs.Pwd)
	if err != nil {
		t.Skipf("跳过：ES 连不上（%v）", err)
	}
	return &cfg, cli
}

func Test_es6CheckAndRepair(t *testing.T) {
	cfg, EsWrite := esClientForTest(t)

	if err := es6CheckAndRepair(cfg, EsWrite); err != nil {
		t.Errorf("es6CheckAndRepair() error = %v", err)
	}
}

func Test_es7CheckAndRepair(t *testing.T) {
	cfg, EsWrite := esClientForTest(t)

	if err := es7CheckAndRepair(cfg, EsWrite); err != nil {
		t.Errorf("es7CheckAndRepair() error = %v", err)
	}
}
