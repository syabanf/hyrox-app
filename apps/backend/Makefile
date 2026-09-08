.DEFAULT_GOAL := help
.PHONY: help run migrate seed test test-integration lint fmt tidy build docker up down logs psql reset

# Local development points at the compose database.
DATABASE_URL ?= postgres://nuhabit:nuhabit@localhost:5433/nuhabit?sslmode=disable
TEST_DATABASE_URL ?= postgres://nuhabit:nuhabit@localhost:5433/nuhabit_test?sslmode=disable
export DATABASE_URL

help: ## Show this help
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[1m%-18s\033[0m %s\n", $$1, $$2}'

run: ## Run the API locally (applies migrations on boot)
	LOG_FORMAT=text go run ./cmd/api

migrate: ## Apply pending migrations
	go run ./cmd/migrate

migrate-status: ## Show which migrations have been applied
	go run ./cmd/migrate status

seed: ## Load the demo studio
	LOG_FORMAT=text go run ./cmd/seed

test: ## Run unit tests (no database needed)
	go test ./...

test-integration: ## Run every test, including the ones that need PostgreSQL
	@createdb_url="$(TEST_DATABASE_URL)"; \
	psql "$(DATABASE_URL)" -c 'CREATE DATABASE nuhabit_test' >/dev/null 2>&1 || true; \
	TEST_DATABASE_URL="$$createdb_url" go test ./... -count=1

lint: ## Vet and check formatting
	go vet ./...
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi

fmt: ## Format the code
	gofmt -w .

tidy: ## Tidy the module graph
	go mod tidy

build: ## Build the binaries into ./bin
	go build -trimpath -o bin/api ./cmd/api
	go build -trimpath -o bin/migrate ./cmd/migrate
	go build -trimpath -o bin/seed ./cmd/seed

docker: ## Build the container image
	docker build -t nuhabit-backend:dev .

# The compose stack lives at the repo root: one file brings up the database,
# this API, the member PWA and the admin panel together.
COMPOSE = docker compose -f ../../docker-compose.yml

up: ## Start the whole stack (database, API, member app, admin panel)
	$(COMPOSE) up -d --build
	$(COMPOSE) --profile seed run --rm seed

down: ## Stop the stack
	$(COMPOSE) down

reset: ## Stop the stack and delete its data
	$(COMPOSE) down -v

logs: ## Tail the API logs
	$(COMPOSE) logs -f api

db: ## Start only PostgreSQL, for running the API from source
	$(COMPOSE) up -d postgres

psql: ## Open a database shell
	psql "$(DATABASE_URL)"
