GITHUB_DIR=${GOPATH}/src/github.com/chenglch/
REPO_DIR=${GOPATH}/src/github.com/chenglch/goconserver
CURRENT_DIR=$(shell pwd)
REPO_DIR_LINK=$(shell readlink -f ${REPO_DIR})
SERVER_CONF_FILE=/etc/goconserver/server.conf
CLIENT_CONF_FILE=~/congo.sh
SERVER_BINARY=goconserver
CLIENT_BINARY=congo
COMMIT=$(shell git rev-parse HEAD)
VERSION=0.1
BUILD_TIME=`date +%FT%T%z`
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.Commit=${COMMIT}"

all: build
deps:
	go get github.com/Masterminds/glide
	glide install

link: deps
	REPO_DIR=${REPO_DIR}; \
	REPO_DIR_LINK=${REPO_DIR_LINK}; \
	CURRENT_DIR=${CURRENT_DIR}; \
	GITHUB_DIR=${GITHUB_DIR}; \
	if [ "$${REPO_DIR_LINK}" != "$${CURRENT_DIR}" ]; then \
		echo "Fixing symlinks for build"; \
		rm -rf $${REPO_DIR}; \
		mkdir -p $${GITHUB_DIR}; \
		ln -s $${CURRENT_DIR} $${REPO_DIR}; \
	fi
	
fmt:
	go fmt $$(go list ./... | grep -v /vendor/)

build: link
	cd ${REPO_DIR}; \
	go build ${LDFLAGS} -o ${SERVER_BINARY} goconserver.go; \
	go build ${LDFLAGS} -o ${CLIENT_BINARY} cmd/congo.go; \
	cd -

install: build
	cp ${SERVER_BINARY} /usr/local/bin/${SERVER_BINARY}
	cp ${CLIENT_BINARY} /usr/local/bin/${CLIENT_BINARY}
	mkdir -p /etc/goconserver /var/log/goconserver/nodes /var/lib/goconserver
	chmod 700 /etc/goconserver /var/log/goconserver/nodes /var/lib/goconserver
        
	if [ ! -f "/etc/goconserver/server.conf" ];  then \
		cp etc/goconserver/server.conf /etc/goconserver/; \
	fi;
	if [ ! -f "/etc/profile.d/congo.sh" ]; then \
		cp etc/goconserver/client.sh /etc/profile.d/congo.sh; \
	fi

clean:
	rm -f ${SERVER_BINARY}
	rm -f ${CLIENT_BINARY}

.PHONY: binary deps fmt build clean link
