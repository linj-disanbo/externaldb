.PHONY: quickStart run logs logs-convert logs-scanner logs-erc20api logs-jrpc stop restart rm down clean initConfig initES restartES downES check es-indices es-marker

include .env

# env/es7 默认是 ES 7.17；老环境要用 7.4，把包内 env/es7.4 的内容换到 env/es7 即可
ENV_DIR ?= env/es${ES_VERSION}

# scanner / erc20api 依赖 MySQL：MYSQL_ENABLED=true 才启动（见 .env）
ERC20_SERVICES := $(if $(filter true,$(MYSQL_ENABLED)),scanner erc20api)

# 按 EX_SERVICE_TYPE 跑对应的初始化：
#   proof   -> 存证索引/初始数据（env/<dir>/proof-init.bash）
#   browser -> 浏览器用到的索引 settings（browser-init.bash：max_result_window / refresh_interval 等）
#   common  -> 两者都跑
# 注意 browser-init.bash 会调整 ${CONVERT_PREFIX}*/_settings（含 translog），
# 但不会碰 seq_marker（进度索引），避免影响游标语义。
define RUN_SERVICE_INIT
	if [ "${EX_SERVICE_TYPE}" = "proof" ]; then \
		cd ./$(ENV_DIR) && chmod +x proof-init.bash && ./proof-init.bash && cd ../..; \
	elif [ "${EX_SERVICE_TYPE}" = "browser" ]; then \
		chmod +x browser-init.bash && ./browser-init.bash; \
	else \
		cd ./$(ENV_DIR) && chmod +x proof-init.bash && ./proof-init.bash && cd ../..; \
		chmod +x browser-init.bash && ./browser-init.bash; \
	fi
endef

quickStart:
	@echo "================start initES================"
	@bash es-init.sh
	@echo "==============waiting es start=============="
	@until [ `curl -o /dev/null -s -w %{http_code} http://localhost:${ES_PORT_HTTP} -u elastic:${ES_PASSWORD}` = 200 ]; do \
        		sleep 1s; \
    done
	@echo "==============start initConfig=============="
	@bash alter-config.sh
	@$(RUN_SERVICE_INIT)
	@echo "============start run externaldb============"
	@cd ./bin && chmod +x convert jrpc $(ERC20_SERVICES)
	@if [ "${EX_SERVICE_TYPE}" = "proof" ]; then \
		docker-compose -p ${PROJECT_NAME} up -d convert; \
	else \
		docker-compose -p ${PROJECT_NAME} up -d convert jrpc $(ERC20_SERVICES); \
	fi
	@echo "==================complete=================="

run:
	@cd ./bin && chmod +x convert jrpc $(ERC20_SERVICES)
	@if [ "${EX_SERVICE_TYPE}" = "proof" ]; then \
		docker-compose -p ${PROJECT_NAME} up -d convert; \
	else \
		docker-compose -p ${PROJECT_NAME} up -d convert jrpc $(ERC20_SERVICES); \
	fi

# 对账/巡检（marker 链连续性、marker 与 block_info 一致性、旧 last_seq 是否停更）
check:
	@./bin/convert_check -f ./etc/externaldb.toml || true

log-convert:
	@tail -200f ./bin/logs/convert.log
log-convert-err:
	@tail -200f ./bin/logs/convert.error.log
log-jrpc:
	@tail -200f ./bin/logs/jrpc.log
log-scanner:
	@tail -200f ./bin/logs/scanner_app.log
log-erc20api:
	@tail -200f ./bin/logs/rpc_app.log

logs:
	@docker-compose -p ${PROJECT_NAME} logs

logs-convert:
	@docker-compose -p ${PROJECT_NAME} logs convert

logs-jrpc:
	@docker-compose -p ${PROJECT_NAME} logs jrpc

logs-scanner:
	@docker-compose -p ${PROJECT_NAME} logs scanner

logs-erc20api:
	@docker-compose -p ${PROJECT_NAME} logs erc20api

stop:
	@docker-compose -p ${PROJECT_NAME} stop

restart:
	@docker-compose -p ${PROJECT_NAME} restart

rm:
	@docker-compose -p ${PROJECT_NAME} rm

down:
	@docker-compose -p ${PROJECT_NAME} down

clean:
	@rm -rf ./bin/logs

initConfig:
	@bash alter-config.sh
	@$(RUN_SERVICE_INIT)

alterExConfig:
	@bash alter-config.sh

alterEsInit:
	@$(RUN_SERVICE_INIT)

# 单机 ES：启动后把已有索引的副本数设为 0（副本无处分配只会让集群一直 yellow；
# 新建索引的副本数由 [esIndex] numberOfReplicas 决定，见 alter-config.sh 的 ES_REPLICAS）
initES:
	@bash es-init.sh
	@echo "==============waiting es start=============="
	@until [ `curl -o /dev/null -s -w %{http_code} http://localhost:${ES_PORT_HTTP} -u elastic:${ES_PASSWORD}` = 200 ]; do \
        		sleep 1s; \
    done
	@echo "==============set existing indices replicas=0=============="
	@curl -s -X PUT "http://localhost:${ES_PORT_HTTP}/${CONVERT_PREFIX}*/_settings" \
		-H 'Content-Type: application/json' -u elastic:${ES_PASSWORD} \
		-d '{"index":{"number_of_replicas":0}}' >/dev/null || true

restartES:
	@docker-compose -f ./$(ENV_DIR)/docker-compose.yaml restart

downES:
	@docker-compose -f ./$(ENV_DIR)/docker-compose.yaml down

es-indices:
	curl -s --location --request GET http://localhost:${ES_PORT_HTTP}/_cat/indices?v -u elastic:${ES_PASSWORD} | grep "${CONVERT_PREFIX}\|${SYNC_PREFIX}\|docs.count"

# 直连改造：seq_marker 是否在推进（游标）；索引名 = [convertEs] prefix + seq_marker
es-marker:
	curl -s --location --request GET "http://localhost:${ES_PORT_HTTP}/${CONVERT_PREFIX}seq_marker/_search?pretty&size=1&sort=seq:desc" -u elastic:${ES_PASSWORD}
