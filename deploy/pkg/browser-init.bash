#!/usr/bin/env bash

. .env

curl -X PUT http://localhost:"$ES_PORT"/"$CONVERT_PREFIX"block_info/_settings -d '{ "index" : { "max_result_window" : 100000}}' -H "Content-Type: application/json" -u elastic:"$ES_PASSWORD"
echo ""
curl -X PUT http://localhost:"$ES_PORT"/"$CONVERT_PREFIX"transaction/_settings -d '{ "index" : { "max_result_window" : 100000}}' -H "Content-Type: application/json" -u elastic:"$ES_PASSWORD"
echo ""

# 浏览器从0开始同步数据时，为了提升写入性能，使用此配置
curl -XPUT http://localhost:"$ES_PORT"/"$CONVERT_PREFIX"*/_settings?preserve_existing=true -u elastic:"$ES_PASSWORD" -H "Content-Type: application/json"  -d '{
    "index.translog.durability" : "async",
    "index.translog.flush_threshold_size" : "512mb",
    "index.translog.sync_interval" : "3s"
}'

# 上面的通配会把 seq_marker（pull_convert 的进度游标）也设成 async —— 掉电时可能丢 marker，
# 重启后游标偏小会重放已处理的 seq，导致 address.tx_count / block_stat 这类读改写计数重复。
# 这里把游标索引单独覆盖回"每次请求 fsync"，并把 translog 阈值调小（小索引高频写，见任务 2）。
curl -s -o /dev/null -XPUT "http://localhost:$ES_PORT/${CONVERT_PREFIX}seq_marker/_settings" -u elastic:"$ES_PASSWORD" -H 'Content-Type: application/json' -d '{
    "index": {
        "translog.durability":"request",
        "translog.flush_threshold_size":"64mb",
        "translog.sync_interval":"5s"
   }
}' || true

curl -XPUT http://localhost:"$ES_PORT"/"$CONVERT_PREFIX"account,"$CONVERT_PREFIX"address/_settings -u elastic:"$ES_PASSWORD" -H 'Content-Type: application/json' -d '{
    "index": {
        "refresh_interval":"60s"
   }
}'

# account_refresh（BTY 余额定时校正）会写这两个字段。
# mapping 只在索引首次创建时由程序写入（util.InitIndex 仅在索引不存在时建），
# 已有 account 索引不会自动补字段；这里显式 PUT（additive，可重复执行）。
curl -s -o /dev/null -XPUT "http://localhost:$ES_PORT/${CONVERT_PREFIX}account/_mapping" -u elastic:"$ES_PASSWORD" -H 'Content-Type: application/json' -d '{
    "properties": {
        "balance_refresh_at": {"type": "long"},
        "balance_refresh_height": {"type": "long"}
    }
}' || echo "!! account mapping 追加失败（索引可能还不存在，动态 mapping 也能写入）"

curl -XPUT http://localhost:"$ES_PORT"/"$CONVERT_PREFIX"last_seq/_settings -u elastic:"$ES_PASSWORD" -H 'Content-Type: application/json' -d '{
    "index": {
        "refresh_interval":"1s"
   }
}'

# 浏览器数据同步到最新时，恢复默认配置，保证account数据的实时性
#curl -XPUT http://localhost:"$ES_PORT"/"$CONVERT_PREFIX"*/_settings?preserve_existing=true -u elastic:"$ES_PASSWORD" -H "Content-Type: application/json"  -d '{
#    "index.translog.durability" : "fsync",
#    "index.translog.flush_threshold_size" : "512mb",
#}'
#
#curl -XPUT http://localhost:"$ES_PORT"/"$CONVERT_PREFIX"account,"$CONVERT_PREFIX"address/_settings -u elastic:"$ES_PASSWORD" -H 'Content-Type: application/json' -d '{
#    "index": {
#        "refresh_interval":"1s"
#   }
#}'