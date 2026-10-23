# golang1.12 or latest
# 1. make help
# 2. make dep
# 3. make build
# ...
SRC := github.com/33cn/externaldb
SRC_CLI := ${SRC}/cmd/
SRC_RPC := ${SRC_CLI}jrpc
PRC := build/jrpc
CLI_SYNC := build/sync
CLI_CONVERT := build/convert

AUTOTEST := build/autotest/autotest
SRC_AUTOTEST := github.com/33cn/chain33/cmd/autotest

PKG_LIST := `go list ./... | grep -v "mocks"`
PKG_LIST_VET := `go list ./... | grep -v "common/log/log15"`
PKG_LIST_INEFFASSIGN= `go list -f {{.Dir}} ./...  grep -v "common/log/log15"`
PKG_LIST_Q := `go list ./... | grep -v "mocks"`
PKG_LIST_GOSEC := `go list -f "{{.Dir}}" ./... | grep -v "mocks" | grep -v "cmd" | grep -v "types" | grep -v "commands" | grep -v "log15"`

# 版本信息
VERSION_LDFLAGS = -X github.com/33cn/externaldb/version.GitCommit=`git rev-parse --short=8 HEAD` -X github.com/33cn/externaldb/version.ReleaseDate=`date +%Y%m%d`
# 动态编译
BUILD_FLAGS = -ldflags "$(VERSION_LDFLAGS)"
# 静态编译 (CGO_ENABLED=0, 纯 Go 无需 musl)
STATIC_BUILD = CGO_ENABLED=0 go build -ldflags "-w -s $(VERSION_LDFLAGS)"
MKPATH=$(abspath $(lastword $(MAKEFILE_LIST)))
MKDIR=$(dir $(MKPATH))
DAPP := ""
PROJ := "build"

.PHONY: default dep all build release linter race test fmt vet bench msan coverage coverhtml docker docker-compose protobuf clean help autotest

default: build
include .env

dep: ## Get the dependencies
	@go get github.com/golangci/golangci-lint/cmd/golangci-lint@v1.40.1
	@go get -u golang.org/x/tools/cmd/goimports
	@go get -u github.com/mitchellh/gox
	@go get -u github.com/vektra/mockery/.../
	@go get -u mvdan.cc/sh/cmd/shfmt
	@go get -u mvdan.cc/sh/cmd/gosh
	@git checkout go.mod go.sum
	@apt install clang-format || echo run \"apt install clang-format\" by root
	@apt install shellcheck || echo run \"apt install shellcheck\" by root
	@go get -u github.com/swaggo/swag/cmd/swag@v1.7.8

.PHONY: proto
proto: ## Generate protbuf file of types package
	@cd ./proto && ./create_protobuf.sh && cd ..

.PHONY: swag
swag: ## gen rpc doc with swag
	@cd ./cmd/rpc && swag init && cd ../..

help: ## Display this help screen
	@printf "Help doc:\nUsage: make [command]\n"
	@printf "[command]\n"
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

fmt: fmt_go fmt_proto fmt_shell ## fmt .go .proto .sh

.PHONY: fmt_go fmt_proto fmt_shell
fmt_go: ## fmt go
	find . -path ./local -prune -o -path ./.gomodcache -prune -o -path ./vendor -prune -o -name '*.go' -print | xargs gofmt -s -w
	find . -path ./local -prune -o -path ./.gomodcache -prune -o -path ./vendor -prune -o -name '*.go' -print | xargs goimports -l -w

fmt_proto: ## go fmt protobuf file
	@find . -path ./local -prune -o -path ./.gomodcache -prune -o -path ./vendor -prune -o -name '*.proto' -print | xargs clang-format -i

fmt_shell: ## check shell file
	find . -path ./local -prune -o -path ./.gomodcache -prune -o -path ./vendor -prune -o -name '*.sh' -print | xargs shfmt -w -s -i 4 -ci -bn

shcheck: ## check shell
	shellcheck tools/*sh deploy/*sh deploy/ci/*sh

.PHONY: checkgofmt
checkgofmt: ## get all go files and run go fmt on them
	@files=$$(find . -path ./local -prune -o -path ./.gomodcache -prune -o -path ./vendor -prune -o -name '*.go' -print | xargs gofmt -l -s); if [ -n "$$files" ]; then \
		  echo "Error: 'make fmt' needs to be run on:"; \
		  find . -path ./local -prune -o -path ./.gomodcache -prune -o -path ./vendor -prune -o -name '*.go' -print | xargs gofmt -l -s ;\
		  exit 1; \
		  fi;
	@files=$$(find . -path ./local -prune -o -path ./.gomodcache -prune -o -path ./vendor -prune -o -name '*.go' -print | xargs goimports -l -w); if [ -n "$$files" ]; then \
		  echo "Error: 'make fmt' needs to be run on:"; \
		  find . -path ./local -prune -o -path ./.gomodcache -prune -o -path ./vendor -prune -o -name '*.go' -print | xargs goimports -l -w ;\
		  exit 1; \
		  fi;

.PHONY: doc
doc: swag
	@go build $(BUILD_FLAGS) -v -o swag2md ${SRC}/tools/swag2md
	@bash ./tools/doc/doc.sh

.PHONY: code_gen
code_gen: swag

build: code_gen ## Build all binaries (static, CGO_ENABLED=0)
	@$(STATIC_BUILD) -o  $(PRC) $(SRC_RPC)
	@$(STATIC_BUILD) -o  $(CLI_SYNC) $(SRC_CLI)/sync
	@$(STATIC_BUILD) -o  $(CLI_CONVERT) $(SRC_CLI)/convert
	@$(STATIC_BUILD) -o  build/convertfix $(SRC_CLI)/convert-fix-tool/
	@$(STATIC_BUILD) -o  build/rpc ${SRC_CLI}rpc/
	@$(STATIC_BUILD) -o  build/dummy_node $(SRC_CLI)/dummy_node/
	@$(STATIC_BUILD) -o  build/sync_convert $(SRC_CLI)/sync_convert
	@$(STATIC_BUILD) -o  build/pull_convert $(SRC_CLI)/pull_convert
	@$(STATIC_BUILD) -o  build/convert_check $(SRC_CLI)/convert_check
	@$(STATIC_BUILD) -o  build/erc20scan $(SRC_CLI)/erc20scan
	@$(STATIC_BUILD) -o  build/erc20api $(SRC_CLI)/erc20api
	@$(STATIC_BUILD) -o  build/account_refresh $(SRC_CLI)/account_refresh
	@cp config/externaldb.toml build/externaldb.toml

.PHONY: build_convert
build_convert: proto ## Build convert binary (static)
	@$(STATIC_BUILD) -o  $(CLI_CONVERT) $(SRC_CLI)/convert

.PHONY: build_rpc
build_rpc: code_gen ## Build rpc binary (static)
	@$(STATIC_BUILD) -o  build/rpc ${SRC_CLI}rpc/

release: code_gen ## Build core binaries (static, CGO_ENABLED=0)
	@$(STATIC_BUILD) -o  $(PRC) $(SRC_RPC)
	@$(STATIC_BUILD) -o  $(CLI_SYNC) $(SRC_CLI)/sync
	@$(STATIC_BUILD) -o  $(CLI_CONVERT) $(SRC_CLI)/convert
	@$(STATIC_BUILD) -o  build/rpc ${SRC_CLI}rpc/
	@cp config/externaldb.toml build/externaldb.toml

clean: ## Remove previous build
	@rm -rf build/*
	@go clean ./...

test: ## Run unittests
	@go test -race $(PKG_LIST)

testq: ## Run unittests
	@go test $(PKG_LIST_Q)

bench: ## Run benchmark of all
	@go test ./... -v -bench=.

autotest: test ## build autotest binary

autotest_ci: test ## autotest jerkins ci

build_ci: dep build ## Build the binary file for CI

linter: vet ineffassign gosec ## Use gometalinter check code, ignore some unserious warning
	@./tools/golinter.sh "filter"
	@find . -path ./local -prune -o -path ./.gomodcache -prune -o -path ./vendor -prune -o -name '*.sh' -print | xargs shellcheck -x

linter_test: ## Use gometalinter check code, for local test
	@./tools/golinter.sh "test" "${p}"
	@find . -path ./local -prune -o -path ./.gomodcache -prune -o -path ./vendor -prune -o -name '*.sh' -print | xargs shellcheck -x

gosec:
	@golangci-lint  run --no-config --issues-exit-code=1  --deadline=2m --disable-all --enable=gosec ${PKG_LIST_GOSEC}

race: ## Run data race detector
	@go test -race -short $(PKG_LIST)

vet:
	@go vet ${PKG_LIST_VET}

ineffassign:
	@golangci-lint  run --no-config --issues-exit-code=1  --deadline=2m --disable-all   --enable=ineffassign   -n ${PKG_LIST_INEFFASSIGN}


msan: ## Run memory sanitizer
	@go test -msan -short $(PKG_LIST)

coverage: ## Generate global code coverage report
	@./build/tools/coverage.sh

coverhtml: ## Generate global code coverage report in HTML
	@./build/tools/coverage.sh html



.PHONY: auto_ci
auto_fmt := find . -name '*.go' -not -path './vendor/*' | xargs goimports -l -w
auto_ci: clean fmt_proto fmt_shell protobuf mock swag
	@-find . -name '*.go' -not -path './vendor/*' | xargs gofmt -l -w -s
	@-${auto_fmt}
	@-find . -name '*.go' -not -path './vendor/*' | xargs gofmt -l -w -s
	@${auto_fmt}
	@git status
	@files=$$(git status -suno);if [ -n "$$files" ]; then \
		  git add *.go *.sh *.proto; \
		  git status; \
		  git commit -a -m "auto ci"; \
		  git push origin HEAD:$(branch); \
		  exit 1; \
		  fi;

.PHONY: update_chain33
update_chain33: ## make update_chain33 version=tag/branch/commit-hash
	if [ "${version}" = "" ]; then \
		go mod edit -require github.com/33cn/chain33@master; \
	else \
		go mod edit -require github.com/33cn/chain33@${version}; \
	fi; \
	go mod tidy

update_plugin: ## make update_plugin version=tag/branch/commit-hash
	if [ "${version}" = "" ]; then \
		go mod edit -require github.com/33cn/plugin@master; \
	else \
		go mod edit -require github.com/33cn/plugin@${version}; \
	fi; \
	go mod tidy

docker: build ## build docker image for externaldb with sync convert jrpc
	cp deploy/docker/*Dockerfile build
	cp deploy/ci/ci-*.sh build
	if [ "${version}" = "" ]; then \
		cd build && \
		docker build . -f sync.Dockerfile   -t chain33/externaldb/sync:lastest; \
		docker build . -f convert.Dockerfile -t chain33/externaldb/convert:lastest; \
		docker build . -f jrpc.Dockerfile -t chain33/externaldb/jrpc:lastest; \
		docker build . -f rpc.Dockerfile -t chain33/externaldb/rpc:lastest; \
		docker build . -f dummy-node.Dockerfile -t chain33/externaldb/dummy-node:lastest; \
		cd ..; \
	else \
		cd build && \
		docker build . -f sync.Dockerfile   -t chain33/externaldb/sync:${version}; \
		docker build . -f convert.Dockerfile -t chain33/externaldb/convert:${version}; \
		docker build . -f jrpc.Dockerfile -t chain33/externaldb/jrpc:${version}; \
		docker build . -f dummy-node.Dockerfile -t chain33/externaldb/dummy-node:${version}; \
		docker build . -f rpc.Dockerfile -t chain33/externaldb/rpc:${version}; \
		cd ..; \
	fi; 

# pkg: 老链路（node push → sync → ES → convert → jrpc/rpc）部署包 → build/externaldb.tgz
#   - 一套物料：四个程序全装，场景由 .env 的 EX_SERVICE_TYPE 决定（proof/browser/common），
#     不再按包区分；init 会把 proof-init 和 browser-init 都跑一遍（多跑没有副作用）
#   - ES 只保留 7.x：env/es7 = 7.17（默认），env/es7.4 = 7.4.2（老环境把 es7.4 换到 es7 即可）
#   - etc/mime.types 只有 rpc 文件下载接口用（按后缀设 Content-Type），跟着包一起发
#   - pkgProof / pkg-quickStart 是兼容别名，产物同为 externaldb.tgz
pkg:
	rm -rf build/externaldb
	mkdir -p build/externaldb/bin build/externaldb/etc build/externaldb/env/es7 build/externaldb/env/es7.4
	cp build/sync build/convert build/rpc build/jrpc build/externaldb/bin/
	cp config/externaldb.toml config/chain33.toml build/externaldb/etc/
	cp deploy/mime.types build/externaldb/etc/mime.types
	cp deploy/env/es7/docker-compose.yaml build/externaldb/env/es7/docker-compose.yaml
	cp deploy/env/es7.4/docker-compose.yaml build/externaldb/env/es7.4/docker-compose.yaml
	cp deploy/env/proof-init.bash build/externaldb/env/es7/proof-init.bash
	cp deploy/env/proof-init.bash build/externaldb/env/es7.4/proof-init.bash
	cp deploy/pkg/docker-compose.yaml deploy/pkg/docker-compose-update.yaml build/externaldb/
	cp deploy/pkg/Makefile deploy/pkg/README.md build/externaldb/
	cp deploy/pkg/alter-config.sh deploy/pkg/es-init.sh deploy/pkg/browser-init.bash deploy/pkg/reindex-shards.sh build/externaldb/
	cp .env build/externaldb/
	tar zcvf build/externaldb.tgz -C build externaldb

.PHONY: pkgProof pkg-quickStart
pkgProof pkg-quickStart: pkg

# pkg-pull: 直连（pull + seq）部署包 = pkg 的直连版，物料脚本与 pkg 共用（deploy/pkg、deploy/env）
#   - bin/：5 个常驻程序 pull_convert（pull+seq 展开）/ scanner（ERC20 扫块）/ jrpc（ES 查询）/
#     erc20api（ERC20 查询，走 MySQL）/ account_refresh（BTY 余额定时校正）
#   - tools/：离线/一次性工具（convert_check 对账，后续同类工具都放这里）
#   - 不含老链路 sync/convert/rpc
#   - 去掉 ES6；env/es7 = ES 7.17（默认，新部署用），包内另存 env/es7.4（ES 7.4.2）备用，
#     老环境要用 7.4 时，把 env/es7.4 的内容换到 env/es7 即可（不改脚本、不加版本配置）
#   - 不带 ex-merge（sync_convert 合并模式）
#   - compose 起 convert + jrpc；[mysql] 启用时再起 scanner + erc20api
pkg-pull:
	rm -rf build/externaldb-pull
	mkdir -p build/externaldb-pull/bin build/externaldb-pull/tools build/externaldb-pull/etc build/externaldb-pull/env/es7 build/externaldb-pull/env/es7.4 build/externaldb-pull/env/mysql/sql
	cp build/pull_convert build/externaldb-pull/bin/pull_convert
	cp build/erc20scan build/externaldb-pull/bin/scanner
	cp build/jrpc build/externaldb-pull/bin/jrpc
	cp build/erc20api build/externaldb-pull/bin/erc20api
	cp build/account_refresh build/externaldb-pull/bin/account_refresh
	cp build/convert_check build/externaldb-pull/tools/
	cp config/externaldb.toml build/externaldb-pull/etc/
	cp config/chain33.toml build/externaldb-pull/etc/
	cp deploy/env/es7/docker-compose.yaml build/externaldb-pull/env/es7/docker-compose.yaml
	cp deploy/env/proof-init.bash build/externaldb-pull/env/es7/proof-init.bash
	cp deploy/env/es7.4/docker-compose.yaml build/externaldb-pull/env/es7.4/docker-compose.yaml
	cp deploy/env/proof-init.bash build/externaldb-pull/env/es7.4/proof-init.bash
	cp deploy/pull-convert/mysql.yaml build/externaldb-pull/env/mysql/docker-compose.yaml
	cp deploy/pull-convert/mysql-init.sh build/externaldb-pull/
	cp erc20scaner/database/schema.sql build/externaldb-pull/env/mysql/sql/schema.sql
	cp deploy/pull-convert/docker-compose.yaml build/externaldb-pull/
	cp deploy/pull-convert/Makefile build/externaldb-pull/
	cp deploy/pull-convert/README.md build/externaldb-pull/
	cp deploy/pkg/reindex-shards.sh build/externaldb-pull/
	cp deploy/pkg/alter-config.sh build/externaldb-pull/
	cp deploy/pkg/es-init.sh build/externaldb-pull/
	cp deploy/pkg/browser-init.bash build/externaldb-pull/
	cp .env build/externaldb-pull/
	tar zcvf build/externaldb-pull.tgz -C build externaldb-pull

docker-compose: ## run docker-compose as jenkins-ci
	@cd build && \
		if ! [ -d ci ]; then \
			make -C .. docker; \
			mkdir ci/configs -p; \
		fi; \
		cp ../deploy/docker/docker-compose-ci.yaml ./ci/docker-compose.yaml; \
		cp ../deploy/ci/ci-*sh ./ci/; \
		cp ../deploy/ci/chain33.para.6.3.toml ./ci/configs/chain33.toml; \
		cp *toml ./ci/configs; \
		cd ci && \
		./ci-setup-config.sh; \
		./ci-test.sh \
		cd ..; \
	cd ..

docker-compose-down: ## stop ci 
	@cd build && if [ -d ci ]; then \
	 	cp ../deploy/docker/docker-compose-ci.yaml ./ci/docker-compose.yaml; \
	 	cd ci/ && \
		 ./ci-setup-config.sh; \
		 docker-compose down && \
	  	cd .. ; \
		rm ci -rf; \
	 fi; \
	 cd ..
