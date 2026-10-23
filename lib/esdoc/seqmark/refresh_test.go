package seqmark

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/33cn/externaldb/lib/escli"
	esmaint "github.com/33cn/externaldb/lib/esmaint"
)

// 集成测试（需要真实 ES）：验证"普通 bulk 写入 + Refresh 后即可被 Max 读到"。
// 运行：TEST_ES_ADDR=http://127.0.0.1:9297 go test ./db/seqmark/ -run TestRefreshVisibility
func TestRefreshVisibility(t *testing.T) {
	addr := os.Getenv("TEST_ES_ADDR")
	if addr == "" {
		t.Skip("未设置 TEST_ES_ADDR，跳过（需要真实 ES）")
	}
	cli, err := escli.NewESShortConnect(addr, "tmark_", 7, "", "")
	if err != nil {
		t.Fatalf("连接 ES 失败: %v", err)
	}
	_, _ = cli.Delete(TableName)
	if err := esmaint.InitIndex(cli, TableName, TableName, Mapping); err != nil {
		t.Fatalf("建索引失败: %v", err)
	}

	// 普通 bulk 写入（不等 refresh）
	assert.NoError(t, Save(cli, []*Marker{{Seq: 1, Height: 1, Hash: "0x1", Type: 1}}))
	// 显式 refresh 后必须能读到
	assert.NoError(t, Refresh(cli))
	seq, m, err := Max(cli)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), seq)
	if assert.NotNil(t, m) {
		assert.Equal(t, "0x1", m.Hash)
	}

	// 再写一条，Refresh 后 Max 跟上
	assert.NoError(t, Save(cli, []*Marker{{Seq: 2, Height: 2, Hash: "0x2", Type: 1}}))
	assert.NoError(t, Refresh(cli))
	seq, _, err = Max(cli)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), seq)
}
