# Image construite hors ligne (dépendances vendorisées) : docker build -t vmware-exporter:1.0.0 .
FROM golang:1.25 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -trimpath -ldflags "-s -w -X main.version=1.0.0" -o /out/vmware-exporter ./cmd/vmware-exporter

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/vmware-exporter /usr/local/bin/vmware-exporter
EXPOSE 9272
USER nonroot
ENTRYPOINT ["/usr/local/bin/vmware-exporter"]
CMD ["-config.file", "/etc/vmware-exporter/config.yml"]
