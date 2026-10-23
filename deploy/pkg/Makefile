.PHONY: quickStart run logs logs-sync logs-convert logs-rpc logs-jrpc stop restart rm down clean initConfig alterExConfig alterEsInit initES restartES downES es-indices reindex-shards check-es-version

include .env

# 只支持 ES 7.x：env/es7 是包内默认（7.17），env/es7.4 是 7.4 备用。
# 老环境要用 7.4，把 env/es7.4 的内容换到 env/es7 即可（不改脚本、不加版本配置）。
ENV_DIR ?= env/es7

# 场景只决定「起哪些服务」；初始化三个场景都跑同样几步（多跑几步没有副作用）。
#   proof   -> sync convert rpc
#   browser -> sync convert jrpc
#   其它    -> 全部
# .env 里的值是带引号的（EX_SERVICE_TYPE="proof"），比对前先去掉引号
SERVICE_TYPE = $(subst ",,$(EX_SERVICE_TYPE))
SERVICES = $(if $(filter proof,$(SERVICE_TYPE)),sync convert rpc,$(if $(filter browser,$(SERVICE_TYPE)),sync convert jrpc,sync convert rpc jrpc))

# proof-init.bash：存证索引/初始管理员（在 env/<dir>/ 下，脚本读 ../../.env）
# browser-init.bash：浏览器用到的索引 settings（max_result_window / refresh_interval / translog）
define RUN_SERVICE_INIT
	cd ./$(ENV_DIR) && chmod +x proof-init.bash && ./proof-init.bash && cd ../..
	chmod +x browser-init.bash && ./browser-init.bash
endef

check-es-version:
	@if [ "${ES_VERSION}" = "6" ]; then \
		echo "ES6 已不支持（escli/v6 已删除），请把 .env 的 ES_VERSION 设为 7"; \
		exit 1; \
	fi

quickStart: check-es-version
	@echo "================start initES================"
	@bash es-init.sh
	@echo "==============waiting es start=============="
	@until [ `curl -o /dev/null -s -w %{http_code} http://localhost:${ES_PORT_HTTP} -u elastic:${ES_PASSWORD}` = 200 ]; do \
        		sleep 1s; \
    done
	@echo "==============start initConfig=============="
	@bash alter-config.sh
	$(RUN_SERVICE_INIT)
	@echo "============start run externaldb============"
	@cd ./bin && chmod +x sync convert rpc jrpc
	@docker-compose -p ${PROJECT_NAME} up -d $(SERVICES)
	@echo "==================complete=================="

run: check-es-version
	@cd ./bin && chmod +x sync convert rpc jrpc
	@docker-compose -p ${PROJECT_NAME} up -d $(SERVICES)

log-sync:
	@tail -200f ./bin/logs/sync.log
log-sync-err:
	@tail -200f ./bin/logs/sync.error.log
log-convert:
	@tail -200f ./bin/logs/convert.log
log-convert-err:
	@tail -200f ./bin/logs/convert.error.log

logs:
	@docker-compose -p ${PROJECT_NAME} logs

logs-sync:
	@docker-compose -p ${PROJECT_NAME} logs sync

logs-convert:
	@docker-compose -p ${PROJECT_NAME} logs convert

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

initConfig: check-es-version
	@bash alter-config.sh
	$(RUN_SERVICE_INIT)

alterExConfig:
	@bash alter-config.sh

alterEsInit: check-es-version
	$(RUN_SERVICE_INIT)

initES: check-es-version
	@bash es-init.sh

restartES:
	@docker-compose -f ./$(ENV_DIR)/docker-compose.yaml restart

downES:
	@docker-compose -f ./$(ENV_DIR)/docker-compose.yaml down

es-indices:
	curl -s --location --request GET http://localhost:${ES_PORT_HTTP}/_cat/indices?v -u elastic:${ES_PASSWORD} | grep "${CONVERT_PREFIX}\|${SYNC_PREFIX}\|docs.count"

# 给已有索引换多分片（number_of_shards 创建后不可改，只能 reindex + 同名别名切换）
# 用法：make reindex-shards N=3 IDX="account transaction"
reindex-shards:
	@./reindex-shards.sh -n ${N} ${IDX}

search-nft:
	curl -s --location --request GET http://localhost:${ES_PORT_HTTP}/${CONVERT_PREFIX}nft/_search?pretty -u elastic:${ES_PASSWORD}

search-contract:
	curl -s --location --request GET http://localhost:${ES_PORT_HTTP}/${CONVERT_PREFIX}contract/_search?pretty -u elastic:${ES_PASSWORD}

reset-convert:stop clean-convert-indices initConfig run
	@echo done

clean-convert-indices:
	curl -s --location --request DELETE http://localhost:${ES_PORT_HTTP}/${CONVERT_PREFIX}* -u elastic:${ES_PASSWORD}

search-evm-%:
	curl -s --location --request GET http://localhost:${ES_PORT_HTTP}/${CONVERT_PREFIX}/_doc/evm-${*} -u elastic:${ES_PASSWORD}

clean-file-cache:
	curl -s --location --request DELETE http://localhost:${ES_PORT_HTTP}/${CONVERT_PREFIX}file_part -u elastic:${ES_PASSWORD}

# 数据需要重新同步时可采用此方法在后台同步，同步完成后更新
.PHONY: init-update run-update stop-update switch-update
init-update:
	@cp ./etc/externaldb.toml ./etc/externaldb-bak.toml
	@make initConfig
	@cp ./etc/externaldb.toml ./etc/externaldb-update.toml
	@cp ./etc/externaldb-bak.toml ./etc/externaldb.toml
	@mkdir -p ./bin-update/
	@cp ./bin/convert ./bin-update/

run-update:
	@docker-compose -f docker-compose-update.yaml -p ${PROJECT_NAME} up -d

stop-update:
	@docker-compose -f docker-compose-update.yaml -p ${PROJECT_NAME} down

switch-update: stop-update stop
	@cp ./etc/externaldb-update.toml ./etc/externaldb.toml
	@make run
