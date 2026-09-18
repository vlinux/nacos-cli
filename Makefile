# go 不在 PATH 时自动回退到常见安装位置；也可显式指定：make dist GO=/path/to/go
GO ?= $(shell command -v go 2>/dev/null || echo /usr/local/go/bin/go)
BIN ?= nacos-cli
DIST := dist
LDFLAGS := -s -w

.PHONY: all build test vet dist clean install

all: build

# 本机编译
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

# 交叉编译三个常用平台，产物在 dist/
dist:
	@mkdir -p $(DIST)
	GOOS=linux   GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)_linux_amd64 .
	GOOS=linux   GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)_linux_arm64 .
	GOOS=darwin  GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)_darwin_arm64 .
	@ls -lh $(DIST)

test:
	$(GO) test -race -count=1 ./...

vet:
	$(GO) vet ./...

# 装到本机
install: build
	install -m 0755 $(BIN) /usr/local/bin/$(BIN)

clean:
	rm -rf $(DIST) $(BIN)
