FROM golang:alpine AS builder

WORKDIR /go/src/github.com/spetzreborn/get_host/
COPY . .
WORKDIR /go/src/github.com/spetzreborn/get_host/cmd/server
RUN CGO_ENABLED=0 go build -tags=netgo -ldflags="-s -w" -o server -a -installsuffix cgo .

FROM scratch
COPY --from=builder /go/src/github.com/spetzreborn/get_host/cmd/server /

EXPOSE 8080
ENTRYPOINT ["/server", "--configfile", "/config/config.toml"]
