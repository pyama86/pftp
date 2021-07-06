TEST ?= $(shell $(GO) list ./... | grep -v vendor)
VERSION = $(shell cat version)
REVISION = $(shell git describe --always)

INFO_COLOR=\033[1;34m
RESET=\033[0m
BOLD=\033[1m
ifeq ("$(shell uname)","Darwin")
GO ?= GO111MODULE=on go
else
GO ?= GO111MODULE=on /usr/local/go/bin/go
endif

default: build
ci: depsdev ftp test lint integration ## Run test and more...

depsdev: ## Installing dependencies for development
	$(GO) get golang.org/x/lint/golint

test: ## Run test
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Testing$(RESET)"
	$(GO) test -v $(TEST) -timeout=5s -parallel=4
	$(GO) test -race $(TEST)

lint: ## Exec golint
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Linting$(RESET)"
	golint -min_confidence 1.1 -set_exit_status $(TEST)

server: ## Run server with gin
	$(GO) run main.go

build: ## Build as linux binary
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Building$(RESET)"
	$(GO) build -o pftp_bin main.go

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "$(INFO_COLOR)%-30s$(RESET) %s\n", $$1, $$2}'

vsftpd: vsftpd-cleanup
	docker build -t vsftpd-server:test -f Dockerfile-vsftpd ./
	docker run -d -v "`pwd`/misc/test/data":/home/vsftpd \
	-p 10021:21 -p 11100-11150:11100-11150 \
	--name vsftpd --restart=always vsftpd-server:test

vsftpd-cleanup:
	docker rm -f vsftpd | true

proftpd: proftpd-cleanup
	docker build -t proftpd-server:test -f Dockerfile-proftpd ./
	docker run -d -v "`pwd`/misc/test/data/prouser":/home/prouser \
	-v "`pwd`/tls/server.crt":/etc/ssl/certs/proftpd.crt \
	-v "`pwd`/tls/server.key":/etc/ssl/private/proftpd.key \
	-v "`pwd`/tls/server.crt":/etc/ssl/certs/chain.crt \
	-p 20021:21 -p 21100-21150:21100-21150 \
	--name proftpd --restart=always proftpd-server:test
proftpd-cleanup:
	docker rm -f proftpd | true

baseftp: baseftp-cleanup
	docker build -t baseftp-server:test -f Dockerfile-base ./
	docker run -d \
	-v "`pwd`/tls/server.crt":/etc/ssl/certs/proftpd.crt \
	-v "`pwd`/tls/server.key":/etc/ssl/private/proftpd.key \
	-v "`pwd`/tls/server.crt":/etc/ssl/certs/chain.crt \
	-p 21:21 -p 31100-31150:31100-31150 \
	--name baseftp --restart=always baseftp-server:test
baseftp-cleanup:
	docker rm -f baseftp | true

ftp: baseftp vsftpd proftpd

ftp-cleanup: baseftp-cleanup vsftpd-cleanup proftpd-cleanup

integration:
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Integration Testing$(RESET)"
	./misc/server stop || true
	./misc/server start
	$(GO) test $(VERBOSE) -timeout=300s -integration $(TEST) $(TEST_OPTIONS)
	./misc/server stop

.PHONY: default dist test ftp proftpd vsftpd help build server
