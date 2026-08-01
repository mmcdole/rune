# Rune build and quality targets. The -jit targets exercise the cgo
# LuaJIT backend and need LuaJIT installed; see docs/luajit.md.

GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -s -w -X github.com/mmcdole/rune/version.Number=$(VERSION)

.PHONY: build build-jit test test-jit check bench clean require-luajit

# Preflight for the tagged targets. Without the headers cgo fails with a
# bare "lua.h: No such file or directory"; say what to install instead.
# The probed paths mirror script/luajit/link_*.go, with pkg-config as the
# fallback those files use on other platforms.
require-luajit:
	@test -e /usr/include/luajit-2.1/lua.h \
	  || test -e /opt/homebrew/include/luajit-2.1/lua.h \
	  || test -e script/luajit/vendor_luajit/include/lua.h \
	  || pkg-config --exists luajit 2>/dev/null \
	  || { \
	    echo "LuaJIT headers not found. Install LuaJIT 2.1, then retry:"; \
	    echo "  Debian/Ubuntu  sudo apt-get install libluajit-5.1-dev"; \
	    echo "  Fedora         sudo dnf install luajit-devel"; \
	    echo "  Arch           sudo pacman -S luajit"; \
	    echo "  macOS          brew install luajit"; \
	    echo "  Windows        .github/actions/setup-luajit-windows"; \
	    echo "See docs/luajit.md."; \
	    exit 1; \
	  }

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/rune ./cmd/rune

build-jit: require-luajit
	$(GO) build -tags luajit -trimpath -ldflags "$(LDFLAGS)" -o bin/rune-jit ./cmd/rune

test:
	$(GO) test -race -shuffle=on ./...

test-jit: require-luajit
	$(GO) test -tags luajit -race -shuffle=on ./...

# Mirrors CI's quality gates plus the tagged vet; run before pushing.
check: require-luajit
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	$(GO) vet ./...
	$(GO) vet -tags luajit ./...
	$(GO) run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...

# Cross-backend comparison of script throughput.
bench: require-luajit
	$(GO) test ./lua/ -run '^$$' -bench EngineScriptWork -benchtime=2s
	$(GO) test -tags luajit ./lua/ -run '^$$' -bench EngineScriptWork -benchtime=2s

clean:
	rm -rf bin/
