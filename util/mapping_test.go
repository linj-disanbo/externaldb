package util

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/33cn/externaldb/lib/proto"
)

// [esIndex] 的 numberOfShards/numberOfReplicas 必须真的写进创建索引的 body
func TestCombineMapKeepsIndexSettings(t *testing.T) {
	InitMapSet(7, &proto.ESIndex{NumberOfShards: 3, NumberOfReplicas: 0})

	out, err := CombineMap(`{"mappings":{"properties":{"a":{"type":"long"}}}}`, "", 7)
	assert.NoError(t, err)

	var got map[string]interface{}
	assert.NoError(t, json.Unmarshal([]byte(out), &got))

	settings, ok := got["settings"].(map[string]interface{})
	if assert.True(t, ok, "settings 应保留，实际: %v", got["settings"]) {
		assert.Equal(t, float64(3), settings["number_of_shards"])
		assert.Equal(t, float64(0), settings["number_of_replicas"])
	}
	assert.NotNil(t, got["mappings"], "mappings 不能丢")

	// 单个 mapping 自带 settings 时优先用它
	out, err = CombineMap(`{"settings":{"number_of_shards":1},"mappings":{}}`, "", 7)
	assert.NoError(t, err)
	got = map[string]interface{}{}
	assert.NoError(t, json.Unmarshal([]byte(out), &got))
	settings, _ = got["settings"].(map[string]interface{})
	if assert.NotNil(t, settings) {
		assert.Equal(t, float64(1), settings["number_of_shards"])
	}
}
