DSN ?= postgres://drizzy:drizzy@localhost:5432/drizzy?sslmode=disable

.PHONY: deps migrate migrate-down build-bot build-profile up down test

## Pull all Go dependencies and generate go.sum
deps:
	go mod tidy

## Run all pending migrations
migrate:
	goose -dir migrations postgres "$(DSN)" up

## Roll back the last migration
migrate-down:
	goose -dir migrations postgres "$(DSN)" down

## Build bot-service binary
build-bot:
	go build -o bin/bot-service ./bot-service/cmd/main.go

## Build profile-service binary
build-profile:
	go build -o bin/profile-service ./profile-service/cmd/main.go

## Start all services via Docker Compose
up:
	docker compose up --build

## Stop all services
down:
	docker compose down

## Run tests across all packages
test:
	go test ./...

## Run tests for a specific package, e.g.: make test-pkg PKG=./bot-service/...
test-pkg:
	go test $(PKG)
