all: build
.PHONY: all build

BINARY_NAME := synctree 
BUILD_IMAGE ?= synctree
PUSH_IMAGE ?= synctree:v0.0.1

VERSION := $(shell git rev-parse --short HEAD)
BUILDTIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

GOLDFLAGS += -X 'main.BuildVersion=$(VERSION)'
GOLDFLAGS += -X 'main.BuildTime=$(BUILDTIME)'

build:
	@CGO_ENABLED=0 go build -ldflags="-s -w $(GOLDFLAGS)" -o ./bin/$(BINARY_NAME) ./cmd/main.go

container:
	docker build -t $(BUILD_IMAGE) .

push:
	docker tag $(BUILD_IMAGE) $(PUSH_IMAGE) 
	docker push $(BUILD_IMAGE)
	docker push $(PUSH_IMAGE)

coverage:
	./buildtools/coverage.sh

test: 
	@cd pkg/abac; go test -v --race
	@cd pkg/core; go test -v --race
	@cd pkg/lseq; go test -v --race
	@cd pkg/utils; go test -v --race
	@cd pkg/random; go test -v --race
	@cd pkg/vectorclock; go test -v --race
	@cd pkg/crdt; go test -v --race
	@cd pkg/securecrdt; go test -v --race
	@cd pkg/security/crypto; go test -v --race
	@cd internal/crypto; go test -v --race

install:
	cp ./bin/$(BINARY_NAME) /usr/local/bin
