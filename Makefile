TEST ?= $(shell go list ./... | grep -v vendor)
VERSION = $(shell cat version)
REVISION = $(shell git describe --always)

INFO_COLOR=\033[1;34m
RESET=\033[0m
BOLD=\033[1m

default: build
#ci: depsdev test vet lint ## Run test and more...
ci: depsdev ftp test lint integration ## Run test and more...

deps: ## Install dependencies
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Installing Dependencies$(RESET)"
	go get -u github.com/golang/dep/...
	dep ensure

depsdev: deps ## Installing dependencies for development
	go get github.com/golang/lint/golint
	go get -u github.com/tcnksm/ghr
	go get github.com/mitchellh/gox

test: ## Run test
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Testing$(RESET)"
	go test -v $(TEST) -timeout=30s -parallel=4
	go test -race $(TEST)

vet: ## Exec go vet
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Vetting$(RESET)"
	go vet $(TEST)

lint: ## Exec golint
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Linting$(RESET)"
	golint -min_confidence 1.1 -set_exit_status $(TEST)

server: ## Run server with gin
	go run main.go

build: ## Build as linux binary
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Building$(RESET)"
	./misc/build $(VERSION) $(REVISION)

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
	./misc/server start
	go test $(VERBOSE) -integration $(TEST) $(TEST_OPTIONS)
	./misc/server stop
.PHONY: default dist test deps
