FROM golang:1.13 as builder

WORKDIR /build
COPY go.sum go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 make

FROM alpine:3

RUN apk add curl
COPY --from=builder /build/e2et /usr/bin/e2et

ENTRYPOINT ["/usr/bin/e2et", "--web-server-port=80"]
