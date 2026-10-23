package block

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/33cn/externaldb/lib/escli"
	esmaint "github.com/33cn/externaldb/lib/esmaint"
	"github.com/33cn/externaldb/lib/schema"
)

// 集成测试（需要真实 ES）：验证 RecoverAt 按高度取恢复点，而不是取全局最新。
// 运行：TEST_ES_ADDR=http://127.0.0.1:9297 go test ./stat/block/ -run TestRecoverAtIntegration
func TestRecoverAtIntegration(t *testing.T) {
	addr := os.Getenv("TEST_ES_ADDR")
	if addr == "" {
		t.Skip("未设置 TEST_ES_ADDR，跳过（需要真实 ES）")
	}
	cli, err := escli.NewESShortConnect(addr, "tstat_", 7, "", "")
	if err != nil {
		t.Fatalf("连接 ES 失败: %v", err)
	}
	if _, err := cli.Delete(DBStatX); err != nil {
		t.Logf("delete index: %v", err)
	}
	s := &blockStat{title: "test", coin: coin}
	if err := esmaint.InitIndex(cli, DBStatX, DBStatX, StatMapping); err != nil {
		t.Fatalf("建索引失败: %v", err)
	}
	// 写入 height=1..5 的累计记录（tx_count = height）
	var records []schema.Record
	for h := int64(1); h <= 5; h++ {
		records = append(records, makeRecord(&BStat{Height: h, TxCount: int(h), Time: h}, schema.OpAdd))
	}
	// 用 refresh=wait_for：写完立即可被 search 到（否则下面的 Search 读不到刚写的文档）
	assert.NoError(t, cli.BulkUpdateRefresh(records, escli.RefreshWaitFor))

	prev = nil
	// 恢复点 height=3 → 必须取 stat-3，而不是全局最新 stat-5
	assert.NoError(t, s.RecoverAt(cli, 3))
	if assert.NotNil(t, prev) {
		assert.Equal(t, int64(3), prev.Height, "应按恢复点取 <=3 的最新一条")
	}

	// 有 prev 但高度不同 → 重新恢复（崩溃重放时会用到）
	assert.NoError(t, s.RecoverAt(cli, 5))
	assert.Equal(t, int64(5), prev.Height)

	// 高度无上界（旧 Recover 语义）→ 取全局最新
	prev = nil
	assert.NoError(t, s.Recover(cli, 0))
	assert.Equal(t, int64(5), prev.Height)
}
