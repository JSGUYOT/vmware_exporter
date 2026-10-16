# Built from vendor/ (offline):      docker build -t vmware-exporter:1.0.0 .
# Modules downloaded from the proxy:  docker build --build-arg GO_MOD=readonly -t vmware-exporter:1.0.0 .
FROM golang:1.25 AS build
ARG GO_MOD=vendor
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -mod=${GO_MOD} -trimpath -ldflags "-s -w -X main.version=$(cat VERSION)" -o /out/vmware-exporter ./cmd/vmware-exporter

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/vmware-exporter /usr/local/bin/vmware-exporter
EXPOSE 9272
USER nonroot
ENTRYPOINT ["/usr/local/bin/vmware-exporter"]
CMD ["-config.file", "/etc/vmware-exporter/config.yml"]
