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
ci: depsdev ftp test lint integration graceful_shutdown ## Run test and more...

depsdev: ## Installing dependencies for development
	$(GO) get github.com/golang/lint/golint
	$(GO) get -u github.com/tcnksm/ghr
	$(GO) get github.com/mitchellh/gox

test: ## Run test
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Testing$(RESET)"
	$(GO) test -v $(TEST) -timeout=5s -parallel=4
	$(GO) test -race $(TEST)

vet: ## Exec $(GO) vet
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Vetting$(RESET)"
	$(GO) vet $(TEST)

lint: ## Exec golint
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Linting$(RESET)"
	golint -min_confidence 1.1 -set_exit_status $(TEST)

server: ## Run server with gin
	$(GO) run main.go

build: ## Build as linux binary
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Building$(RESET)"
	$(GO) build -o pftp_bin main.go


ghr: ## Upload to Github releases without token check
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Releasing for Github$(RESET)"
	ghr -u pyama86 v$(VERSION)-$(REVISION) pkg

dist: build ## Upload to Github releases
	@test -z $(GITHUB_TOKEN) || test -z $(GITHUB_API) || $(MAKE) ghr

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "$(INFO_COLOR)%-30s$(RESET) %s\n", $$1, $$2}'

vsftpd: vsftpd-cleanup
	docker build -t vsftpd-server:test -f Dockerfile-vsftpd ./
	docker run -d -v "`pwd`/misc/test/data":/home/vsftpd \
	-p 10020-10021:20-21 -p 11100-11110:11100-11110 \
	-e FTP_USER=vsuser -e FTP_PASS=vsuser \
	-e PASV_ADDRESS=127.0.0.1 -e PASV_MIN_PORT=11100 -e PASV_MAX_PORT=11110 \
	--name vsftpd --restart=always vsftpd-server:test

vsftpd-cleanup:
	docker rm -f vsftpd | true

proftpd: proftpd-cleanup
	docker build -t proftpd-server:test -f Dockerfile-proftpd ./
	docker run -d -v "`pwd`/misc/test/data/prouser":/home/prouser \
	-p 20-21:20-21 -p 21100-21110:21100-21110 \
	--name proftpd --restart=always proftpd-server:test

proftpd-cleanup:
	docker rm -f proftpd | true

ftp: vsftpd proftpd

ftp-cleanup: vsftpd-cleanup proftpd-cleanup

integration:
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Integration Testing$(RESET)"
	./misc/server stop || true
	./misc/server start
	$(GO) test $(VERBOSE) -integration $(TEST) $(TEST_OPTIONS)
	./misc/server stop
graceful_shutdown:
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Shutdown Testing$(RESET)"
	SERVER_STARTER_PORT="127.0.0.1:2121=testfd"
	./misc/server stop || true
	./misc/server start
	./misc/server hup

.PHONY: default dist test
