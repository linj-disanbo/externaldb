package esmaint

import (
	"testing"

	"github.com/33cn/externaldb/lib/escli"
	"github.com/33cn/externaldb/lib/proto"
	tml "github.com/BurntSushi/toml"
)

func TestInitLastSyncSeqCache(t *testing.T) {
	var cfg proto.ConfigNew
	if _, err := tml.DecodeFile("../../config/externaldb.toml", &cfg); err != nil {
		t.Skipf("跳过：读不到 config/externaldb.toml（%v）", err)
	}

	EsWrite, err := escli.NewESLongConnect(cfg.ConvertEs.Host, cfg.ConvertEs.Prefix, cfg.EsVersion, cfg.ConvertEs.User, cfg.ConvertEs.Pwd)
	if err != nil {
		t.Skipf("跳过：ES 连不上（%v）", err)
	}
	if err := InitLastSyncSeqCache(EsWrite, "last_seq", cfg.Sync.StartSeq); err != nil {
		t.Errorf("InitLastSyncSeqCache() error = %v", err)
	}

}
