# Build the binary
FROM golang:1.20 as builder

WORKDIR /workspace

# Install upx for compress binary file
RUN apt update && apt install -y upx

# Copy the go source
COPY . .

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GO111MODULE=on

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Build and compression
RUN go build -a -installsuffix cgo -ldflags="-s -w" -o bin/server server/main.go \
    && go build -a -installsuffix cgo -ldflags="-s -w" -o bin/client client/main.go \
    && upx bin/server bin/client

# build server
FROM frolvlad/alpine-glibc:glibc-2.34 as server
WORKDIR /
COPY --from=builder /workspace/bin/server .

ENTRYPOINT ["/server"]

# build client
FROM frolvlad/alpine-glibc:glibc-2.34 as client
WORKDIR /
COPY --from=builder /workspace/bin/client .

ENTRYPOINT ["/client"]
