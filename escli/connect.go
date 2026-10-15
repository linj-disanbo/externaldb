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
