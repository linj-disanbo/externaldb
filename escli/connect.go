package escli

import (
	"encoding/json"
	"net/http"
	"net/url"

	l "github.com/33cn/chain33/common/log/log15"

	"github.com/33cn/externaldb/db"
	"github.com/33cn/externaldb/escli/aggdecode"
	"github.com/33cn/externaldb/escli/querypara"
	"github.com/33cn/externaldb/escli/status"
	v7 "github.com/33cn/externaldb/escli/v7"
)

var log = l.New("module", "escli")

// BulkError / ItemFailure 由具体实现（escli/v7）产生，这里转出类型别名，
// 方便调用方断言错误类型、定位失败记录。
type (
	BulkError   = v7.BulkError
	ItemFailure = v7.ItemFailure
)

// FilterFailed 返回 rs 中 _bulk 失败的记录，供"只重试失败项"使用：
// 失败记录的重写是幂等的（值是绝对值），而整批重跑转换器不是
// （db/address 的 TxCount、block_stat 是读改写累计，会重复计数）。
// 返回 nil 表示无法定位（调用方按整批失败处理）。
func FilterFailed(cli ESClient, rs []db.Record, bulkErr error) []db.Record {
	if bulkErr == nil {
		return nil
	}
	type failFilter interface {
		FilterFailed([]db.Record, error) []db.Record
	}
	if f, ok := cli.(failFilter); ok {
		return f.FilterFailed(rs, bulkErr)
	}
	return nil
}

type ESClient interface {
	Update(index, typ, id string, value string) error
	Set(index, typ, id string, value db.Record) error
	Get(index, typ, id string) (*json.RawMessage, error)
	List(index, typ string, kv []*db.ListKV) ([]*json.RawMessage, error)
	BulkUpdate(rs []db.Record) error

	IndexExists(index string) (bool, error)
	CreateIndex(index string, typ, mapping string) (bool, error)
	DeleteIndex(index string) (bool, error)

	MGet(idx, typ string, ids []string, decode func(x *json.RawMessage) (interface{}, error)) ([]interface{}, error)
	Search(idx, typ string, query *querypara.Query, decode func(x *json.RawMessage) (interface{}, error)) ([]interface{}, error)
	Count(idx, typ string, query *querypara.Query) (int64, error)
	Agg(idx, typ string, query *querypara.Search) (*aggdecode.AggregationBucketKeyItem, error)

	Exists(name string) (bool, error)
	Create(name string, table, definition string) (bool, error)
	Delete(name string) (bool, error)

	Status() *status.Status
	GetVersion() int32
	PerformRequest(method, path string, params url.Values, body interface{}, headers http.Header) (interface{}, error)
	DeleteByQuery(idx, typ string, query *querypara.Query) error
}

// NewESLongConnect create db handler
// 只支持 ES7（ES6 支持已移除）；version 参数保留仅为兼容现有调用点，请统一传 7。
func NewESLongConnect(host string, prefix string, version int32, username, password string) (ESClient, error) {
	if version != 0 && version != 7 {
		log.Warn("NewESLongConnect: only es7 supported, ignore esVersion", "version", version)
	}
	return v7.NewESLongConnect(host, prefix, 7, username, password)
}

// NewESShortConnect create db handler
// 只支持 ES7（ES6 支持已移除）；version 参数保留仅为兼容现有调用点，请统一传 7。
func NewESShortConnect(host string, prefix string, version int32, username, password string) (ESClient, error) {
	if version != 0 && version != 7 {
		log.Warn("NewESShortConnect: only es7 supported, ignore esVersion", "version", version)
	}
	return v7.NewESShortConnect(host, prefix, 7, username, password)
}
