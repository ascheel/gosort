CMD=go build

VERSION=`cat version.txt`
LDFLAGS=-ldflags "-w -s -X main.Version=${VERSION}"

API_BINARY=api
CLIENT_BINARY=client

build:
	@echo ""
	@echo "Building Linux x86_64"
	GOOS=linux GOARCH=amd64 ${CMD} ${LDFLAGS} -o bin/${API_BINARY}-linux-amd64 ./cmd/api
	GOOS=linux GOARCH=amd64 ${CMD} ${LDFLAGS} -o bin/${CLIENT_BINARY}-linux-amd64 ./cmd/client
	@cp -v config.yml bin/config.yml

clean:
	rm -rfv bin
