package v7

import (
	"context"
	"encoding/json"
	"fmt"
	logx "log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/33cn/chain33/common/log/log15"
	"github.com/33cn/chain33/types"
	elasticV7 "github.com/olivere/elastic/v7"
	"github.com/pkg/errors"

	"github.com/33cn/externaldb/lib/escli/querypara"
	"github.com/33cn/externaldb/lib/escli/status"
	"github.com/33cn/externaldb/lib/schema"
)

var (
	log = log15.New("module", "escliV7")
)

// ESClient db handler
type ESClientV7 struct {
	Host     string
	Prefix   string
	Version  int32
	Username string
	Password string
	Client   *elasticV7.Client
}

func NewESLongConnect(host string, prefix string, version int32, username, password string) (*ESClientV7, error) {
	cli := &ESClientV7{Host: host, Prefix: prefix, Version: version, Username: username, Password: password}
	err := cli.Connect()
	if err != nil {
		return nil, err
	}
	return cli, nil
}

func NewESShortConnect(host string, prefix string, version int32, username, password string) (*ESClientV7, error) {
	var err error
	cli := &ESClientV7{Host: host, Prefix: prefix, Version: version}
	errorLog := logx.New(os.Stdout, "APP", logx.LstdFlags)

	cli.Client, err = elasticV7.NewSimpleClient(elasticV7.SetErrorLog(errorLog), elasticV7.SetURL(cli.Host), elasticV7.SetSniff(false), elasticV7.SetBasicAuth(username, password))
	if err != nil {
		return nil, err
	}
	return cli, nil
}

func (cli *ESClientV7) index(idx string) string {
	return cli.Prefix + idx
}

func (cli *ESClientV7) Connect() error {
	errorLog := logx.New(os.Stdout, "APP", logx.LstdFlags)
	var err error
	cli.Client, err = elasticV7.NewClient(elasticV7.SetErrorLog(errorLog), elasticV7.SetURL(cli.Host), elasticV7.SetSniff(false), elasticV7.SetBasicAuth(cli.Username, cli.Password))
	if err != nil {
		return err
	}
	info, code, err := cli.Client.Ping(cli.Host).Do(context.Background())
	if err != nil {
		return err
	}
	log.Info("Elasticsearch connect", "code", code, "version", info.Version.Number)

	version, err := cli.Client.ElasticsearchVersion(cli.Host)
	if err != nil {
		return err
	}
	log.Info("Elasticsearch get version", "version", version)
	return nil
}

// Update db operator
func (cli *ESClientV7) Update(index, _, id string, value string) error {
	_, err := cli.Client.Index().Index(cli.index(index)).Id(id).BodyJson(value).Do(context.Background())
	if err != nil {
		log.Error("escli update failed", "index", index, "typ", "_doc", "id", id, "value", value)
		return err
	}
	return nil
}

// Set db operator:
func (cli *ESClientV7) Set(index, _, id string, value schema.Record) error {
	_, err := cli.Client.Index().Index(cli.index(index)).Id(id).BodyJson(string(value.Value())).Do(context.Background())
	if err != nil {
		log.Error("escli set failed", "index", index, "typ", "_doc", "id", id, "value", value)
		return err
	}
	return nil
}

// Get RawMessage
func (cli *ESClientV7) Get(index, _, id string) (*json.RawMessage, error) {
	res, err := cli.Client.Get().Index(cli.index(index)).Id(id).Do(context.Background())
	if err != nil {
		if elasticV7.IsNotFound(err) {
			return nil, schema.ErrDBNotFound
		}
		log.Error("escli get failed", "index", index, "typ", "_doc", "id", id, "err", err)
		return nil, err
	}
	//改变 res.Source 源代码是为了获取到res.Source(之前是一个指针类型*json.RawMessage，现在是[]byte)
	return &res.Source, nil
}

// List RawMessage
func (cli *ESClientV7) List(index, _ string, kv []*schema.ListKV) ([]*json.RawMessage, error) {
	if len(kv) == 0 {
		return nil, schema.ErrDBBadParam
	}
	query := querypara.Query{
		Match: make([]*querypara.QMatch, 0),
	}
	cnt := len(kv)
	for i := 0; i < cnt; i++ {
		query.Match = append(query.Match, &querypara.QMatch{
			Key:   kv[i].Key,
			Value: kv[i].Value,
		})
	}
	q := elasticV7.NewBoolQuery()
	q = addMatch(&query, q)
	//修改
	search := cli.Client.Search(cli.index(index))
	search.Query(q)
	responses, err := search.Do(context.Background())
	if err != nil {
		if elasticV7.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "Search failed")
	}
	rs := make([]*json.RawMessage, 0)
	if responses != nil && responses.Hits != nil && responses.Hits.Hits != nil {
		for _, hit := range responses.Hits.Hits {
			//修改string(*hit.Source)
			log.Debug("Search", "r", string(hit.Source))
			//修改hit.Source
			rs = append(rs, &hit.Source)
		}
	}

	return rs, err
}

// BulkUpdate 一个区块对应的记录一起更新
func (cli *ESClientV7) BulkUpdate(rs []schema.Record) error {
	return cli.bulkUpdate(rs, "")
}

// BulkUpdateRefresh 同 BulkUpdate，但带 refresh 策略（如 "wait_for"）。
// seq_marker 用它写入：返回时 marker 已可被 search 到，游标（max(seq)）就不会读到旧值。
func (cli *ESClientV7) BulkUpdateRefresh(rs []schema.Record, refresh string) error {
	return cli.bulkUpdate(rs, refresh)
}

func (cli *ESClientV7) bulkUpdate(rs []schema.Record, refresh string) error {
	beg := types.Now()
	defer func() {
		log15.Info("BulkUpdate", "cost", types.Since(beg), "refresh", refresh)
	}()
	b := cli.Client.Bulk()
	if refresh != "" {
		b = b.Refresh(refresh)
	}
	for _, r := range rs {
		if r.OpType() == schema.OpAdd {
			req := elasticV7.NewBulkIndexRequest().Index(cli.index(r.Index())).Id(r.ID()).Doc(json.RawMessage(r.Value()))
			b.Add(req)
		} else if r.OpType() == schema.OpDel {
			req := elasticV7.NewBulkDeleteRequest().Index(cli.index(r.Index())).Id(r.ID())
			b.Add(req)
		} else if r.OpType() == schema.OpUpdate {
			req := elasticV7.NewBulkUpdateRequest().Index(cli.index(r.Index())).Id(r.ID()).Doc(json.RawMessage(r.Value()))
			b.Add(req)
		}
	}
	responses, err := b.Do(context.Background())
	if err != nil {
		log.Error("Bulk Save failed", "err", err)
		return err
	}

	fails := collectBulkFailures(responses)
	for _, f := range fails {
		log.Error("Bulk Save Part failed", "index", f.Index, "id", f.ID,
			"status", f.Status, "err", f.Reason)
	}
	return bulkFailureError(responses, fails)
}

// bulkFailurePreview 错误信息里最多列出的失败明细条数
const bulkFailurePreview = 5

// ItemFailure 一条失败的 _bulk 子请求：Index 是**物理索引**（已带前缀）+ doc id，
// 可据此从原始 records 里定位失败项。
type ItemFailure struct {
	Index  string
	ID     string
	Status int
	Reason string
}

// BulkError 携带 _bulk 的 per-item 失败明细
type BulkError struct {
	Total    int
	Failures []ItemFailure
}

func (e *BulkError) Error() string {
	if e == nil || len(e.Failures) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "bulk update: %d/%d item(s) failed", len(e.Failures), e.Total)
	for i, f := range e.Failures {
		if i >= bulkFailurePreview {
			fmt.Fprintf(&sb, "; ...and %d more", len(e.Failures)-bulkFailurePreview)
			break
		}
		fmt.Fprintf(&sb, "; [%s/%s] status=%d %s", f.Index, f.ID, f.Status, f.Reason)
	}
	return sb.String()
}

// isDeleteNotFound 删除一个不存在的文档：ES 返回 status=404、result=not_found，
// 且**不带 error 字段**（update 缺失文档是 document_missing_exception，带 error）。
// 语义上等价于"已经删掉了"，不算失败：回滚重放的 del seq、同一 _bulk 里同一 id
// 被删两次都会命中，硬当失败会让进度永远卡在 404 上。
func isDeleteNotFound(action string, v *elasticV7.BulkResponseItem) bool {
	if action != "delete" || v == nil || v.Error != nil || v.Status != 404 {
		return false
	}
	// not_found 是 delete 缺失文档的标准 result；个别版本/场景可能不带 result，按 404 兜底
	return v.Result == "" || v.Result == "not_found"
}

// collectBulkFailures 收集 _bulk 响应里的 per-item 失败（status>=400）。
// 注意：_bulk 的 HTTP 状态码是 200，即使部分 item 失败也一样，必须逐条检查。
// 例外：delete 不存在的文档（404/not_found、无 error）按成功处理，见 isDeleteNotFound。
func collectBulkFailures(responses *elasticV7.BulkResponse) []ItemFailure {
	if responses == nil {
		return nil
	}
	var fails []ItemFailure
	for _, item := range responses.Items {
		for action, v := range item {
			if v == nil || v.Status < 400 {
				continue
			}
			if isDeleteNotFound(action, v) {
				continue
			}
			reason := ""
			if v.Error != nil {
				reason = v.Error.Reason
				if v.Error.Type != "" {
					reason = v.Error.Type + ": " + reason
				}
			}
			fails = append(fails, ItemFailure{
				Index:  v.Index,
				ID:     v.Id,
				Status: v.Status,
				Reason: reason,
			})
		}
	}
	return fails
}

// bulkFailureError 把 per-item 失败转成 *BulkError；无失败返回 nil。
// 上层（进度跟随批次）必须拿到错误后不推进进度。
func bulkFailureError(responses *elasticV7.BulkResponse, fails []ItemFailure) error {
	if len(fails) == 0 {
		return nil
	}
	total := 0
	if responses != nil {
		total = len(responses.Items)
	}
	return &BulkError{Total: total, Failures: fails}
}

// FilterFailed 从原记录里挑出 bulkErr 中失败的记录（按物理索引 + doc id 匹配）。
// 只重写失败项是幂等的（Record.Value 是绝对值）；整批重跑转换器不是
// （db/address 的 TxCount、block_stat 是读改写累计，会重复计数）。
func (cli *ESClientV7) FilterFailed(rs []schema.Record, bulkErr error) []schema.Record {
	be, ok := bulkErr.(*BulkError)
	if !ok || be == nil || len(be.Failures) == 0 {
		return nil
	}
	failed := make(map[string]struct{}, len(be.Failures))
	for _, f := range be.Failures {
		failed[f.Index+"/"+f.ID] = struct{}{}
	}
	var out []schema.Record
	for _, r := range rs {
		if _, hit := failed[cli.index(r.Index())+"/"+r.ID()]; hit {
			out = append(out, r)
		}
	}
	return out
}

// IndexExists check index exists
func (cli *ESClientV7) IndexExists(index string) (bool, error) {
	return cli.Client.IndexExists(cli.index(index)).Do(context.Background())
}

// CreateIndex create index
func (cli *ESClientV7) CreateIndex(index string, _, mapping string) (bool, error) {
	//可以在创建索引的时候址接将索引的分片书数和副本创建好
	ret3, err := cli.Client.CreateIndex(cli.index(index)).BodyString(mapping).Do(context.Background())
	if err != nil {
		log15.Error("CreateIndex Mappings", "err", err, "ret", ret3)
		return false, err
	}
	//ret, err := cli.client.CreateIndex(cli.index(index)).Do(context.Background())
	//if err != nil {
	//	log15.Error("CreateIndex", "err", err, "ret", ret)
	//	return false, err
	//}
	////不支持Type,将type去掉,使用默认分片
	//ret2, err := cli.client.PutMapping().Index(cli.index(index)).BodyString(mapping).Do(context.Background())
	//if err != nil {
	//	cli.client.DeleteIndex(cli.index(index)).Do(context.Background())
	//	log15.Error("CreateIndex PutMapping", "err", err, "ret", ret2)
	//	return false, err
	//}
	return true, nil
}

// DeleteIndex  DeleteIndex
func (cli *ESClientV7) DeleteIndex(index string) (bool, error) {
	ret, err := cli.Client.DeleteIndex(cli.index(index)).Do(context.Background())
	if err != nil {
		log15.Info("IndexDelete", "err", err, "ret", ret)
		return false, err
	}
	return true, nil
}

// Exists adapter DBCreator
func (cli *ESClientV7) Exists(name string) (bool, error) {
	return cli.IndexExists(name)
}

// Create adapter DBCreator
func (cli *ESClientV7) Create(name string, table, definition string) (bool, error) {
	return cli.CreateIndex(name, table, definition)
}

// Delete adapter DBCreator
func (cli *ESClientV7) Delete(name string) (bool, error) {
	return cli.DeleteIndex(name)
}

// DeleteByQuery deletes documents that match a query.
func (cli *ESClientV7) DeleteByQuery(idx, _ string, query *querypara.Query) error {
	service := elasticV7.NewDeleteByQueryService(cli.Client).Index(cli.index(idx))
	resp, err := service.Query(getQuery(query)).Do(context.Background())
	if err != nil {
		log.Error("DeleteByQuery", "err", err, "resp", resp)
		return err
	}
	return err
}

// RefreshIndex 显式 refresh 一个索引（POST <prefix><index>/_refresh）。
// 用途：marker/游标这类"写完要能被 search 立刻读到"的场景，用一次显式 refresh 取代
// 每次写入的 refresh=wait_for（后者要等一个 refresh 周期，实测 ~1s/次，会把逐块循环拖死）。
func (cli *ESClientV7) RefreshIndex(index string) error {
	if index == "" {
		return errors.New("RefreshIndex: empty index")
	}
	_, err := cli.Client.PerformRequest(context.Background(), elasticV7.PerformRequestOptions{
		Method: "POST",
		Path:   "/" + cli.index(index) + "/_refresh", // PerformRequest 的 path 必须以 / 开头
	})
	if err != nil {
		log.Error("RefreshIndex failed", "index", index, "err", err)
	}
	return err
}

// Status 获取ES状态信息
func (cli *ESClientV7) Status() (res *status.Status) {
	res = &status.Status{Status: "UP"}
	resp, err := cli.Client.PerformRequest(context.Background(), elasticV7.PerformRequestOptions{
		Method: "GET",
		Path:   "_nodes/stats",
	})
	if err != nil {
		log15.Info("Status, PerformRequest _nodes/stats", "err", err)
		res.Status = err.Error()
		return
	}
	if err := json.Unmarshal(resp.Body, &res); err != nil {
		log15.Info("Status, json.Unmarshal", "err", err)
		res.Status = err.Error()
		return
	}
	return
}

func (cli *ESClientV7) GetVersion() int32 {
	return cli.Version
}

func (cli *ESClientV7) PerformRequest(method, path string, params url.Values, body interface{}, headers http.Header) (interface{}, error) {
	return cli.Client.PerformRequest(context.Background(), elasticV7.PerformRequestOptions{
		Method:  method,
		Path:    path,
		Params:  params,
		Body:    body,
		Headers: headers,
	})
}
