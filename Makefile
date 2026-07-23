# Rune build and quality targets. The -jit targets exercise the cgo
# LuaJIT backend and need LuaJIT installed; see docs/luajit.md.

GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -s -w -X github.com/mmcdole/rune/version.Number=$(VERSION)

.PHONY: build build-jit test test-jit check bench clean

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/rune ./cmd/rune

build-jit:
	$(GO) build -tags luajit -trimpath -ldflags "$(LDFLAGS)" -o bin/rune-jit ./cmd/rune

test:
	$(GO) test -race -shuffle=on ./...

test-jit:
	$(GO) test -tags luajit -race -shuffle=on ./lua/ ./script/...

# Mirrors CI's quality gates plus the tagged vet; run before pushing.
check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	$(GO) vet ./...
	$(GO) vet -tags luajit ./...
	$(GO) run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...

# Cross-backend comparison of script throughput.
bench:
	$(GO) test ./lua/ -run '^$$' -bench EngineScriptWork -benchtime=2s
	$(GO) test -tags luajit ./lua/ -run '^$$' -bench EngineScriptWork -benchtime=2s

clean:
	rm -rf bin/
