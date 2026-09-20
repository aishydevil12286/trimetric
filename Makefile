COMPOSE ?= docker compose
DEV_COMPOSE = $(COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml

# Start the stack on http://localhost:8080.
up:
	$(COMPOSE) up --build

# Start it with hot reload for the web app.
dev:
	$(DEV_COMPOSE) up --build

# Start it with the realtime pipeline routed through Kafka.
kafka:
	$(COMPOSE) -f docker-compose.yml -f docker-compose.kafka.yml up --build

down:
	$(COMPOSE) down

# Wipe the database, so the next start reloads the static feed from scratch.
clean:
	$(COMPOSE) down -v

build:
	$(COMPOSE) build

lint: node_modules
	go vet ./...
	npm run lint

# The Go tests need Postgres with PostGIS. `make up` provides one, or point
# them elsewhere with: make test-go POSTGRES_ADDR=host:port
POSTGRES_ADDR ?= localhost:5432

test: test-go test-web

test-go:
	go test ./... -postgres-addr=$(POSTGRES_ADDR)

test-web: node_modules
	npm test

node_modules: package.json package-lock.json
	npm install
	@touch node_modules

.PHONY: up dev kafka down clean build lint test test-go test-web
