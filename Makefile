BRANCH=`git rev-parse --abbrev-ref HEAD`
COMMIT=`git rev-parse --short HEAD`
CFLAGS=`llvm-config --cflags`
LDFLAGS="`llvm-config --ldflags` -Wl,-L`llvm-config --libdir` -lLLVM-`llvm-config --version`"
GOLDFLAGS="-X main.branch $(BRANCH) -X main.commit $(COMMIT)"
COVERPROFILE=/tmp/c.out
TEST=.
PKG=./...
GO=/usr/local/go/bin/go

REPO_OWNER=github.com/skydb
APP_NAME=sky
APP_REPO=$(REPO_OWNER)/$(APP_NAME)
CWD=$(shell pwd)
export GOBIN=$(GOPATH)/bin

default: build

bench: grammar
	CGO_CFLAGS=$(CFLAGS) CGO_LDFLAGS=$(LDFLAGS) $(GO) test -v -test.bench=. $(PKG)

build: grammar env get
	CGO_CFLAGS=$(CFLAGS) CGO_LDFLAGS=$(LDFLAGS) $(GO) build -ldflags=$(GOLDFLAGS) -a -o bin/skyd ./cmd/skyd/main.go

cover: fmt
	CGO_CFLAGS=$(CFLAGS) CGO_LDFLAGS=$(LDFLAGS) $(GO) test -v -test.run=$(TEST) -coverprofile=$(COVERPROFILE) $(PKG)
	go tool cover -html=$(COVERPROFILE) -o=coverage.html
	rm $(COVERPROFILE)

env:
	@echo "CGO_CFLAGS=$(CFLAGS) CGO_LDFLAGS=$(LDFLAGS)"
	@echo "GOPATH=$(GOPATH) GOBIN=$(GOBIN)"
	mkdir -p $(GOBIN)
	mkdir -p $(GOPATH)/src/$(REPO_OWNER)
	[ -d $(GOPATH)/src/$(APP_REPO) ] || ln -sfv $(CWD) $(GOPATH)/src/$(APP_REPO)

fmt:
	go fmt ./...

get: env
	$(GO) get github.com/stretchr/testify
	CGO_CFLAGS=$(CFLAGS) CGO_LDFLAGS=$(LDFLAGS) $(GO) get github.com/axw/gollvm/llvm
	CGO_CFLAGS=$(CFLAGS) CGO_LDFLAGS=$(LDFLAGS) $(GO) get -d ./cmd/... ./db/... ./hash/... ./query/... ./server/...

grammar:
	${MAKE} -C query/parser

install: build
	mv bin/skyd /usr/local/bin/skyd

run: grammar
	CGO_CFLAGS=$(CFLAGS) CGO_LDFLAGS=$(LDFLAGS) $(GO) run -ldflags=$(GOLDFLAGS) ./cmd/skyd/main.go

test: grammar
	CGO_CFLAGS=$(CFLAGS) CGO_LDFLAGS=$(LDFLAGS) $(GO) test -v -test.run=$(TEST) $(PKG)

clean:
	rm -Rf .go/

.PHONY: default bench build cover env fmt get grammar run test
