CMD=go build

VERSION=`cat version.txt`
LDFLAGS=-ldflags "-w -s -X main.Version=${VERSION}"

SORT_BINARY=gosort
METADATA_BINARY=metadata

build:
	@echo ""
	@echo "Building Linux x86_64"
	GOOS=linux GOARCH=amd64 ${CMD} ${LDFLAGS} -o bin/${SORT_BINARY}-linux-amd64 ./cmd/gosort
	GOOS=linux GOARCH=amd64 ${CMD} ${LDFLAGS} -o bin/${METADATA_BINARY}-linux-amd64 ./cmd/metadata

clean:
	rm -rfv bin
