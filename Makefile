# Never let the go command download a different toolchain.
export GOTOOLCHAIN := local

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
BIN     := bin/vaegter
# Names, not Go values: armv7, armel, mipsel and mips64el are mapped in `cross`.
TARGETS := linux/amd64 linux/arm64 linux/armv7 linux/armel linux/ppc64 linux/mipsel linux/mips64el \
           darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: all build check fmt vet test race example cross dist notices clean

all: check build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/vaegter

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
	  goarch=$$arch; goarm=; gomips=; gomips64=; \
	  case $$arch in \
	    armv7) goarch=arm; goarm=7;; \
	    armel) goarch=arm; goarm=5;; \
	    mipsel) goarch=mipsle; gomips=softfloat;; \
	    mips64el) goarch=mips64le; gomips64=softfloat;; \
	  esac; \
	  echo "building $$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$goarch GOARM=$$goarm GOMIPS=$$gomips GOMIPS64=$$gomips64 go build -trimpath -ldflags '$(LDFLAGS)' \
	    -o dist/vaegter-$(VERSION)-$$os-$$arch$$ext ./cmd/vaegter || exit 1; \
	done

# Release files: bare binaries, which embed the licences, and archives with the
# binary, licences, README and launcher entry. Notices come first so that the
# binaries embed the current text.
dist: notices cross
	@cd dist && for f in vaegter-$(VERSION)-*; do \
	  case $$f in *.tar.gz|*.zip|*.sha256) continue;; esac; \
	  d=$${f%.exe}; mkdir -p "$$d.pkg"; \
	  cp "$$f" "$$d.pkg/vaegter$${f##$$d}"; \
	  cp ../README.md ../LICENSE ../NOTICE ../THIRD_PARTY_NOTICES.txt ../packaging/vaegter.desktop "$$d.pkg/"; \
	  tar czf "$$d.tar.gz" --transform "s,^$$d.pkg,$$d," "$$d.pkg"; \
	  rm -rf "$$d.pkg"; \
	done
	cd dist && sha256sum vaegter-* > SHA256SUMS

notices:
	tools/notices.sh > THIRD_PARTY_NOTICES.txt

clean:
	rm -rf bin dist
