include .env
export

.PHONY: up down run tidy

up:
	docker compose up -d
	@echo "Waiting for PostgreSQL..."
	@until docker compose exec -T postgres pg_isready -U $(POSTGRES_USER) -q; do sleep 1; done
	@echo "PostgreSQL ready."

down:
	docker compose down -v

tidy:
	go mod tidy

run: up
	go run ./cmd
