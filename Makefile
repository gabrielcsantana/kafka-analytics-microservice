.PHONY: help build up down logs test clean health db-shell kafka-topics

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build Docker images
	docker-compose build

.PHONY: up
up: ## Start all services
	docker-compose up -d

.PHONY: down
down: ## Stop all services
	docker-compose down

.PHONY: logs
logs: ## View logs from all services
	docker-compose logs -f

.PHONY: logs-producer
logs-producer: ## View producer logs
	docker-compose logs -f producer

.PHONY: logs-consumer
logs-consumer: ## View consumer logs
	docker-compose logs -f consumer

.PHONY: restart
restart: ## Restart all services
	docker-compose restart

.PHONY: clean
clean: ## Stop and remove all containers and volumes
	docker-compose down -v

.PHONY: test
test: ## Send test events to producer
	chmod +x test-producer.sh
	./test-producer.sh 100

.PHONY: test-unit
test-unit: ## Run unit tests for all services
	@echo "Running producer unit tests..."
	cd producer && go test -v -cover ./...
	@echo "\nRunning consumer unit tests..."
	cd consumer && go test -v -cover ./...

.PHONY: test-producer-unit
test-producer-unit: ## Run unit tests for producer service
	cd producer && go test -v -cover ./...

.PHONY: test-consumer-unit
test-consumer-unit: ## Run unit tests for consumer service
	cd consumer && go test -v -cover ./...

.PHONY: test-all
test-all: test-unit test ## Run all tests (unit + integration)

.PHONY: metrics
metrics: ## View metrics from database
	docker exec -it postgres psql -U postgres -d analytics -c "SELECT * FROM user_activity_metrics ORDER BY created_at DESC LIMIT 20;"

.PHONY: logs-producer
logs-producer:
	docker-compose logs -f producer

.PHONY: logs-consumer
logs-consumer:
	docker-compose logs -f consumer

.PHONY: logs-kafka
logs-kafka:
	docker-compose logs -f kafka

.PHONY: help
help:
	@echo "Available commands:"
	@echo "  make up          - Start all services"
	@echo "  make down        - Stop all services"
	@echo "  make logs        - View all logs"
	@echo "  make test        - Send test events to producer"
	@echo "  make check-db    - View metrics in PostgreSQL"
	@echo "  make clean       - Remove all containers and volumes"
