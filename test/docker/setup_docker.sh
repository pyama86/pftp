#!/bin/bash

apt-get update
apt-get -y upgrade
apt-get install -y ca-certificates openssh-client build-essential make libssl-dev libghc-zlib-dev libcurl4-gnutls-dev libexpat1-dev gettext unzip

# set up nsswitch.conf for Go's "netgo" implementation (which Docker explicitly uses)
# - https://github.com/docker/docker-ce/blob/v17.09.0-ce/components/engine/hack/make.sh#L149
# - https://github.com/golang/go/blob/go1.9.1/src/net/conf.go#L194-L275
# - docker run --rm debian:stretch grep '^hosts:' /etc/nsswitch.conf
[ ! -e /etc/nsswitch.conf ] && echo 'hosts: files dns' > /etc/nsswitch.conf || echo 'do nothing'

DOCKER_VERSION=20.10.5
# TODO ENV DOCKER_SHA256
# https://github.com/docker/docker-ce/blob/5b073ee2cf564edee5adca05eee574142f7627bb/components/packaging/static/hash_files !!
# (no SHA file artifacts on download.docker.com yet as of 2017-06-07 though)

set -eux; \
	\
	Arch="$(uname -m)"; \
	case "$Arch" in \
		'x86_64') \
			url="https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VERSION}.tgz"; \
			;; \
		'armhf') \
			url="https://download.docker.com/linux/static/stable/armel/docker-${DOCKER_VERSION}.tgz"; \
			;; \
		'armv7') \
			url="https://download.docker.com/linux/static/stable/armhf/docker-${DOCKER_VERSION}.tgz"; \
			;; \
		'aarch64') \
			url="https://download.docker.com/linux/static/stable/aarch64/docker-${DOCKER_VERSION}.tgz"; \
			;; \
		*) echo >&2 "error: unsupported architecture ($apkArch)"; exit 1 ;; \
	esac; \
	\
	wget -O docker.tgz "$url"; \
	\
	tar --extract \
		--file docker.tgz \
		--strip-components 1 \
		--directory /usr/local/bin/ \
	; \
	rm docker.tgz; \
	\
	dockerd --version; \
	docker --version

export DOCKER_TLS_CERTDIR=/certs
mkdir /certs /certs/client && chmod 1777 /certs /certs/client
