-include .env
export

COMPOSE := docker compose
BIN := ./bin/booking
SSLMODE := $(or $(POSTGRES_SSLMODE),disable)

# Тестовая БД. Имя обязано оканчиваться на _test: тесты откажутся работать
# с базой, где это не так, чтобы TRUNCATE не задел dev-данные.
TEST_POSTGRES_HOST ?= 127.0.0.1
TEST_POSTGRES_PORT ?= 5433
TEST_POSTGRES_USER ?= $(POSTGRES_USER)
TEST_POSTGRES_PASSWORD ?= $(POSTGRES_PASSWORD)
TEST_POSTGRES_DB ?= $(POSTGRES_DB)_test
TEST_POSTGRES_SSLMODE ?= $(SSLMODE)

.PHONY: help check-env app-build app-run dev dev-down db-up db-down db-reset \
        migrate-create migrate-up migrate-down migrate-action \
        test test-integration lint fmt fmt-check check pr

help:
	@echo "app-build          build the binary"
	@echo "app-run            build and run the app"
	@echo "dev                db + migrations + app"
	@echo "dev-down           stop containers, keep data"
	@echo "db-up / db-down    postgres only"
	@echo "db-reset           remove containers and volumes (asks first)"
	@echo "migrate-create     make migrate-create seq=<name>"
	@echo "migrate-up         apply all migrations"
	@echo "migrate-down       roll back one migration (steps=N for more)"
	@echo "test               unit tests with -race"
	@echo "test-integration   integration tests against real PostgreSQL (-race)"
	@echo "lint               golangci-lint"
	@echo "fmt                format code (gofmt + import grouping)"
	@echo "fmt-check          check gofmt and import grouping"
	@echo "check              fmt-check + build + vet + test + lint"
	@echo "pr                 push branch and open a PR (TITLE=... DRAFT=1)"

check-env:
	@test -f .env || { echo "error: .env is missing. Copy .env.example to .env and fill it in." >&2; exit 1; }
	@test -n "$(POSTGRES_USER)" || { echo "error: POSTGRES_USER is empty in .env" >&2; exit 1; }
	@test -n "$(POSTGRES_PASSWORD)" || { echo "error: POSTGRES_PASSWORD is empty in .env" >&2; exit 1; }
	@test -n "$(POSTGRES_DB)" || { echo "error: POSTGRES_DB is empty in .env" >&2; exit 1; }

app-build:
	@go build -o $(BIN) ./cmd/booking

app-run: check-env app-build
	@$(BIN) --config=./config/config.yml

dev: db-up migrate-up app-run

dev-down:
	@$(COMPOSE) down

db-up: check-env
	@$(COMPOSE) up -d --wait db

db-down:
	@$(COMPOSE) stop db

db-reset:
	@printf "Delete the environment's volumes and all DB data? [y/N]: "; \
	read ans; \
	if [ "$$ans" = "y" ]; then \
		$(COMPOSE) down -v; \
		echo "Environment stopped, volumes removed."; \
	else \
		echo "Reset cancelled."; \
	fi

migrate-create:
	@if [ -z "$(seq)" ]; then \
		echo "seq parameter is required. Example: make migrate-create seq=init"; \
		exit 1; \
	fi; \
	mkdir -p migrations; \
	$(COMPOSE) run --rm --no-deps --user $(shell id -u):$(shell id -g) migrate \
		create \
		-ext sql \
		-dir /migrations \
		-seq "$(seq)"

migrate-up:
	@$(MAKE) migrate-action action=up

migrate-down:
	@$(MAKE) migrate-action action=down steps=$(or $(steps),1)

migrate-action: check-env
	@if [ -z "$(action)" ]; then \
		echo "action parameter is required. Example: make migrate-action action=up"; \
		exit 1; \
	fi; \
	$(COMPOSE) run --rm migrate \
		-path /migrations \
		-database "postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@db:5432/$(POSTGRES_DB)?sslmode=$(SSLMODE)" \
		"$(action)" $(if $(steps),$(steps))

test:
	@go test -race ./...

test-integration: check-env db-up
	@$(COMPOSE) exec -T db psql -U $(POSTGRES_USER) -d postgres -tAc \
		"SELECT 1 FROM pg_database WHERE datname = '$(TEST_POSTGRES_DB)'" | grep -q 1 || \
		$(COMPOSE) exec -T db createdb -U $(POSTGRES_USER) $(TEST_POSTGRES_DB)
	@$(COMPOSE) run --rm migrate \
		-path /migrations \
		-database "postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@db:5432/$(TEST_POSTGRES_DB)?sslmode=$(SSLMODE)" \
		up
	@go test -race -shuffle=on -count=1 -tags integration ./...

lint:
	@golangci-lint run --build-tags=integration

fmt:
	@gofmt -w .
	@golangci-lint fmt

fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "These files are not gofmt'ed:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	@golangci-lint fmt --diff

check:
	@$(MAKE) fmt-check
	@go build ./...
	@go vet ./...
	@$(MAKE) test
	@$(MAKE) lint

pr:
	@TITLE="$(TITLE)" DRAFT="$(DRAFT)" ./scripts/pushpr.sh
