#!/usr/bin/env bash

. ../../.env

prefix=${CONVERT_PREFIX}
port=${ES_PORT}
manager=${PROOF_MANAGER}

ES_USER=elastic

OLD_IFS="$IFS"
IFS=","
proof_manager=($manager)
IFS="$OLD_IFS"

managerCount=${#proof_manager[*]}

# 用管理员地址替换
for i in ${proof_manager[*]}; do
  curl -X PUT http://localhost:${port}/${prefix}proof_config/proof_config/member-${i}/_create -d'{"address":"'${i}'","role": "manager","organization": "system", "note" : "system-init-manager"}' -H "Content-Type: application/json" -u $ES_USER:$ES_PASSWORD
  echo ""
done

curl -X PUT http://localhost:${port}/${prefix}proof_config_org/proof_config_org/org-system/_create -d'{"count": '${managerCount}',"organization": "system", "note" : "system-init"}' -H "Content-Type: application/json" -u $ES_USER:$ES_PASSWORD
echo ""

curl -X PUT http://localhost:${port}/${prefix}proof/proof/proof-create-proof-index/_create -d '{"init": 1}' -H "Content-Type: application/json" -u $ES_USER:$ES_PASSWORD
echo ""
curl -X PUT http://localhost:${port}/${prefix}proof/proof/_mapping?pretty -d '{"proof": {"properties": {"捐赠数量": {"type": "double"}}}}' -H "Content-Type: application/json" -u $ES_USER:$ES_PASSWORD
echo ""
curl -X PUT http://localhost:${port}/${prefix}proof/proof/_mapping?pretty -d '{"proof": {"properties": {"志愿者数量": {"type": "long"}}}}' -H "Content-Type: application/json" -u $ES_USER:$ES_PASSWORD
echo ""
curl -X PUT http://localhost:${port}/${prefix}proof/_settings -d '{ "index" : { "max_result_window" : 20000}}' -H "Content-Type: application/json" -u $ES_USER:$ES_PASSWORD
echo ""
curl -X PUT http://localhost:${port}/${prefix}proof/_settings -d '{"index.mapping.total_fields.limit":1000000}' -H "Content-Type: application/json" -u $ES_USER:$ES_PASSWORD
echo ""
curl -X DELETE http://localhost:${port}/${prefix}proof/proof/proof-create-proof-index -H "Content-Type: application/json" -u $ES_USER:$ES_PASSWORD
echo ""

if [ "${START_SEQ}" != 0 ] && [ `curl -o /dev/null -s -w %{http_code} http://localhost:${ES_PORT}/${SYNC_PREFIX}last_seq/_search -u $ES_USER:$ES_PASSWORD` != 200 ]; then
    curl -X PUT http://localhost:${port}/${SYNC_PREFIX}last_seq/last_seq/sync_seq?pretty -H 'Content-Type: application/json' -d '{"sync_seq":'${START_SEQ}'}' -u $ES_USER:$ES_PASSWORD
else
  echo "seq record has exist"
fi
