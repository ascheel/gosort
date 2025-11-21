CMD=go build

VERSION=$(shell cat version.txt)
LDFLAGS=-ldflags "-w -s -X main.Version=${VERSION}"

API_BINARY=api
CLIENT_BINARY=client

# Target-specific variables
buildlinux: TARGET_ARCH := amd64
buildlinux: TARGET_GOOS := linux
buildlinuxarm32: TARGET_ARCH := arm
buildlinuxarm32: TARGET_GOOS := linux
buildlinuxarm64: TARGET_ARCH := arm64
buildlinuxarm64: TARGET_GOOS := linux
buildwin: TARGET_ARCH := amd64
buildwin: TARGET_GOOS := windows

# Common build function
define BUILD_TARGET
	@echo ""
	@echo "Building $(1) $(2) version ${VERSION}"
	GOOS=${TARGET_GOOS} GOARCH=${TARGET_ARCH} CGO_ENABLED=0 ${CMD} ${LDFLAGS} -o bin/gosort_$(1).${VERSION}-${TARGET_ARCH}/${API_BINARY}$(3) ./cmd/api
	GOOS=${TARGET_GOOS} GOARCH=${TARGET_ARCH} CGO_ENABLED=0 ${CMD} ${LDFLAGS} -o bin/gosort_$(1).${VERSION}-${TARGET_ARCH}/${CLIENT_BINARY}$(3) ./cmd/client
	@cp -v config.yml.example bin/gosort_$(1).${VERSION}-${TARGET_ARCH}/config.yml.example
	@tar cvzf bin/gosort_$(1).${VERSION}-${TARGET_ARCH}.tar.gz -C bin gosort_$(1).${VERSION}-${TARGET_ARCH}
endef

all:
	make buildlinux
	make buildwin
	make buildlinuxarm32
	make buildlinuxarm64

buildlinux:
	$(call BUILD_TARGET,linux,x86_64,)

buildlinuxarm32:
	$(call BUILD_TARGET,linux,arm32,)

buildlinuxarm64:
	$(call BUILD_TARGET,linux,arm64,)

buildwin:
	$(call BUILD_TARGET,win,x86_64,.exe)

package:
	make build
	@echo ""
	@echo "Packaging Gosort ${VERSION}"
	@cp -r bin gosort
	@mv -v gosort/config.yml gosort/config.yml.example
	@tar cvzf gosort.${VERSION}.tar.gz gosort
	@echo "Package built!"
	@rm -rf gosort

clean:
	rm -rfv bin
