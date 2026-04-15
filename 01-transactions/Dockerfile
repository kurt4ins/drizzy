FROM golang:1.26.2-alpine3.23 AS builder

WORKDIR /app

COPY . .

RUN go build -o /bin/app ./cmd/main.go



FROM alpine:3.23

WORKDIR /app

COPY --from=builder /bin/app /bin/app

ENTRYPOINT ["/bin/app"]