#!/bin/bash
# shellcheck disable=SC2086
CONF=./etc/externaldb.toml

. .env

# get_key_value key [section]
# 取 key 的当前值：给了 section 就只在 [section] 段内找（段内第一个匹配），否则全文找第一个。
get_key_value() {
    local key=$1 section=${2#\[}
    section=${section%\]}
    awk -v sec="$section" -v key="$key" '
        BEGIN { insec = (sec == "") ? 1 : 0 }
        $0 ~ ("^\\[" sec "\\][ \t\r]*$") { insec = 1; next }
        sec != "" && insec && $0 ~ /^\[/ { insec = 0 }
        insec && $0 ~ ("^" key "[ \t\r]*=") {
            sub("^" key "[ \t\r]*=[ \t\r]*", "")
            print
            exit
        }
    ' "$CONF"
}

# replace_key_value key value [section]
# 按 section 定位替换（不再用行号）：只改该段内第一个 key，避免误改其它段同名 key。
# 完整重写文件（cat 覆盖）以保留原文件权限/inode；值里带 | & / 等都安全。
replace_key_value() {
    local key=$1 value=$2 section=${3#\[}
    section=${section%\]}
    awk -v sec="$section" -v key="$key" -v val="$value" '
        BEGIN { insec = (sec == "") ? 1 : 0 }
        $0 ~ ("^\\[" sec "\\][ \t\r]*$") { insec = 1; print; next }
        sec != "" && insec && $0 ~ /^\[/ { insec = 0 }
        insec && !done && $0 ~ ("^" key "[ \t\r]*=") {
            print key " = " val
            done = 1
            insec = 0
            next
        }
        { print }
    ' "$CONF" > "$CONF.tmp" && cat "$CONF.tmp" > "$CONF" && rm -f "$CONF.tmp"
}

# set_key_value key value [section]
set_key_value() {
    local key=$1 value=$2 section=$3 current
    if [ -z "$value" ] || [ "$value" = \"\" ]; then
        return 0
    fi
    current=$(get_key_value "$key" "$section")
    if [ -z "$current" ]; then
        # 改成按段定位后，未命中要说出来（以前行号失配是静默跳过）
        echo "!! NOT FOUND: $key${section:+ in [$section]} => 跳过"
        return 0
    fi
    echo "setting $CONF : $key = $value${section:+ (section [$section])}  [was: $current]"
    replace_key_value "$key" "$value" "$section"
}

# combine_es_host key port [section]：host 统一写成 "http://localhost:<port>/"
combine_es_host() {
    local key=$1 port=$2 section=$3
    if [ -z "$port" ] || [ "$port" = \"\" ]; then
        return 0
    fi
    set_key_value "$key" "\"http://localhost:$port/\"" "$section"
}

deal_browser_config() {
    line1=$(sed -n "/======start======/=" $CONF)
    line2=$(sed -n "/======end======/=" $CONF)

    start=$(("$line1" + 1))
    end=$(("$line2" - 1))

    if [ "$EX_SERVICE_TYPE" = "proof" ]; then
        sed -i "${start},${end}s/^/#/g" $CONF
        set_key_value "openAccessControl" true
        set_key_value "saveBlockInfo" false
    else
        sed -i "${start},${end}s/^#*\s*//g" $CONF
        set_key_value "openAccessControl" false
        set_key_value "saveBlockInfo" true
    fi
}

# set_key_value key value [section]：key 在多个段里重复时用第三个参数指定段名（如 chain/syncEs/convertEs/sync/rpc）；
# 按 section 定位，配置增删行都不会失效。
# chain相关
set_key_value "title" \"${CHAIN_TITLE}\"
set_key_value "host" \"${CHAIN_HOST}\" chain
set_key_value "grpcHost" \"${CHAIN_GRPC_HOST}\"
set_key_value "symbol" \"${CHAIN_SYMBOL}\"
# es相关
set_key_value "esVersion" ${ES_VERSION}

combine_es_host "host" ${ES_PORT} syncEs
set_key_value "prefix" \"${SYNC_PREFIX}\" syncEs
set_key_value "pwd" \"${ES_PASSWORD}\" syncEs

combine_es_host "host" ${ES_PORT} convertEs
set_key_value "prefix" \"${CONVERT_PREFIX}\" convertEs
set_key_value "pwd" \"${ES_PASSWORD}\" convertEs
# sync相关
set_key_value "pushBind" \"${PUSH_BIND}\"
set_key_value "pushHost" \"${PUSH_HOST}\"
set_key_value "pushName" \"${PUSH_NAME}\"
set_key_value "pushFormat" \"${PUSH_FORMAT}\"
set_key_value "startSeq" ${START_SEQ} sync
set_key_value "startHeight" ${START_HEIGHT}
# rpc相关
set_key_value "host" \"${RPC_HOST}\" rpc
set_key_value "jrpcHost" \"${JRPC_HOST}\"
set_key_value "swaggerHost" \"${SWAGGER_HOST}\"
# convert相关
set_key_value "addressDriver" \"${ADDR_DRIVER}\"
set_key_value "dealOtherChain" ${DEAL_OTHER_CHAIN}

deal_browser_config
