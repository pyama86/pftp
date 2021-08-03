FROM golang:1.16

WORKDIR /go/src/github.com/pyama86/pftp/

COPY go.mod go.sum ./
RUN go mod download

COPY pftp ./pftp
COPY example ./example
COPY main.go ./

RUN GOOS=linux CGO_ENABLED=0 go build -a -o pftp_bin main.go


FROM alpine:latest
WORKDIR /app/

COPY --from=0 /go/src/github.com/pyama86/pftp/pftp_bin ./

COPY config.toml config.toml
COPY tls/ tls/

CMD ["./pftp_bin"]