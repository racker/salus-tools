FROM golang:1.13 as builder

WORKDIR /build
COPY go.sum go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build

FROM alpine:3

# CA certs needed for Github HTTPS
RUN apk -U add ca-certificates

COPY --from=builder /build/data-loader /usr/local/bin/data-loader
ENTRYPOINT ["/usr/local/bin/data-loader"]
CMD ["webhook-server"]
