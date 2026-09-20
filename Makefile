# go 不在 PATH 时自动回退到常见安装位置；也可显式指定：make dist GO=/path/to/go
GO ?= $(shell command -v go 2>/dev/null || echo /usr/local/go/bin/go)
BIN ?= nacos-cli
DIST := dist

# 本地构建时把 git 描述注入 --version，例如 v1.0.0-3-g58b7a3c
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build dist test vet fmt install clean

all: build

# 编译到当前目录
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

# 交叉编译三个常用平台，产物在 dist/
dist:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)_linux_amd64 .
	CGO_ENABLED=0 GOOS=linux  GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)_linux_arm64 .
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)_darwin_arm64 .
	@ls -lh $(DIST)

test:
	$(GO) test -race -count=1 ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

# 装到本机
install: build
	install -m 0755 $(BIN) /usr/local/bin/$(BIN)

clean:
	rm -rf $(DIST) $(BIN)
