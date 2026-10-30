# Never let the go command download a different toolchain.
export GOTOOLCHAIN := local

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
BIN     := bin/vogter
TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all build check fmt vet test race example cross dist notices clean

all: check build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/vogter

# fmt fails if any file needs formatting; it never rewrites files.
fmt:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

check: fmt vet race

# configs/config.example.toml is generated from the init template, which is
# the single source of truth; a test fails if the two drift apart.
example: build
	rm -f configs/config.example.toml
	./$(BIN) -c configs/config.example.toml init

cross:
	@for t in $(TARGETS); do \
	  os=$${t%/*}; arch=$${t#*/}; ext=; [ $$os = windows ] && ext=.exe; \
	  echo "building $$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' \
	    -o dist/vogter-$(VERSION)-$$os-$$arch$$ext ./cmd/vogter || exit 1; \
	done

# Release archives: binary, licences, README and the launcher entry.
dist: cross notices
	@cd dist && for f in vogter-$(VERSION)-*; do \
	  case $$f in *.tar.gz|*.zip|*.sha256) continue;; esac; \
	  d=$${f%.exe}; mkdir -p "$$d.pkg"; \
	  cp "$$f" "$$d.pkg/vogter$${f##$$d}"; \
	  cp ../README.md ../LICENSE ../NOTICE ../THIRD_PARTY_NOTICES.txt ../packaging/vogter.desktop "$$d.pkg/"; \
	  tar czf "$$d.tar.gz" --transform "s,^$$d.pkg,$$d," "$$d.pkg"; \
	  rm -rf "$$d.pkg"; \
	done
	cd dist && sha256sum *.tar.gz > SHA256SUMS

notices:
	tools/notices.sh > THIRD_PARTY_NOTICES.txt

clean:
	rm -rf bin dist
