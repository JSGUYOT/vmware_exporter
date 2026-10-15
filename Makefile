# Construction hors ligne : toutes les dépendances sont dans vendor/.
BINARY  := vmware-exporter
VERSION ?= 1.0.0
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo local)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
GOFLAGS := -mod=vendor -trimpath

.PHONY: build build-windows test lint check-rules test-rules sim clean

build:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BINARY) ./cmd/vmware-exporter

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BINARY).exe ./cmd/vmware-exporter

test:
	go test $(GOFLAGS) ./...

lint:
	go vet $(GOFLAGS) ./...
	test -z "$$(gofmt -l cmd internal hack)"

# Nécessite promtool (fourni avec Prometheus)
check-rules:
	promtool check rules deploy/prometheus/vmware-alerts.yml

test-rules:
	cd deploy/prometheus && promtool test rules vmware-alerts.test.yml

# vCenter simulé pour tester sans infrastructure (VERSION_SIM=8.0.3 API_SIM=8.0.3.0)
VERSION_SIM ?= 9.0.0
API_SIM     ?= 9.0.0.0
sim:
	go run $(GOFLAGS) ./hack/vcsim -version $(VERSION_SIM) -api $(API_SIM) -l 127.0.0.1:8989

clean:
	rm -rf dist
