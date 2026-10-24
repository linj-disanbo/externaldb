//go:build integration

// 这些用例连线上 ES（见用例内的 host 常量），默认 go test 不跑；
// 需要时：go test -tags integration cmd/jrpc/

package main

import (
	"testing"

	"github.com/33cn/externaldb/lib/escli/querypara"
	"github.com/stretchr/testify/assert"
)

var testClient = &EVM{
	&DBRead{Host: "http://183.134.99.140:9200", Title: "bityuan", Symbol: "bty", Prefix: "v2db_23_", Version: 7},
}

func TestEVM_TokenListAgg(t1 *testing.T) {
	var ans interface{}
	err := testClient.TokenListAgg(&querypara.Query{
		Match: []*querypara.QMatch{
			{Key: "owner", Value: "1E5saiXVb9mW8wcWUUZjsHJPZs5GmdzuSY"},
		},
	}, &ans)
	assert.NoError(t1, err)
}

func TestEVM_ContractList(t1 *testing.T) {
	ctQy := &querypara.Query{Fetch: &querypara.QFetch{FetchSource: true, Keys: []string{"contract_address", "contract_type"}}}
	var res interface{}
	err := testClient.ContractList(ctQy, &res)
	assert.NoError(t1, err)
}
