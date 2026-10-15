package v7

import (
	"testing"

	elasticV7 "github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
)

func item(action string, v *elasticV7.BulkResponseItem) map[string]*elasticV7.BulkResponseItem {
	return map[string]*elasticV7.BulkResponseItem{action: v}
}

func TestCollectBulkFailures(t *testing.T) {
	assert.Nil(t, collectBulkFailures(nil), "nil 响应按成功处理")
	assert.Nil(t, bulkFailureError(nil, nil))

	// 全部成功：_bulk 的 HTTP 200 不代表 item 成功，这里逐条 status 判定
	ok := &elasticV7.BulkResponse{Items: []map[string]*elasticV7.BulkResponseItem{
		item("index", &elasticV7.BulkResponseItem{Index: "db05_account", Id: "a", Status: 201}),
		item("delete", &elasticV7.BulkResponseItem{Index: "db05_account", Id: "b", Status: 200}),
	}}
	fails := collectBulkFailures(ok)
	assert.Empty(t, fails)
	assert.NoError(t, bulkFailureError(ok, fails))

	// 部分失败：必须收集到，且错误信息带条数/状态/原因
	bad := &elasticV7.BulkResponse{Errors: true, Items: []map[string]*elasticV7.BulkResponseItem{
		item("index", &elasticV7.BulkResponseItem{Index: "db05_account", Id: "a", Status: 201}),
		item("index", &elasticV7.BulkResponseItem{
			Index: "db05_account", Id: "c", Status: 400,
			Error: &elasticV7.ErrorDetails{Type: "mapper_parsing_exception", Reason: "failed to parse field"},
		}),
	}}
	fails = collectBulkFailures(bad)
	if assert.Len(t, fails, 1) {
		assert.Equal(t, "index", fails[0].action)
		assert.Equal(t, "db05_account", fails[0].index)
		assert.Equal(t, "c", fails[0].id)
		assert.Equal(t, 400, fails[0].status)
		assert.Contains(t, fails[0].reason, "mapper_parsing_exception")
	}
	err := bulkFailureError(bad, fails)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "1/2 item(s) failed")
		assert.Contains(t, err.Error(), "status=400")
		assert.Contains(t, err.Error(), "mapper_parsing_exception")
	}

	// item 为 nil 不 panic
	withNil := &elasticV7.BulkResponse{Items: []map[string]*elasticV7.BulkResponseItem{
		item("index", nil),
	}}
	assert.Empty(t, collectBulkFailures(withNil))
}

func TestBulkFailureErrorTruncates(t *testing.T) {
	resp := &elasticV7.BulkResponse{}
	var fails []bulkFailure
	for i := 0; i < bulkFailurePreview+3; i++ {
		resp.Items = append(resp.Items, item("index", &elasticV7.BulkResponseItem{
			Index: "db05_x", Id: "id", Status: 500,
		}))
		fails = append(fails, bulkFailure{action: "index", index: "db05_x", id: "id", status: 500})
	}
	err := bulkFailureError(resp, fails)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "8/8 item(s) failed")
		assert.Contains(t, err.Error(), "...and 3 more")
	}
}
