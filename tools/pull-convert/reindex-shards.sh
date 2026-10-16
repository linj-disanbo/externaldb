#!/bin/bash
# reindex-shards.sh —— 把已有索引迁到多分片（number_of_shards 创建后不可改，只能 reindex + 别名切换）
#
# 用法：
#   ./reindex-shards.sh -n 3 account transaction          # 把 db01_account / db01_transaction 迁到 3 分片
#   ./reindex-shards.sh -n 3 -r 0 account
# 参数：-n 目标分片数（必填）；-r 目标副本数（默认取 .env 的 ES_REPLICAS，缺省 0）
#
# 重要：**先停 pull_convert**（reindex 期间的新写入不会进新索引）：
#   docker-compose -p ${PROJECT_NAME} stop pull_convert
#   迁完再 `make run`（游标在 ${CONVERT_PREFIX}seq_marker 里，重启会接着跑，不丢数据）
#
# 做法（每个索引）：取源码 mapping → 建 <idx>_new（目标分片/副本 + 同 mapping）
#   → _reindex（异步，轮询 _tasks）→ 校验文档数一致 → 删旧索引 → 建同名别名指向 _new
#   （代码/查询都用 <物理名>=prefix+逻辑名，别名对读写透明）
# 依赖：curl；mapping 转换需要 python3 或 jq 之一。
set -euo pipefail

. .env

ES="http://localhost:${ES_PORT_HTTP}"
AUTH="elastic:${ES_PASSWORD}"
PREFIX="${CONVERT_PREFIX}"

SHARDS=""
REPLICAS="${ES_REPLICAS:-0}"
while getopts "n:r:h" opt; do
  case $opt in
    n) SHARDS=$OPTARG ;;
    r) REPLICAS=$OPTARG ;;
    *) echo "usage: $0 -n <shards> [-r <replicas>] idx1 [idx2 ...]"; exit 2 ;;
  esac
done
shift $((OPTIND - 1))
[ -n "$SHARDS" ] || { echo "-n <shards> 必填"; exit 2; }
[ $# -gt 0 ] || { echo "至少给一个索引名（不含前缀，如 account transaction）"; exit 2; }

have() { command -v "$1" >/dev/null 2>&1; }
have curl || { echo "需要 curl"; exit 2; }
if ! have python3 && ! have jq; then echo "需要 python3 或 jq（用于转换 mapping）"; exit 2; fi

# 构造建索引 body：目标分片/副本 + 原索引的 mappings（保持字段类型不变）
build_body() { # $1=逻辑索引物理名 $2=shards $3=replicas
  local idx=$1 shards=$2 replicas=$3
  curl -s "$ES/$idx/_mapping" -u "$AUTH" | {
    if have python3; then
      python3 -c "
import json, sys
d = json.load(sys.stdin)
m = list(d.values())[0].get('mappings', {})
print(json.dumps({'settings': {'number_of_shards': $shards, 'number_of_replicas': $replicas}, 'mappings': m}))
"
    else
      jq -c "{settings:{number_of_shards:$shards,number_of_replicas:$replicas},mappings:(.[]|.mappings)}"
    fi
  }
}

echo "== 检查 pull_convert 是否已停 =="
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q -- "${PROJECT_NAME}-pull-convert"; then
  echo "!! ${PROJECT_NAME}-pull-convert 还在运行：请先 docker-compose -p ${PROJECT_NAME} stop pull_convert"
  exit 1
fi

FAILED=0
for name in "$@"; do
  idx="${PREFIX}${name}"
  echo "==== ${idx} -> ${SHARDS} shards / ${REPLICAS} replicas ===="
  info=$(curl -s -o /dev/null -w '%{http_code}' "$ES/$idx" -u "$AUTH")
  if [ "$info" = "404" ]; then echo "!! $idx 不存在，跳过"; continue; fi

  cur=$(curl -s "$ES/$idx/_settings?flat_settings=true" -u "$AUTH" | tr ',' '\n' | grep -m1 'number_of_shards' | tr -dc '0-9')
  echo "当前分片: ${cur:-未知}"
  if [ "${cur:-0}" = "$SHARDS" ]; then echo "已是指定分片数，跳过"; continue; fi

  new="${idx}_new"
  curl -s -o /dev/null -X DELETE "$ES/$new" -u "$AUTH" || true

  body=$(build_body "$idx" "$SHARDS" "$REPLICAS")
  if ! curl -s -X PUT "$ES/$new" -u "$AUTH" -H 'Content-Type: application/json' -d "$body" | grep -q '"acknowledged":true'; then
    echo "!! 建 $new 失败：$body"; FAILED=1; continue
  fi

  echo "开始 reindex（异步）..."
  task=$(curl -s -X POST "$ES/_reindex?wait_for_completion=false" -u "$AUTH" -H 'Content-Type: application/json' \
    -d "{\"source\":{\"index\":\"$idx\"},\"dest\":{\"index\":\"$new\"}}" | sed -n 's/.*"task":"\([^"]*\)".*/\1/p')
  [ -n "$task" ] || { echo "!! reindex 提交失败"; FAILED=1; continue; }
  while :; do
    st=$(curl -s "$ES/_tasks/$task" -u "$AUTH")
    echo "$st" | grep -q '"completed":true' && break
    printf '.'
    sleep 5
  done
  echo

  srcN=$(curl -s "$ES/$idx/_count" -u "$AUTH" | sed -n 's/.*"count":\([0-9]*\).*/\1/p')
  dstN=$(curl -s "$ES/$new/_count" -u "$AUTH" | sed -n 's/.*"count":\([0-9]*\).*/\1/p')
  echo "文档数: 旧=$srcN 新=$dstN"
  if [ "$srcN" != "$dstN" ]; then
    echo "!! 文档数不一致，保留两者不动，人工核对后再切（旧 $idx / 新 $new）"; FAILED=1; continue
  fi

  curl -s -o /dev/null -X DELETE "$ES/$idx" -u "$AUTH"
  curl -s -X POST "$ES/_aliases" -u "$AUTH" -H 'Content-Type: application/json' \
    -d "{\"actions\":[{\"add\":{\"index\":\"$new\",\"alias\":\"$idx\"}}]}" | grep -q '"acknowledged":true' \
    && echo "完成：$idx 现在是别名 -> $new（$SHARDS 分片）" || { echo "!! 建别名失败"; FAILED=1; }
done

echo "== 结果 =="
curl -s "$ES/_cat/indices/${PREFIX}*?v" -u "$AUTH" | grep -E "^health|^green|^yellow|^red" || true
curl -s "$ES/_cat/aliases/${PREFIX}*?v" -u "$AUTH" || true
echo "迁移完成后记得 make run 把 pull_convert 起回来（游标在 seq_marker，重启接着跑）"
exit $FAILED
