CGO_ENABLED ?= 0
BINARY := xuanchu
CMD := ./cmd/xuanchu
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "")
LDFLAGS := $(if $(VERSION),-ldflags "-X main.version=$(VERSION)",)

GOOS   ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

DIST_DIR := dist
WEB_DIR := web

.PHONY: all build build-linux build-darwin build-release \
        test lint vet clean install \
        web-console-build web-console-check web-console-dev \
        $(BINARY)

all: build

build: $(BINARY)

$(BINARY):
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build $(LDFLAGS) -o $(BINARY) $(CMD)

build-linux: ## 交叉编译 Linux amd64（部署用）
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=amd64 \
		go build $(LDFLAGS) -o $(BINARY) $(CMD)

build-darwin: ## 本地 macOS 构建
	CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin GOARCH=arm64 \
		go build $(LDFLAGS) -o $(BINARY) $(CMD)

web-console-build: ## 构建嵌入式 Web Admin Console dist
	cd $(WEB_DIR) && pnpm build

web-console-check: ## 检查 Web Admin Console 前端
	cd $(WEB_DIR) && pnpm test && pnpm typecheck

web-console-dev: ## 启动 Web Admin Console 开发服务器
	cd $(WEB_DIR) && pnpm dev

build-release: web-console-build ## 发布构建：linux/amd64 + linux/arm64 + darwin/arm64
	@mkdir -p $(DIST_DIR)
	@set -e; \
	for target in linux/amd64 linux/arm64 darwin/arm64; do \
		GOOS=$${target%/*} GOARCH=$${target#*/}; \
		echo "Building $$GOOS/$$GOARCH..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$$GOOS GOARCH=$$GOARCH \
			go build $(LDFLAGS) \
			-o "$(DIST_DIR)/$(BINARY)-$${GOOS}-$${GOARCH}" $(CMD); \
	done
	@echo "Release binaries in $(DIST_DIR)/"

test: ## 跑全部测试
	CGO_ENABLED=$(CGO_ENABLED) go test ./... -count=1

test-race: ## 竞态检测
	CGO_ENABLED=1 go test ./... -race -count=1

lint: vet
	@which golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

vet:
	go vet ./...

install: build
	mv $(BINARY) $(shell go env GOPATH)/bin/$(BINARY)

clean:
	rm -f $(BINARY)
	rm -rf $(DIST_DIR)
