SHELL := /bin/sh

COMPOSE_FILE := infra/docker-compose.yml
DATABASE_URL ?= postgres://migrator:dev_migrator_password@127.0.0.1:15432/office_ai?sslmode=disable
ACL_TEST_DATABASE_URL ?= postgres://migrator:dev_migrator_password@127.0.0.1:15432/office_ai?sslmode=disable
WORKER_TEST_DATABASE_URL ?= postgres://migrator:dev_migrator_password@127.0.0.1:15432/office_ai?sslmode=disable

.PHONY: help dev up down migrate migrate-down migrate-version test test-go test-worker test-web lint lint-go lint-worker lint-web secret-scan

help:
	@echo "Targets:"
	@echo "  dev              start infra and print commands to run api/worker/web"
	@echo "  up / down        start/stop local Postgres(pgvector) + Redis"
	@echo "  migrate          apply pending migrations"
	@echo "  migrate-down     roll back the latest migration"
	@echo "  migrate-version  show current schema version"
	@echo "  test             run Go, Python and web tests (offline)"
	@echo "  lint             go vet + ruff/mypy (if installed) + eslint"
	@echo "  secret-scan      gitleaks if installed, else pattern scan"

dev: up
	@echo "Infra running. Start apps in separate terminals:"
	@echo "  API:    go run ./apps/api/cmd/api"
	@echo "  Worker: cd apps/worker && python -m worker.main"
	@echo "  Web:    cd apps/web && npm run dev"

up:
	docker compose -f $(COMPOSE_FILE) up -d

down:
	docker compose -f $(COMPOSE_FILE) down

migrate:
	go -C apps/api run ./cmd/migrate -dir ../../infra/db/migrations -database "$(DATABASE_URL)" up

migrate-down:
	go -C apps/api run ./cmd/migrate -dir ../../infra/db/migrations -database "$(DATABASE_URL)" down 1

migrate-version:
	go -C apps/api run ./cmd/migrate -dir ../../infra/db/migrations -database "$(DATABASE_URL)" version

test: test-go test-worker test-web

test-go:
	cd apps/api && ACL_TEST_DATABASE_URL="$(ACL_TEST_DATABASE_URL)" go test -p 1 ./...

test-worker:
	cd apps/worker && WORKER_TEST_DATABASE_URL="$(WORKER_TEST_DATABASE_URL)" python -m unittest discover -s tests -v

test-web:
	cd apps/web && npm run test

lint: lint-go lint-worker lint-web

lint-go:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		cd apps/api && golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, running go vet"; \
		cd apps/api && go vet ./...; \
	fi

lint-worker:
	@if command -v ruff >/dev/null 2>&1; then \
		cd apps/worker && ruff check worker tests; \
	else \
		echo "ruff not installed, skipping"; \
	fi
	@if command -v mypy >/dev/null 2>&1; then \
		cd apps/worker && mypy worker; \
	else \
		echo "mypy not installed, skipping"; \
	fi

lint-web:
	cd apps/web && npm run lint

secret-scan:
	@if command -v gitleaks >/dev/null 2>&1; then \
		gitleaks detect --no-banner --redact --source .; \
	else \
		sh scripts/secret-scan.sh; \
	fi

backup-db:
	@mkdir -p backups
	docker compose -f $(COMPOSE_FILE) exec -T postgres pg_dump -U $${POSTGRES_USER:-migrator} $${POSTGRES_DB:-office_ai} | gzip > backups/office_ai_$$(date +%Y%m%d_%H%M%S).sql.gz
	@echo "backup written to backups/"

restore-db:
	@if [ -z "$(BACKUP_FILE)" ]; then echo "usage: make restore-db BACKUP_FILE=backups/office_ai_YYYYmmdd_HHMMSS.sql.gz [RESTORE_DB=office_ai_restore]"; exit 1; fi
	docker compose -f $(COMPOSE_FILE) exec -T postgres psql -U $${POSTGRES_USER:-migrator} -d postgres -c "DROP DATABASE IF EXISTS $${RESTORE_DB:-office_ai_restore}" >/dev/null
	docker compose -f $(COMPOSE_FILE) exec -T postgres psql -U $${POSTGRES_USER:-migrator} -d postgres -c "CREATE DATABASE $${RESTORE_DB:-office_ai_restore}" >/dev/null
	gunzip -c "$(BACKUP_FILE)" | docker compose -f $(COMPOSE_FILE) exec -T postgres psql -U $${POSTGRES_USER:-migrator} -d $${RESTORE_DB:-office_ai_restore}
	@echo "restored into $${RESTORE_DB:-office_ai_restore}; verify, then drop it"

loadtest:
	go -C apps/api run ./cmd/loadtest -url "$${LOADTEST_URL:-http://127.0.0.1:18080}" -n $${N:-200} -c $${C:-10} -email $${LOADTEST_EMAIL:-admin@dev.local}
