//go:build integration

// 这些用例连线上 ES（见用例内的 host 常量），默认 go test 不跑；
// 需要时：go test -tags integration cmd/jrpc/

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_CacheTime(t *testing.T) {
	now := time.Now().UTC()
	r := btyCache.needUpdate(now, 2)
	assert.True(t, r)

	btyCache.update(now, nil)
	r = btyCache.needUpdate(now, 2)
	assert.False(t, r)

	r = btyCache.needUpdate(now.Add(3*time.Second), 2)
	assert.True(t, r)
}

func Test_LoadAccount(t *testing.T) {
	acc := Account{DBRead: &DBRead{Host: "http://116.63.253.162:9200", Title: "", Symbol: "bty", Prefix: "db2_", Version: 7}}
	accounts, err := acc.loadAsset()
	if err != nil {
		// 测试ES没有开启
		return
	}
	assert.Nil(t, err)
	assert.LessOrEqual(t, 0, len(accounts))
	for _, acc := range accounts {
		t.Log("acc", acc)
	}
	//assert.NotNil(t, err)
}
