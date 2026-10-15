package v7

import (
	"testing"

	elasticV7 "github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"

	"github.com/33cn/externaldb/db"
)

func item(action string, v *elasticV7.BulkResponseItem) map[string]*elasticV7.BulkResponseItem {
	return map[string]*elasticV7.BulkResponseItem{action: v}
}

func respItem(index, id string, status int) *elasticV7.BulkResponseItem {
	return &elasticV7.BulkResponseItem{Index: index, Id: id, Status: status}
}

func TestCollectBulkFailures(t *testing.T) {
	assert.Nil(t, collectBulkFailures(nil), "nil 响应按成功处理")
	assert.Nil(t, bulkFailureError(nil, nil))

	// 全部成功：_bulk 的 HTTP 200 不代表 item 成功，这里逐条 status 判定
	ok := &elasticV7.BulkResponse{Items: []map[string]*elasticV7.BulkResponseItem{
		item("index", respItem("db05_account", "a", 201)),
		item("delete", respItem("db05_account", "b", 200)),
	}}
	fails := collectBulkFailures(ok)
	assert.Empty(t, fails)
	assert.NoError(t, bulkFailureError(ok, fails))

	// 部分失败：必须收集到，且错误信息带条数/状态/原因
	bad := &elasticV7.BulkResponse{Errors: true, Items: []map[string]*elasticV7.BulkResponseItem{
		item("index", respItem("db05_account", "a", 201)),
		item("index", &elasticV7.BulkResponseItem{
			Index: "db05_account", Id: "c", Status: 400,
			Error: &elasticV7.ErrorDetails{Type: "mapper_parsing_exception", Reason: "failed to parse field"},
		}),
	}}
	fails = collectBulkFailures(bad)
	if assert.Len(t, fails, 1) {
		assert.Equal(t, "db05_account", fails[0].Index)
		assert.Equal(t, "c", fails[0].ID)
		assert.Equal(t, 400, fails[0].Status)
		assert.Contains(t, fails[0].Reason, "mapper_parsing_exception")
	}
	err := bulkFailureError(bad, fails)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "1/2 item(s) failed")
		assert.Contains(t, err.Error(), "status=400")
		assert.Contains(t, err.Error(), "mapper_parsing_exception")
		var be *BulkError
		if assert.ErrorAs(t, err, &be) {
			assert.Equal(t, 2, be.Total)
			assert.Len(t, be.Failures, 1)
		}
	}

	// item 为 nil 不 panic
	withNil := &elasticV7.BulkResponse{Items: []map[string]*elasticV7.BulkResponseItem{
		item("index", nil),
	}}
	assert.Empty(t, collectBulkFailures(withNil))
}

func TestBulkFailureErrorTruncates(t *testing.T) {
	resp := &elasticV7.BulkResponse{}
	var fails []ItemFailure
	for i := 0; i < bulkFailurePreview+3; i++ {
		resp.Items = append(resp.Items, item("index", respItem("db05_x", "id", 500)))
		fails = append(fails, ItemFailure{Index: "db05_x", ID: "id", Status: 500})
	}
	err := bulkFailureError(resp, fails)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "8/8 item(s) failed")
		assert.Contains(t, err.Error(), "...and 3 more")
	}
}

type fakeRecord struct {
	*db.IKey
	*db.Op
}

func (r *fakeRecord) Value() []byte { return []byte(`{}`) }

func newFakeRecord(index, id string) *fakeRecord {
	return &fakeRecord{IKey: db.NewIKey(index, index, id), Op: db.NewOp(db.OpAdd)}
}

func TestFilterFailed(t *testing.T) {
	cli := &ESClientV7{Prefix: "seq05_"}
	rs := []db.Record{
		newFakeRecord("account", "a"),
		newFakeRecord("account", "b"),
		newFakeRecord("block_info", "c"),
	}
	// 失败项命中（物理索引带前缀）
	bulkErr := &BulkError{Total: 3, Failures: []ItemFailure{
		{Index: "seq05_account", ID: "b", Status: 400, Reason: "boom"},
	}}
	got := cli.FilterFailed(rs, bulkErr)
	if assert.Len(t, got, 1) {
		assert.Equal(t, "b", got[0].ID())
		assert.Equal(t, "account", got[0].Index())
	}

	// 无法定位 / nil 错误：返回 nil，调用方按整批失败处理
	assert.Nil(t, cli.FilterFailed(rs, nil))
	assert.Nil(t, cli.FilterFailed(rs, assert.AnError))
	assert.Nil(t, cli.FilterFailed(rs, &BulkError{}))
	assert.Nil(t, cli.FilterFailed(rs, &BulkError{Failures: []ItemFailure{{Index: "seq05_account", ID: "nope"}}}))
}
