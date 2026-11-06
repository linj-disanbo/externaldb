#!/usr/bin/env bash
# 部署后自检：容器与资源 / ES 索引与写入 / convert 进度 / scanner 进度 / MySQL 数据健康 /
# erc20api 接口 / jrpc 接口 / 日志错误。
#
# 用法（在包根目录或任意位置都行）：
#   bash check/check.sh              # 全部检查
#   bash check/check.sh progress     # 只查 convert + scanner 进度
#   bash check/check.sh erc20api     # 只查 erc20api
#   bash check/check.sh jrpc es docker
#
# 配置：默认读包根目录的 .env（与 quickStart/Makefile 同一份），可用 ENV_FILE 覆盖。
# 下面这些都可以用环境变量覆盖：
#   ES_URL / ES_USER / ES_PASSWORD / CONVERT_PREFIX
#   CHAIN_URL（节点 HTTP jsonrpc，用于算落后块数）
#   JRPC_URL / ERC20_API_URL
#   CHECK_MYSQL_MODE=auto|docker|cli|skip   CHECK_MYSQL_NAME / MYSQL_*（自动从 .env 取）
#   MAX_LAG_BLOCKS=100   PROGRESS_WAIT=8   CHECK_TIMEOUT=10
#   LOG_LINES=2000（每个日志文件看最后多少行） DISK_WARN=85 DISK_FAIL=95
#
# 退出码：0 = 没有 FAIL（WARN 不算失败）；1 = 有 FAIL；2 = 前置条件不满足（如 .env 缺失）
set -u

CHECK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$CHECK_DIR/.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"

if [ ! -f "$ENV_FILE" ]; then
  echo "FATAL: 找不到 .env：$ENV_FILE（用 ENV_FILE=... 指定）" >&2
  exit 2
fi

# shellcheck disable=SC1090
. "$ENV_FILE"


# ---------- 可从环境覆盖的配置 ----------
ES_URL="${ES_URL:-http://${ES_BIND:-127.0.0.1}:${ES_PORT_HTTP:-9200}}"
ES_USER="${ES_USER:-elastic}"
ES_PASSWORD="${ES_PASSWORD:-${ES_PASSWORD:-}}"
CONVERT_PREFIX="${CONVERT_PREFIX:-db01_}"
CHAIN_URL="${CHAIN_URL:-${CHAIN_HOST:-}}"
ERC20_API_URL="${ERC20_API_URL:-http://127.0.0.1:${ERC20_API_PORT:-8080}}"
# jrpc 绑定的是 0.0.0.0:9993 这类，自检按本机访问
JRPC_PORT="${JRPC_PORT:-${JRPC_HOST##*:}}"
JRPC_URL="${JRPC_URL:-http://127.0.0.1:${JRPC_PORT:-9993}}"
MAX_LAG_BLOCKS="${MAX_LAG_BLOCKS:-100}"
LOG_LINES="${LOG_LINES:-2000}"
DISK_WARN="${DISK_WARN:-85}"
DISK_FAIL="${DISK_FAIL:-95}"
PROGRESS_WAIT="${PROGRESS_WAIT:-8}"
CHECK_TIMEOUT="${CHECK_TIMEOUT:-10}"

PASS=0; WARN=0; FAIL=0; SKIP=0
CONVERT_HEIGHT=""   # check_convert 记录，供 jrpc 交叉校验
sec_fail() { FAIL=$((FAIL + 1)); printf '  [FAIL] %s\n' "$*"; }
sec_warn() { WARN=$((WARN + 1)); printf '  [WARN] %s\n' "$*"; }
sec_pass() { PASS=$((PASS + 1)); printf '  [PASS] %s\n' "$*"; }
sec_skip() { SKIP=$((SKIP + 1)); printf '  [SKIP] %s\n' "$*"; }

command -v python3 >/dev/null 2>&1 || { echo "FATAL: 需要 python3 解析 JSON" >&2; exit 2; }
command -v curl >/dev/null 2>&1 || { echo "FATAL: 需要 curl" >&2; exit 2; }

# JSON 取值助手：stdin 读 JSON，$1=get|len，$2=dotted.path
PY_HELPER="$(mktemp -t checkjson.XXXXXX)"
trap 'rm -f "$PY_HELPER"' EXIT
cat > "$PY_HELPER" <<'PYEOF'
import json, sys

mode, path = sys.argv[1], [p for p in sys.argv[2].split('.') if p != '']
raw = sys.stdin.read()
try:
    d = json.loads(raw)
    for p in path:
        d = d[int(p)] if isinstance(d, list) else d[p]
    if mode == 'len':
        print(len(d))
    elif isinstance(d, bool):
        print('true' if d else 'false')
    elif isinstance(d, (int, float, str)):
        print(d)
    elif d is None:
        print('')
    else:
        print(json.dumps(d, ensure_ascii=False))
except Exception:
    print(0 if mode == 'len' else '')
PYEOF

# jget <dotted.path>  从 stdin 的 JSON 取值（不存在/解析失败 → 空）
jget() { python3 "$PY_HELPER" get "$1"; }
# jlen <dotted.path>  取列表/字符串长度（失败 → 0）
jlen() { python3 "$PY_HELPER" len "$1"; }

es_get() { curl -s -m "$CHECK_TIMEOUT" -u "${ES_USER}:${ES_PASSWORD}" "$ES_URL$1"; }
api_get() { curl -s -m "$CHECK_TIMEOUT" "$ERC20_API_URL$1"; }
is_num() { case "${1:-}" in ''|*[!0-9-]*) return 1;; *) return 0;; esac; }

# ---------- 1. 容器 ----------
check_docker() {
  echo
  echo "[docker] 容器状态（项目名 ${PROJECT_NAME:-?}）"
  if ! command -v docker >/dev/null 2>&1; then
    sec_skip "没装 docker，跳过容器检查"
    return
  fi
  local svc cnt=0
  for svc in pull-convert jrpc scanner erc20api accountrefresh; do
    case "$svc" in
      scanner|erc20api) [ "${MYSQL_ENABLED:-false}" = "true" ] || continue ;;
      accountrefresh) [ "${ACCOUNT_REFRESH_ENABLED:-false}" = "true" ] || continue ;;
    esac
    cnt=$((cnt + 1))
    local name="${PROJECT_NAME:-ex}-${svc}"
    local status
    status="$(docker ps --filter "name=^/${name}$" --format '{{.Status}}' 2>/dev/null | head -1)"
    if [ -z "$status" ]; then
      sec_fail "${name} 未运行（docker-compose -p ${PROJECT_NAME} up -d ${svc}）"
    elif printf '%s' "$status" | grep -qi '^up'; then
      sec_pass "${name}  ${status}"
    else
      sec_fail "${name} 状态异常：${status}"
    fi
  done
  [ "$cnt" -gt 0 ] || sec_skip "按 .env 没有需要检查的常驻服务"
}


# ---------- 1b. 容器重启/退出与磁盘 ----------
check_runtime() {
  echo
  echo "[runtime] 容器重启/退出码与磁盘"
  if ! command -v docker >/dev/null 2>&1; then
    sec_skip "没装 docker，跳过"
    return
  fi
  local svc name info restarts status exitcode started
  for svc in pull-convert jrpc scanner erc20api accountrefresh; do
    case "$svc" in
      scanner|erc20api) [ "${MYSQL_ENABLED:-false}" = "true" ] || continue ;;
      accountrefresh) [ "${ACCOUNT_REFRESH_ENABLED:-false}" = "true" ] || continue ;;
    esac
    name="${PROJECT_NAME:-ex}-${svc}"
    info="$(docker inspect -f '{{.RestartCount}}|{{.State.Status}}|{{.State.ExitCode}}|{{.State.StartedAt}}' "$name" 2>/dev/null)"
    if [ -z "$info" ]; then
      sec_skip "${name} 不存在"
      continue
    fi
    IFS='|' read -r restarts status exitcode started <<<"$info"
    if [ "$status" != "running" ]; then
      sec_fail "${name} 状态 ${status}（exit=${exitcode}，重启 ${restarts} 次）"
      continue
    fi
    if [ "${restarts:-0}" -gt 5 ]; then
      sec_warn "${name} 已重启 ${restarts} 次（疑似 crash-loop，看日志）"
    else
      sec_pass "${name} 运行中（重启 ${restarts} 次，started ${started}）"
    fi
  done

  local dir used
  for dir in "${ES_DIR:-}" "${MYSQL_DIR:-}" "$ROOT_DIR"; do
    [ -n "$dir" ] && [ -d "$dir" ] || continue
    used="$(df -P "$dir" 2>/dev/null | awk 'NR==2 {gsub(/%/,"",$5); print $5}')"
    is_num "${used:-}" || continue
    if [ "$used" -ge "$DISK_FAIL" ]; then
      sec_fail "${dir} 磁盘使用 ${used}%（≥${DISK_FAIL}%）"
    elif [ "$used" -ge "$DISK_WARN" ]; then
      sec_warn "${dir} 磁盘使用 ${used}%（≥${DISK_WARN}%）"
    else
      sec_pass "${dir} 磁盘使用 ${used}%"
    fi
  done
}

# ---------- 2. ES 与索引 ----------
check_es() {
  echo
  echo "[es] 集群与索引（${ES_URL}，前缀 ${CONVERT_PREFIX}）"
  local health status
  health="$(es_get '/_cluster/health')"
  status="$(printf '%s' "$health" | jget status)"
  case "$status" in
    green) sec_pass "集群状态 green" ;;
    yellow) sec_warn "集群状态 yellow（副本未分配，单节点正常）" ;;
    red) sec_fail "集群状态 red（有主分片不可用）" ;;
    "") sec_fail "连不上 ES：${ES_URL}" ;;
    *) sec_warn "集群状态 ${status}" ;;
  esac

  local idx code
  for idx in "${CONVERT_PREFIX}seq_marker" "${CONVERT_PREFIX}last_seq" "${CONVERT_PREFIX}contract" \
             "${CONVERT_PREFIX}transaction" "${CONVERT_PREFIX}account" "${CONVERT_PREFIX}block_info"; do
    code="$(curl -s -m "$CHECK_TIMEOUT" -o /dev/null -w '%{http_code}' -u "${ES_USER}:${ES_PASSWORD}" \
      -I "$ES_URL/$idx")"
    case "$code" in
      200) sec_pass "索引 ${idx} 存在" ;;
      404) sec_fail "索引 ${idx} 不存在（convert 还没写过？）" ;;
      *) sec_warn "索引 ${idx} 探测返回 HTTP ${code}" ;;
    esac
  done

  local unassigned rejected heap
  unassigned="$(printf '%s' "$health" | jget unassigned_shards)"
  if [ -n "${unassigned:-}" ] && [ "${unassigned:-0}" -gt 0 ]; then
    sec_warn "有 ${unassigned} 个分片未分配（单节点副本未分配属正常）"
  fi

  rejected="$(es_get '/_nodes/stats/thread_pool?filter_path=nodes.*.thread_pool.write.rejected' |
    python3 -c 'import json,sys
try:
    d=json.load(sys.stdin)
    print(sum(int(n["thread_pool"]["write"]["rejected"]) for n in d.get("nodes",{}).values()))
except Exception:
    print(-1)')"
  if [ "${rejected:-0}" -gt 0 ]; then
    sec_warn "ES write 线程池累计拒绝 ${rejected} 次（曾触发 429/队列满，看 pull_convert 日志）"
  fi

  heap="$(es_get '/_nodes/stats/jvm?filter_path=nodes.*.jvm.mem.heap_used_percent' |
    python3 -c 'import json,sys
try:
    d=json.load(sys.stdin)
    print(max(int(n["jvm"]["mem"]["heap_used_percent"]) for n in d.get("nodes",{}).values()))
except Exception:
    print(-1)')"
  if [ "${heap:-0}" -ge 85 ] && [ "${heap:-0}" -le 100 ]; then
    sec_warn "ES 堆使用 ${heap}%（≥85% 容易出现 429 circuit_breaking）"
  elif [ "${heap:-0}" -ge 0 ]; then
    sec_pass "ES 堆使用 ${heap}%"
  fi

  local alloc
  alloc="$(es_get '/_cat/allocation?h=disk.percent&format=json' |
    python3 -c 'import json,sys
try:
    d=json.load(sys.stdin)
    print(max(int(x.get("disk.percent") or 0) for x in d))
except Exception:
    print(-1)')"
  if [ "${alloc:-0}" -ge "$DISK_FAIL" ]; then
    sec_fail "ES 数据盘使用 ${alloc}%（≥${DISK_FAIL}% 会触发只读/写拒绝）"
  elif [ "${alloc:-0}" -ge "$DISK_WARN" ]; then
    sec_warn "ES 数据盘使用 ${alloc}%（建议清理或扩容）"
  elif [ "${alloc:-0}" -ge 0 ]; then
    sec_pass "ES 数据盘使用 ${alloc}%"
  fi

  local docs
  docs="$(es_get "/${CONVERT_PREFIX}transaction/_count" | jget count)"
  if is_num "${docs:-}" && [ "${docs:-0}" -gt 0 ]; then
    sec_pass "${CONVERT_PREFIX}transaction 文档数 ${docs}"
  else
    sec_fail "${CONVERT_PREFIX}transaction 文档数为 0 或查询失败（${docs:-?}）"
  fi
}

# ---------- 3. convert（pull_convert）进度 ----------
es_seq_top() { # $1=索引  → 输出 "seq height"（取 seq 最大的那条）
  es_get "/$1/_search?size=1&sort=seq:desc" | python3 -c '
import json,sys
try:
    d=json.load(sys.stdin)
    h=d["hits"]["hits"][0]["_source"]
    print(int(h.get("seq",-1)), int(h.get("height",-1)))
except Exception:
    print("-1 -1")
'
}

es_last_seq() { # $1=索引 → 输出 sync_seq
  es_get "/$1/_search?size=1" | python3 -c '
import json,sys
try:
    d=json.load(sys.stdin)
    h=d["hits"]["hits"][0]["_source"]
    print(int(h.get("sync_seq",-1)))
except Exception:
    print(-1)
'
}

chain_height() {
  [ -n "${CHAIN_URL:-}" ] || { echo -1; return; }
  curl -s -m "$CHECK_TIMEOUT" -X POST "$CHAIN_URL" -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"Chain33.GetLastHeader","params":[]}' |
    python3 -c '
import json,sys
try:
    d=json.load(sys.stdin)
    r=d.get("result") or {}
    print(int(r.get("height",-1)))
except Exception:
    print(-1)
'
}

check_convert() {
  echo
  echo "[progress] convert（pull_convert）进度"
  local marker seq1 height1
  marker="${CONVERT_PREFIX}seq_marker"
  read -r seq1 height1 <<<"$(es_seq_top "$marker")"

  if [ "${seq1:-0}" -lt 0 ]; then
    sec_fail "读不到 ${marker}（ES 连不通或索引为空）"
    return
  fi
  CONVERT_HEIGHT="$height1"
  sec_pass "seq_marker: max seq=${seq1} height=${height1}（文档数 $(es_get "/${marker}/_count" | jget count)）"

  local lseq
  lseq="$(es_last_seq "${CONVERT_PREFIX}last_seq")"
  if [ "${lseq:-0}" -ge 0 ]; then
    sec_pass "last_seq 记录 convert 进度 ${lseq}"
  else
    sec_warn "读不到 ${CONVERT_PREFIX}last_seq（旧链路才有？pull 链路看 seq_marker 即可）"
  fi

  local seq2 height2 chain lag
  sleep "$PROGRESS_WAIT"
  read -r seq2 height2 <<<"$(es_seq_top "$marker")"
  if [ "${seq2:-0}" -gt "$seq1" ] || [ "${height2:-0}" -gt "$height1" ]; then
    sec_pass "${PROGRESS_WAIT}s 内推进：seq ${seq1}→${seq2}，height ${height1}→${height2}"
  else
    sec_warn "${PROGRESS_WAIT}s 内没有推进（链可能没新块；或 pull_convert 卡住）"
  fi

  chain="$(chain_height)"
  if is_num "$chain" && [ "$chain" -ge 0 ] && is_num "$height2" && [ "$height2" -ge 0 ]; then
    lag=$((chain - height2))
    if [ "$lag" -lt 0 ]; then
      sec_warn "convert 进度(${height2}) 比链高(${chain}) 还大，检查 CHAIN_HOST 是否指向同一条链"
    elif [ "$lag" -le "$MAX_LAG_BLOCKS" ]; then
      sec_pass "落后链尖 ${lag} 块（链高 ${chain}，阈值 ${MAX_LAG_BLOCKS}）"
    elif [ "$lag" -le $((MAX_LAG_BLOCKS * 10)) ]; then
      sec_warn "落后链尖 ${lag} 块（链高 ${chain}，阈值 ${MAX_LAG_BLOCKS}；同步中属正常）"
    else
      sec_fail "落后链尖 ${lag} 块（链高 ${chain}）：convert 可能卡住或节点不对"
    fi
  else
    sec_skip "拿不到链高度（CHAIN_URL=${CHAIN_URL:-空}），跳过落后判定"
  fi
}

# ---------- 4. scanner 进度（MySQL） ----------
MYSQL_MODE_RESOLVED=""
mysql_resolve() {
  [ -n "$MYSQL_MODE_RESOLVED" ] && return
  local mode="${CHECK_MYSQL_MODE:-auto}"
  if [ "${MYSQL_ENABLED:-false}" != "true" ]; then
    MYSQL_MODE_RESOLVED="skip"
    return
  fi
  if [ "$mode" = "skip" ]; then MYSQL_MODE_RESOLVED="skip"; return; fi
  if [ "$mode" = "docker" ] || { [ "$mode" = "auto" ] && command -v docker >/dev/null 2>&1 &&
        [ -n "$(docker ps --filter "name=^/${MYSQL_NAME:-externaldb-mysql}$" --format '{{.Names}}' 2>/dev/null)" ]; }; then
    MYSQL_MODE_RESOLVED="docker"
    return
  fi
  if command -v mysql >/dev/null 2>&1; then MYSQL_MODE_RESOLVED="cli"; return; fi
  MYSQL_MODE_RESOLVED="skip"
}

mysql_query() { # $1=SQL（-N -B 输出）
  case "$MYSQL_MODE_RESOLVED" in
    docker) docker exec -i "${MYSQL_NAME:-externaldb-mysql}" \
              mysql -uroot -p"${MYSQL_ROOT_PASSWORD:-}" "${MYSQL_DATABASE:-token_scanner}" -N -B -e "$1" 2>/dev/null ;;
    cli)
      local host port user pwd db rest
      if [ -n "${MYSQL_DSN:-}" ]; then
        rest="${MYSQL_DSN#*@tcp(}"
        hostport="${rest%%)*}"
        host="${hostport%%:*}"; port="${hostport##*:}"
        user="${MYSQL_DSN%%:*}"; pwd="${MYSQL_DSN#*:}"; pwd="${pwd%%@*}"
        db="${rest#*)/}"; db="${db%%\?*}"
        mysql -h"$host" -P"${port:-3306}" -u"$user" -p"$pwd" "$db" -N -B -e "$1" 2>/dev/null
      else
        mysql -h"${MYSQL_BIND:-127.0.0.1}" -P"${MYSQL_PORT:-3306}" -uroot \
          -p"${MYSQL_ROOT_PASSWORD:-}" "${MYSQL_DATABASE:-token_scanner}" -N -B -e "$1" 2>/dev/null
      fi ;;
    *) return 1 ;;
  esac
}

check_scanner() {
  echo
  echo "[progress] scanner（ERC20 扫块）进度"
  mysql_resolve
  if [ "$MYSQL_MODE_RESOLVED" = "skip" ]; then
    sec_skip "MYSQL_ENABLED!=true 或无 MySQL 访问方式（CHECK_MYSQL_MODE=auto）"
    return
  fi

  local row h1 tx1 t1
  row="$(mysql_query "SELECT last_block_number, processed_tx_count, COALESCE(updated_at,'-') FROM scan_progress WHERE id=1")"
  if [ -z "$row" ]; then
    sec_fail "查不到 scan_progress（MySQL 连不上或表不存在）"
    return
  fi
  h1="$(printf '%s' "$row" | awk -F'\t' '{print $1}')"
  tx1="$(printf '%s' "$row" | awk -F'\t' '{print $2}')"
  t1="$(printf '%s' "$row" | awk -F'\t' '{print $3}')"
  sec_pass "scan_progress: height=${h1} 已处理交易=${tx1} 更新于 ${t1}"

  local mk mseq mheight
  mk="$(mysql_query "SELECT COALESCE(MAX(seq),0), COALESCE(MAX(height),0), COUNT(*) FROM scan_seq_marker")"
  mseq="$(printf '%s' "$mk" | awk -F'\t' '{print $1}')"
  mheight="$(printf '%s' "$mk" | awk -F'\t' '{print $2}')"
  if [ "${mseq:-0}" -gt 0 ]; then
    sec_pass "scan_seq_marker: max seq=${mseq} height=${mheight}（$(printf '%s' "$mk" | awk -F'\t' '{print $3}') 条）"
  else
    sec_warn "scan_seq_marker 为空（还没扫到块？）"
  fi

  local h2
  sleep "$PROGRESS_WAIT"
  h2="$(mysql_query "SELECT last_block_number FROM scan_progress WHERE id=1")"
  if [ -n "$h2" ] && [ "${h2:-0}" -gt "${h1:-0}" ]; then
    sec_pass "${PROGRESS_WAIT}s 内推进：height ${h1}→${h2}"
  else
    sec_warn "${PROGRESS_WAIT}s 内没有推进（链可能没新块；或 scanner 卡住/未启动）"
  fi

  local chain lag
  chain="$(chain_height)"
  if is_num "$chain" && [ "$chain" -ge 0 ] && is_num "${h2:-}" && [ "${h2:-0}" -ge 0 ]; then
    lag=$((chain - h2))
    if [ "$lag" -lt 0 ]; then
      sec_warn "scanner 进度(${h2}) 比链高(${chain}) 还大，检查 CHAIN_HOST 是否指向同一条链"
    elif [ "$lag" -le "$MAX_LAG_BLOCKS" ]; then
      sec_pass "落后链尖 ${lag} 块（阈值 ${MAX_LAG_BLOCKS}）"
    elif [ "$lag" -le $((MAX_LAG_BLOCKS * 10)) ]; then
      sec_warn "落后链尖 ${lag} 块（同步中属正常）"
    else
      sec_fail "落后链尖 ${lag} 块：scanner 可能卡住"
    fi
  fi

  local bal
  bal="$(mysql_query "SELECT COUNT(*) FROM token_balances")"
  if [ -n "$bal" ] && [ "${bal:-0}" -gt 0 ]; then
    sec_pass "token_balances ${bal} 行"
  else
    sec_warn "token_balances 为空（还没扫到 ERC20 转账，或余额回填没做）"
  fi
}


# ---------- 4b. MySQL 数据健康（回归信号） ----------
check_data() {
  echo
  echo "[data] MySQL 数据新鲜度与回归信号"
  mysql_resolve
  if [ "$MYSQL_MODE_RESOLVED" = "skip" ]; then
    sec_skip "MYSQL_ENABLED!=true 或无 MySQL 访问方式"
    return
  fi

  # 1) 是否还在写（transactions 的 created_at 是服务器写入时间，与链时间无关）
  local age
  age="$(mysql_query "SELECT COALESCE(TIMESTAMPDIFF(MINUTE, MAX(created_at), NOW()), -1) FROM transactions")"
  if ! is_num "${age:-}"; then
    sec_fail "查不到 transactions（MySQL 连不上？）"
    return
  elif [ "$age" -lt 0 ]; then
    sec_warn "transactions 表为空"
  elif [ "$age" -le 30 ]; then
    sec_pass "最近一笔交易写入 ${age} 分钟前（仍在写）"
  elif [ "$age" -le 180 ]; then
    sec_warn "最近一笔交易写入 ${age} 分钟前（链上无新块？）"
  else
    sec_fail "已经 ${age} 分钟没有新交易写入：scanner/pull_convert 可能停写"
  fi

  # 2) 回归信号：这些数字一旦涨就说明又踩到过的坑
  local unk nullval nullmeta
  unk="$(mysql_query "SELECT COUNT(*) FROM contracts WHERE contract_type='UNKNOWN' AND created_at > NOW() - INTERVAL 1 HOUR")"
  if is_num "${unk:-}"; then
    if [ "$unk" -gt 50 ]; then
      sec_warn "近 1 小时新增 UNKNOWN 合约 ${unk} 条（合约判定可能又失败：看 scanner 日志 saving as UNKNOWN）"
    else
      sec_pass "近 1 小时新增 UNKNOWN 合约 ${unk} 条"
    fi
  fi

  # 只看最近区块，避免全表扫（value 无索引）
  local maxh fromh
  maxh="$(mysql_query "SELECT COALESCE(MAX(last_block_number),0) FROM scan_progress WHERE id=1")"
  if is_num "${maxh:-}" && [ "${maxh:-0}" -gt 1000 ]; then
    fromh=$((maxh - 2000))
    nullval="$(mysql_query "SELECT COUNT(*) FROM transactions WHERE block_number > ${fromh} AND value IS NULL AND func_name='eth_transfer'")"
    if is_num "${nullval:-}"; then
      if [ "$nullval" -gt 0 ]; then
        sec_warn "最近 2000 块里有 ${nullval} 条主币转账 value 为 NULL（scanner 版本过旧或写失败）"
      else
        sec_pass "最近 2000 块主币转账 value 均已写入"
      fi
    fi
  fi

  nullmeta="$(mysql_query "SELECT COUNT(*) FROM token_balances WHERE last_tx_hash IS NULL")"
  if is_num "${nullmeta:-}"; then
    if [ "$nullmeta" -gt 0 ]; then
      sec_warn "token_balances 有 ${nullmeta} 行 last_tx_hash 为 NULL（老版本 erc20api 会 500，跑过 SQL 或更新 erc20api）"
    else
      sec_pass "token_balances 的 last_tx_hash 无 NULL"
    fi
  fi

  # 3) 表是否在增长（用近似行数，避免 COUNT(*) 扫大表）
  local t1 t2
  t1="$(mysql_query "SELECT COALESCE(table_rows,0) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='transactions'")"
  sleep "$PROGRESS_WAIT"
  t2="$(mysql_query "SELECT COALESCE(table_rows,0) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='transactions'")"
  if is_num "${t1:-}" && is_num "${t2:-}" && [ "$t2" -gt "$t1" ]; then
    sec_pass "${PROGRESS_WAIT}s 内 transactions 增长 ${t1}→${t2}"
  elif is_num "${t1:-}" && is_num "${t2:-}"; then
    sec_warn "${PROGRESS_WAIT}s 内 transactions 行数没变（${t1}）：链没新块或写停"
  fi

  # 4) 行锁等待（本次事故里出现过 lock wait timeout）
  local lw1 lw2 delta
  lw1="$(mysql_query "SHOW GLOBAL STATUS LIKE 'Innodb_row_lock_waits'" | awk '{print $2}')"
  sleep 2
  lw2="$(mysql_query "SHOW GLOBAL STATUS LIKE 'Innodb_row_lock_waits'" | awk '{print $2}')"
  if is_num "${lw1:-}" && is_num "${lw2:-}"; then
    delta=$((lw2 - lw1))
    if [ "$delta" -gt 20 ]; then
      sec_warn "2 秒内新增 ${delta} 次 InnoDB 行锁等待（有长事务/并发争抢）"
    else
      sec_pass "行锁等待平稳（2s 内 +${delta}）"
    fi
  fi
}

# ---------- 5. erc20api ----------
check_erc20api() {
  echo
  echo "[erc20api] 接口验证（${ERC20_API_URL}）"
  local body code

  body="$(api_get '/health')"
  code="$(printf '%s' "$body" | jget code)"
  if [ "$(printf '%s' "$body" | jget data.status)" = "healthy" ]; then
    sec_pass "/health 正常（code=${code}）"
  else
    sec_fail "/health 异常：$(printf '%s' "$body" | head -c 120)"
  fi

  body="$(api_get '/evmapi/tokens?page=1&size=5')"
  code="$(printf '%s' "$body" | jget code)"
  local total sym dec
  total="$(printf '%s' "$body" | jget data.total)"
  sym="$(printf '%s' "$body" | jget data.tokens.0.symbol)"
  dec="$(printf '%s' "$body" | jget data.tokens.0.decimals)"
  if [ "$code" = "0" ] && [ "${total:-0}" -gt 0 ] && [ -n "${sym:-}" ]; then
    sec_pass "/evmapi/tokens 共 ${total} 个代币，示例 ${sym}(decimals=${dec:-?})"
  elif [ "$code" = "0" ]; then
    sec_warn "/evmapi/tokens 返回成功但没有数据（contracts 里还没有 ERC20？）"
  else
    sec_fail "/evmapi/tokens 失败：$(printf '%s' "$body" | head -c 160)"
  fi

  body="$(api_get '/evmapi/contracts?page=1&size=5')"
  code="$(printf '%s' "$body" | jget code)"
  total="$(printf '%s' "$body" | jget data.total)"
  sym="$(printf '%s' "$body" | jget data.contracts.0.symbol)"
  if [ "$code" = "0" ] && [ "${total:-0}" -gt 0 ]; then
    sec_pass "/evmapi/contracts 共 ${total} 个合约，示例 ${sym:-?}"
  elif [ "$code" = "0" ]; then
    sec_warn "/evmapi/contracts 返回成功但为空"
  else
    sec_fail "/evmapi/contracts 失败：$(printf '%s' "$body" | head -c 160)"
  fi

  # 代币详情（用列表里的第一个地址）
  local token_addr detail
  token_addr="$(printf '%s' "$(api_get '/evmapi/tokens?page=1&size=1')" | jget data.tokens.0.address)"
  if [ -n "${token_addr:-}" ]; then
    detail="$(api_get "/evmapi/tokens/${token_addr}")"
    if [ "$(printf '%s' "$detail" | jget code)" = "0" ] && [ -n "$(printf '%s' "$detail" | jget data.symbol)" ]; then
      sec_pass "/evmapi/tokens/${token_addr:0:10}… 详情正常（$(printf '%s' "$detail" | jget data.symbol)，总量 $(printf '%s' "$detail" | jget data.total_supply_formatted)）"
    else
      sec_warn "/evmapi/tokens/{addr} 详情异常：$(printf '%s' "$detail" | head -c 120)"
    fi
  fi

  # 非法地址必须 4xx（5xx 说明又出现了 NULL/空指针这类后端错误）
  local code_only
  code_only="$(curl -s -o /dev/null -w '%{http_code}' -m "$CHECK_TIMEOUT" \
    "$ERC20_API_URL/evmapi/accounts/0xnotanaddress/erc20-balances")"
  case "$code_only" in
    4*) sec_pass "非法地址返回 ${code_only}（参数校验正常）" ;;
    5*) sec_fail "非法地址返回 ${code_only}（服务端错误，可能是 Scan NULL/panic）" ;;
    *) sec_warn "非法地址返回 ${code_only}（预期 4xx）" ;;
  esac

  # 响应耗时
  local cost
  cost="$(curl -s -o /dev/null -w '%{time_total}' -m "$CHECK_TIMEOUT" "$ERC20_API_URL/evmapi/contracts?page=1&size=20")"
  if [ -n "${cost:-}" ]; then
    if awk -v c="$cost" 'BEGIN{exit !(c>2)}'; then
      sec_warn "/evmapi/contracts 耗时 ${cost}s（>2s，检查 MySQL 慢查询/锁等待）"
    else
      sec_pass "/evmapi/contracts 耗时 ${cost}s"
    fi
  fi

  # 拿余额最大的地址验证持仓接口（需要 MySQL）
  mysql_resolve
  local holder=""
  if [ "$MYSQL_MODE_RESOLVED" != "skip" ]; then
    holder="$(mysql_query "SELECT address FROM token_balances ORDER BY balance DESC LIMIT 1")"
  fi
  if [ -n "${holder:-}" ]; then
    body="$(api_get "/evmapi/accounts/${holder}/erc20-balances?page=1&size=5&min_balance=0")"
    code="$(printf '%s' "$body" | jget code)"
    local n
    n="$(printf '%s' "$body" | jlen data.tokens)"
    if [ "$code" = "0" ] && [ "${n:-0}" -gt 0 ]; then
      sec_pass "/evmapi/accounts/${holder:0:10}…/erc20-balances 返回 ${n} 条，示例余额 $(printf '%s' "$body" | jget data.tokens.0.balance)（symbol $(printf '%s' "$body" | jget data.tokens.0.symbol)）"
    elif [ "$code" = "0" ]; then
      sec_warn "/evmapi/accounts/…/erc20-balances 返回成功但无数据"
    else
      sec_fail "/evmapi/accounts/…/erc20-balances 失败：$(printf '%s' "$body" | head -c 160)"
    fi
    body="$(api_get "/evmapi/accounts/${holder}/approvals?page=1&size=5")"
    if [ "$(printf '%s' "$body" | jget code)" = "0" ]; then
      sec_pass "/evmapi/accounts/…/approvals 正常（$(printf '%s' "$body" | jlen data.approvals) 条）"
    else
      sec_warn "/evmapi/accounts/…/approvals 异常：$(printf '%s' "$body" | head -c 120)"
    fi
  else
    sec_skip "没有可用地址（MySQL 不可访问或 token_balances 为空），跳过持仓接口"
  fi
}

# ---------- 6. jrpc ----------
jrpc_call() { # $1=method $2=params json
  curl -s -m "$CHECK_TIMEOUT" -X POST "$JRPC_URL" -H 'Content-Type: application/json' \
    -d "{\"method\":\"$1\",\"params\":$2,\"id\":1}"
}

check_jrpc() {
  echo
  echo "[jrpc] 接口验证（${JRPC_URL}）"
  local body err count n first_height

  body="$(jrpc_call 'Block.Count' '[{}]')"
  err="$(printf '%s' "$body" | jget error)"
  count="$(printf '%s' "$body" | jget result)"
  if [ -n "${err:-}" ]; then
    sec_fail "Block.Count 报错：${err}"
  elif is_num "${count:-}" && [ "${count:-0}" -gt 0 ]; then
    sec_pass "Block.Count = ${count}（block_info 索引可查）"
  else
    sec_fail "Block.Count 返回异常：$(printf '%s' "$body" | head -c 160)"
  fi

  body="$(jrpc_call 'Block.BlockList' '[{"page":{"number":1,"size":1},"sort":[{"key":"height","ascending":false}]}]')"
  err="$(printf '%s' "$body" | jget error)"
  n="$(printf '%s' "$body" | jlen result)"
  first_height="$(printf '%s' "$body" | jget result.0.height)"
  if [ -n "${err:-}" ]; then
    sec_fail "Block.BlockList 报错：${err}"
  elif [ "${n:-0}" -gt 0 ] && [ -n "${first_height:-}" ]; then
    sec_pass "Block.BlockList 返回 ${n} 条，最新 height=${first_height}"
  else
    sec_fail "Block.BlockList 返回异常：$(printf '%s' "$body" | head -c 160)"
  fi

  body="$(jrpc_call 'Account.TopAssetCount' '[{}]')"
  err="$(printf '%s' "$body" | jget error)"
  count="$(printf '%s' "$body" | jget result)"
  if [ -n "${err:-}" ]; then
    sec_warn "Account.TopAssetCount 报错：${err}"
  elif is_num "${count:-}" && [ "${count:-0}" -gt 0 ]; then
    sec_pass "Account.TopAssetCount = ${count}"
  else
    sec_warn "Account.TopAssetCount 暂无数据"
  fi

  body="$(jrpc_call 'Account.TopAsset' '[{"page":{"number":1,"size":3}}]')"
  err="$(printf '%s' "$body" | jget error)"
  n="$(printf '%s' "$body" | jlen result)"
  if [ -n "${err:-}" ]; then
    sec_warn "Account.TopAsset 报错（可能是缓存未就绪）：${err}"
  elif [ "${n:-0}" -gt 0 ]; then
    sec_pass "Account.TopAsset 返回 ${n} 条，榜首余额 $(printf '%s' "$body" | jget result.0.total)"
  else
    sec_warn "Account.TopAsset 暂无数据（account 索引还没写？）"
  fi

  if is_num "${CONVERT_HEIGHT:-}" && [ "${CONVERT_HEIGHT:-0}" -gt 0 ] && is_num "${first_height:-}"; then
    local diff=$((CONVERT_HEIGHT - first_height))
    if [ "$diff" -lt 0 ]; then diff=$((-diff)); fi
    if [ "$diff" -le 2 ]; then
      sec_pass "jrpc 最新块高 ${first_height} 与 convert 进度 ${CONVERT_HEIGHT} 一致"
    else
      sec_warn "jrpc 最新块高 ${first_height} 与 convert 进度 ${CONVERT_HEIGHT} 相差 ${diff} 块（可能缓存/刷新延迟）"
    fi
  fi
}


# ---------- 7. 日志错误扫描 ----------
check_logs() {
  echo
  echo "[logs] 各程序日志最近 ${LOG_LINES} 行的错误信号（${ROOT_DIR}/bin/logs）"
  local dir="$ROOT_DIR/bin/logs"
  if [ ! -d "$dir" ]; then
    sec_skip "没有日志目录 $dir（程序没起过？）"
    return
  fi
  local f n panic
  for f in "$dir"/*.log; do
    [ -f "$f" ] || continue
    case "$f" in *.error.log) continue ;; esac
    n="$(tail -n "$LOG_LINES" "$f" | grep -cE 'lvl=eror|lvl=crit|level=(ERROR|CRIT)|"level":"(ERROR|CRITICAL)"' 2>/dev/null || true)"
    panic="$(tail -n "$LOG_LINES" "$f" | grep -cE 'panic:|SIGSEGV' 2>/dev/null || true)"
    if [ "${panic:-0}" -gt 0 ]; then
      sec_fail "$(basename "$f") 出现 panic/SIGSEGV ${panic} 次（进程可能反复退出）"
    elif [ "${n:-0}" -gt 0 ]; then
      sec_warn "$(basename "$f") 最近 ${LOG_LINES} 行有 ${n} 条 ERROR"
    else
      sec_pass "$(basename "$f") 无 ERROR"
    fi
  done

  # 历史踩过的坑，出现即提示
  local kw cnt
  for kw in "saving as UNKNOWN" "invalid connection" "circuit_breaking" "Lock wait timeout"             "unexpected EOF" "empty return data"; do
    cnt=0
    for f in "$dir"/*.log; do
      [ -f "$f" ] || continue
      case "$f" in *.error.log) continue ;; esac
      c="$(tail -n "$LOG_LINES" "$f" | grep -c "$kw" 2>/dev/null || true)"
      cnt=$((cnt + ${c:-0}))
    done
    if [ "$cnt" -gt 0 ]; then
      sec_warn "日志关键字「${kw}」出现 ${cnt} 次（对应已知故障：合约判定失败/连接被断/ES 429/锁冲突）"
    fi
  done
}

# ---------- main ----------
SECTIONS="${*:-all}"
want() {
  [ "$SECTIONS" = "all" ] && return 0
  for s in $SECTIONS; do [ "$s" = "$1" ] && return 0; done
  return 1
}

echo "=== externaldb（pull 链路）部署自检 ==="
echo "包目录 : $ROOT_DIR"
echo "ES     : $ES_URL（前缀 ${CONVERT_PREFIX}）"
echo "链节点 : ${CHAIN_URL:-未配置}"
echo "erc20api: $ERC20_API_URL"
echo "jrpc   : $JRPC_URL"

want docker && check_docker
want runtime && check_runtime
want es && check_es
if want progress || want convert; then check_convert; fi
if want progress || want scanner; then check_scanner; fi
want data && check_data
want erc20api && check_erc20api
want jrpc && check_jrpc
want logs && check_logs

echo
echo "=== 汇总：PASS ${PASS}  WARN ${WARN}  FAIL ${FAIL}  SKIP ${SKIP} ==="
if [ "$FAIL" -gt 0 ]; then
  echo "结果：有 FAIL，按上面提示排查（WARN 通常是同步中/节点未配置，可接受）"
  exit 1
fi
echo "结果：通过"
exit 0
