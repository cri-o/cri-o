

Proper Dockerfile for application

```Dockerfile

FROM golang:1.19 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

# Copy the go source
COPY main.go main.go

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o metrics-exporter main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/metrics-exporter .
USER 65532:65532

ENTRYPOINT ["/metrics-exporter"]
```


Test locally
```
go build -o bin/metrics-exporter
```
