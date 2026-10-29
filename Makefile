# Dependency source, chosen at build time:
#   make build               -> DEPS=vendor (default): vendor/ only, no network access
#   make build DEPS=online   -> modules downloaded from the Go proxy (proxy.golang.org
#                               or $$GOPROXY), checked against go.sum; vendor/ is ignored
DEPS ?= vendor
BINARY  := vmware_exporter
VERSION ?= $(shell cat VERSION)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo local)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
ifeq ($(DEPS),vendor)
MODFLAG := -mod=vendor
else ifeq ($(DEPS),online)
MODFLAG := -mod=readonly
else
$(error DEPS must be "vendor" or "online", got "$(DEPS)")
endif
GOFLAGS := $(MODFLAG) -trimpath

.PHONY: build build-windows promu promu-tarball release-tarballs test lint check-rules test-rules sim vendor clean

build:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BINARY) ./cmd/vmware_exporter

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BINARY).exe ./cmd/vmware_exporter

# Same build through promu (https://github.com/prometheus/promu), configured in .promu.yml.
# Produces ./vmware_exporter; DEPS applies as above.
promu:
	GOFLAGS=$(MODFLAG) promu build

# Release tarball (binary + README + example config + deploy/) in dist/
promu-tarball: promu
	promu tarball --prefix dist .

# Release archives for several platforms (promu, no Docker needed) + checksums in .tarballs/
PLATFORMS ?= linux/amd64 linux/arm64 windows/amd64
release-tarballs:
	rm -rf .build .tarballs
	for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		GOOS=$$os GOARCH=$$arch GOFLAGS=$(MODFLAG) promu build --prefix .build/$$os-$$arch || exit 1; \
		GOOS=$$os GOARCH=$$arch promu tarball --prefix .tarballs .build/$$os-$$arch || exit 1; \
	done
	cd .tarballs && sha256sum * > sha256sums.txt

test:
	go test $(GOFLAGS) ./...

lint:
	go vet $(GOFLAGS) ./...
	test -z "$$(gofmt -l cmd internal hack)"

# Requires promtool (shipped with Prometheus)
check-rules:
	promtool check rules deploy/prometheus/vmware-alerts.yml deploy/prometheus/pryorda-alerts.yml

test-rules:
	cd deploy/prometheus && promtool test rules vmware-alerts.test.yml pryorda-alerts.test.yml

# Simulated vCenter for testing without infrastructure (VERSION_SIM=8.0.3 API_SIM=8.0.3.0)
VERSION_SIM ?= 9.0.0
API_SIM     ?= 9.0.0.0
sim:
	go run $(GOFLAGS) ./hack/vcsim -version $(VERSION_SIM) -api $(API_SIM) -l 127.0.0.1:8989

# Refresh vendor/ after changing dependencies in go.mod (needs network access)
vendor:
	go mod tidy
	go mod vendor

clean:
	rm -rf dist vmware_exporter .build .tarballs
