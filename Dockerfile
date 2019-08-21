FROM golang:alpine AS builder

WORKDIR /go/src/github.com/spetzreborn/get_host/
COPY . .
WORKDIR /go/src/github.com/spetzreborn/get_host/cmd/server
RUN CGO_ENABLED=0 go build -tags=netgo -o server .

FROM scratch
COPY --from=builder /go/src/github.com/spetzreborn/get_host/cmd/server /

EXPOSE 8080
ENTRYPOINT ["/server", "--configfile", "/config/config.toml"]
