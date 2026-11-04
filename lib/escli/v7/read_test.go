package v7

import (
	"net/http"
	"net/http/httptest"
	"testing"

	elasticV7 "github.com/olivere/elastic/v7"

	"github.com/33cn/externaldb/lib/escli/querypara"
)

// 回归：索引不存在时 ES 返回 404，Agg 必须返回"空结果"而不是 nil。
// 历史问题：返回 (nil, nil) 让调用方（account_refresh 首轮、jrpc Account.TopAsset、
// rpc proof 统计）在 "res.Term(...)" 上 nil 解引用 panic —— 新环境索引还没建时必现。
func TestAggIndexNotFoundReturnsEmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// olivere v7 建 client 时会 ping 根路径，先让它起来
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`{"name":"mock","cluster_name":"mock","version":{"number":"7.12.0"},` +
				`"tagline":"You Know, for Search"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"root_cause":[{"type":"index_not_found_exception",` +
			`"reason":"no such index [db05_account]","index":"db05_account"}],` +
			`"type":"index_not_found_exception","reason":"no such index [db05_account]"},"status":404}`))
	}))
	defer srv.Close()

	client, err := elasticV7.NewClient(
		elasticV7.SetURL(srv.URL),
		elasticV7.SetHttpClient(srv.Client()),
		elasticV7.SetHealthcheck(false),
		elasticV7.SetSniff(false),
	)
	if err != nil {
		t.Fatalf("new es client: %v", err)
	}
	cli := &ESClientV7{Prefix: "db05_", Version: 7, Client: client}

	s := &querypara.Search{Agg: &querypara.Agg{
		Name: "total",
		Size: &querypara.ASize{Size: 10},
		Term: &querypara.AAgg{Key: "address"},
		Subs: &querypara.ASub{Sum: []*querypara.AAgg{{Name: "total", Key: "total"}}},
	}}

	res, err := cli.Agg("account", "account", s)
	if err != nil {
		t.Fatalf("索引不存在不应返回错误（调用方按空结果处理）: %v", err)
	}
	if res == nil {
		t.Fatal("索引不存在时 Agg 不能返回 nil（会导致调用方 nil 解引用 panic）")
	}
	// 取值接收者方法：能调用即说明不是 nil，且应走 found=false 分支
	if buckets, found := res.Term("total"); found || len(buckets) != 0 {
		t.Errorf("空结果不应有 term bucket: found=%v len=%d", found, len(buckets))
	}
	if v, found := res.Sum("total"); found || v != -1 {
		t.Errorf("空结果的 Sum 应为 (-1,false), got (%v,%v)", v, found)
	}
}
