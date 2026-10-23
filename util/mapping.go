package util

import (
	"encoding/json"

	"github.com/33cn/externaldb/lib/proto"
)

var mappings = make(map[string]interface{})

func InitMapSet(esVersion int32, esIndex *proto.ESIndex) {
	if esVersion != 0 && esVersion != 7 {
		log.Warn("InitMapSet: only es7 supported, ignore esVersion", "version", esVersion)
	}
	// ES7：设置索引的分片和副本数
	if esIndex != nil {
		settingMap(esIndex.NumberOfShards, esIndex.NumberOfReplicas)
	}
}

// es7时设置分片和副本数
func settingMap(numberOfShards, numberOfReplicas int32) {
	mappings["settings"] = map[string]interface{}{
		"number_of_shards":   numberOfShards,   //指索引要做多少个分片，只能在创建索引时指定，后期无法修改
		"number_of_replicas": numberOfReplicas, //指每个分片有多少个副本，后期可以动态修改
	}
}

// 组合mapping结构（只支持 ES7；typ/version 参数保留仅为兼容调用点）
func CombineMap(mapping, typ string, version int32) (string, error) {
	para := make(map[string]interface{})
	if err := json.Unmarshal([]byte(mapping), &para); err != nil {
		log.Error("json mapping Unmarshal", "err", err, "mapping", mapping)
		return "", err
	}

	if version != 0 && version != 7 {
		log.Warn("CombineMap: only es7 supported, ignore esVersion", "version", version)
	}
	// 保留 InitMapSet 设进 mappings["settings"] 的分片/副本数（原来这里把 settings 一起删了，
	// 导致 [esIndex] numberOfShards/numberOfReplicas 从来没生效，索引都是 ES 默认 1 分片 1 副本）。
	settings := mappings["settings"]
	for k := range mappings {
		delete(mappings, k)
	}
	if s, ok := para["settings"]; ok {
		settings = s // 单个 mapping 自带 settings 时以它为准
	}
	if settings != nil {
		mappings["settings"] = settings
	}
	mappings["mappings"] = para["mappings"]

	res, err := json.Marshal(mappings)
	if err != nil {
		log.Error("json mapping failed", "err", err)
		return "", err
	}
	return string(res), nil
}
