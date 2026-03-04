.PHONY: build run sync-station sync-schedule sync sync-mrt-station sync-mrt-schedule sync-mrt up down prod-up prod-down prod-logs swag tidy clean help

# Binary output path
BIN := bin/api

## build: compile the API binary
build:
	go build -ldflags="-s -w" -o $(BIN) ./cmd/api

## run: run the API server (with live .env)
run:
	go run ./cmd/api

## sync-station: fetch and upsert stations from KRL API
sync-station:
	go run ./cmd/sync --type station

## sync-schedule: fetch and upsert schedules from KRL API
sync-schedule:
	go run ./cmd/sync --type schedule

## sync: run both station and schedule sync in order
sync: sync-station sync-schedule sync-mrt

## sync-mrt-station: fetch and upsert MRT stations
sync-mrt-station:
	go run ./cmd/sync --type mrt-station

## sync-mrt-schedule: fetch and upsert MRT schedules
sync-mrt-schedule:
	go run ./cmd/sync --type mrt-schedule

## sync-mrt: run MRT station and schedule sync
sync-mrt: sync-mrt-station sync-mrt-schedule

## up: start PostgreSQL and Redis via Docker Compose
up:
	docker compose up -d

## down: stop Docker Compose services
down:
	docker compose down

## prod-up: build and start all production services (app + postgres + redis)
prod-up:
	docker compose -f docker-compose.prod.yml up -d --build

## prod-down: stop production services (data is preserved)
prod-down:
	docker compose -f docker-compose.prod.yml down

## prod-logs: follow logs from all production services
prod-logs:
	docker compose -f docker-compose.prod.yml logs -f

## swag: regenerate Swagger docs (requires swag: go install github.com/swaggo/swag/cmd/swag@latest)
swag:
	swag init -g cmd/api/main.go -o docs

## tidy: tidy and verify Go modules
tidy:
	go mod tidy
	go mod verify

## clean: remove build artifacts
clean:
	rm -rf bin/

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
