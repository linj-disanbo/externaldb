.PHONY: quickStart run logs logs-pull-convert logs-rpc logs-jrpc stop restart rm down clean initConfig initES restartES downES check

include .env

# env/es7 默认是 ES 7.17；老环境要用 7.4，把包内 env/es7.4 的内容换到 env/es7 即可
ENV_DIR ?= env/es${ES_VERSION}

quickStart:
	@echo "================start initES================"
	@bash es-init.sh
	@echo "==============waiting es start=============="
	@until [ `curl -o /dev/null -s -w %{http_code} http://localhost:${ES_PORT_HTTP} -u elastic:${ES_PASSWORD}` = 200 ]; do \
        		sleep 1s; \
    done
	@echo "==============start initConfig=============="
	@bash alter-config.sh
	@cd ./$(ENV_DIR) && chmod +x proof-init.bash && ./proof-init.bash && cd ../..
	@echo "============start run externaldb============"
	@cd ./bin && chmod +x pull_convert rpc jrpc
	@if [ "${EX_SERVICE_TYPE}" = "proof" ]; then \
		docker-compose -p ${PROJECT_NAME} up -d pull_convert rpc; \
	elif [ "${EX_SERVICE_TYPE}" = "browser" ]; then \
		docker-compose -p ${PROJECT_NAME} up -d pull_convert jrpc; \
	else \
		docker-compose -p ${PROJECT_NAME} up -d; \
	fi
	@echo "==================complete=================="

run:
	@cd ./bin && chmod +x pull_convert rpc jrpc
	@if [ "${EX_SERVICE_TYPE}" = "proof" ]; then \
		docker-compose -p ${PROJECT_NAME} up -d pull_convert rpc; \
	elif [ "${EX_SERVICE_TYPE}" = "browser" ]; then \
		docker-compose -p ${PROJECT_NAME} up -d pull_convert jrpc; \
	else \
		docker-compose -p ${PROJECT_NAME} up -d; \
	fi

# 对账/巡检（marker 链连续性、marker 与 block_info 一致性、旧 last_seq 是否停更）
check:
	@./bin/convert_check -f ./etc/externaldb.toml || true

log-pull-convert:
	@tail -200f ./bin/logs/pull_convert.log
log-pull-convert-err:
	@tail -200f ./bin/logs/pull_convert.error.log
log-rpc:
	@tail -200f ./bin/logs/rpc.log

logs:
	@docker-compose -p ${PROJECT_NAME} logs

logs-pull-convert:
	@docker-compose -p ${PROJECT_NAME} logs pull_convert

logs-rpc:
	@docker-compose -p ${PROJECT_NAME} logs rpc

logs-jrpc:
	@docker-compose -p ${PROJECT_NAME} logs jrpc

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
	@cd ./$(ENV_DIR) && chmod +x proof-init.bash && ./proof-init.bash && cd ../..

alterExConfig:
	@bash alter-config.sh

alterEsInit:
	@cd ./$(ENV_DIR) && ./proof-init.bash && cd ../..

initES:
	@bash es-init.sh

restartES:
	@docker-compose -f ./$(ENV_DIR)/docker-compose.yaml restart

downES:
	@docker-compose -f ./$(ENV_DIR)/docker-compose.yaml down

es-indices:
	curl -s --location --request GET http://localhost:${ES_PORT_HTTP}/_cat/indices?v -u elastic:${ES_PASSWORD} | grep "${CONVERT_PREFIX}\|${SYNC_PREFIX}\|docs.count"

# 直连改造：seq_marker 是否在推进（游标）；索引名 = [convertEs] prefix + seq_marker
es-marker:
	curl -s --location --request GET "http://localhost:${ES_PORT_HTTP}/${CONVERT_PREFIX}seq_marker/_search?pretty&size=1&sort=seq:desc" -u elastic:${ES_PASSWORD}
