package v7

import (
	"context"
	"encoding/json"

	elasticV7 "github.com/olivere/elastic/v7"
	"github.com/pkg/errors"

	"github.com/33cn/externaldb/lib/escli/aggdecode"
	"github.com/33cn/externaldb/lib/escli/querypara"
)

// Decode source
type Decode func(x *json.RawMessage) (interface{}, error)

// MGet MGet
func (cli *ESClientV7) MGet(idx, typ string, ids []string, decode func(x *json.RawMessage) (interface{}, error)) ([]interface{}, error) {
	reqs := elasticV7.NewMgetService(cli.Client)
	for _, id := range ids {
		reqs = reqs.Add(elasticV7.NewMultiGetItem().Index(cli.index(idx)).Id(id).FetchSource(elasticV7.NewFetchSourceContext(true)))
	}
	responses, err := reqs.Do(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "Mget failed")
	}
	rs := make([]interface{}, 0)
	if responses != nil {
		for _, response := range responses.Docs {
			log.Debug("MGet", "r", response)
			if response.Found {
				//修改 decode(response.Source)
				r, err := decode(&response.Source)
				if err != nil {
					log.Error("decode get response", "err", err, "id", response.Id)
					rs = append(rs, nil)
					continue
				}
				rs = append(rs, r)
			} else {
				rs = append(rs, nil)
			}
		}
	}

	return rs, nil
}

// Search Search
func (cli *ESClientV7) Search(idx, typ string, query *querypara.Query, decode func(x *json.RawMessage) (interface{}, error)) ([]interface{}, error) {
	var err error

	search := cli.Client.Search(cli.index(idx))

	if query != nil {
		search, err = Query(search, query)
		if err != nil {
			return nil, errors.Wrap(err, "Query failed")
		}
	}

	responses, err := search.Do(context.Background())
	if err != nil {
		if elasticV7.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "Search failed")
	}
	rs := make([]interface{}, 0)
	if responses != nil && responses.Hits != nil && responses.Hits.Hits != nil {
		for _, hit := range responses.Hits.Hits {
			//log.Debug("Search", "r", string(*hit.Source))
			//修改decode(hit.Source)
			r, err := decode(&hit.Source)
			if err != nil {
				log.Error("decode get hit", "err", err, "id", hit.Uid)
				continue
			}
			rs = append(rs, r)
		}
	}

	return rs, err
}

// Agg Agg
func (cli *ESClientV7) Agg(idx, typ string, query *querypara.Search) (*aggdecode.AggregationBucketKeyItem, error) {
	var err error

	search := cli.Client.Search(cli.index(idx)).Size(0)

	if query.Query != nil {
		search, err = Query(search, query.Query)
		if err != nil {
			return nil, errors.Wrap(err, "Query failed")
		}
	}

	if query.Agg != nil {
		search, err = Aggregation(search, query.Agg)
		if err != nil {
			return nil, errors.Wrap(err, "Agg failed")
		}
	}

	responses, err := search.Do(context.TODO())
	if err != nil {
		if elasticV7.IsNotFound(err) {
			// 索引不存在（新环境还没写入过该索引）：返回空结果而不是 nil。
			// 契约：Agg 永远返回非 nil，调用方统一走 Term/Sum/Range 的 found=false 分支；
			// 返回 nil 会让 "res.Term(...)" 这类取值接收者的调用直接 nil 解引用 panic
			// （account_refresh 首轮、jrpc Account.TopAsset 等）。
			return &aggdecode.AggregationBucketKeyItem{
				Aggregations: map[string]json.RawMessage{},
				Version:      cli.Version,
			}, nil
		}
		return nil, errors.Wrap(err, "Search failed")
	}

	if responses == nil {
		// 防御：正常不会出现，但不让调用方拿到 nil
		return &aggdecode.AggregationBucketKeyItem{
			Aggregations: map[string]json.RawMessage{},
			Version:      cli.Version,
		}, nil
	}

	b := &aggdecode.AggregationBucketKeyItem{
		Aggregations: responses.Aggregations,
		Version:      cli.Version,
	}

	return b, err
}

// Count Count
func (cli *ESClientV7) Count(idx, typ string, query *querypara.Query) (int64, error) {
	search := cli.Client.Count(cli.index(idx))
	q := getQuery(query)
	if q != nil {
		search.Query(q)
	}

	count, err := search.Do(context.Background())
	if err != nil {
		if elasticV7.IsNotFound(err) {
			return 0, nil
		}
		return 0, errors.Wrap(err, "Count failed")
	}

	return count, err
}
