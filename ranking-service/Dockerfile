FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /ranking-service ./ranking-service/cmd/main.go

FROM alpine:3.20
RUN apk --no-cache add ca-certificates
COPY --from=builder /ranking-service /ranking-service
EXPOSE 8081
ENTRYPOINT ["/ranking-service"]
