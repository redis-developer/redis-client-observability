# Redis Metrics Stack - Makefile
# Observability infrastructure for Redis testing applications

.PHONY: help start stop restart status logs clean build

# Default target
help: ## Show this help message
	@echo "Redis Metrics Stack - Available Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Quick Start:"
	@echo "  make start           # Start the metrics stack"
	@echo "  make status          # Check if services are running"
	@echo "  make logs            # View service logs"
	@echo "  make stop            # Stop all services"
	@echo ""
	@echo "Service Management:"
	@echo "  make restart         # Restart all services"
	@echo ""
	@echo "Logs (Individual Services):"
	@echo "  make logs-prometheus # View Prometheus logs only"
	@echo "  make logs-grafana    # View Grafana logs only"
	@echo "  make logs-otel       # View OpenTelemetry Collector logs only"
	@echo ""
	@echo "Cleanup:"
	@echo "  make clean           # Stop services and remove volumes (deletes all data)"

start: ## Start the metrics stack (Redis, OpenTelemetry, Prometheus, Grafana)
	@echo "🚀 Starting Redis Metrics Stack..."
	docker compose up -d
	@echo ""
	@echo "✅ Metrics stack started!"
	@echo ""
	@echo "📊 Access your services:"
	@echo "  • Grafana:    http://localhost:3000 (admin/admin)"
	@echo "  • Prometheus: http://localhost:9090"
	@echo "  • Redis:      localhost:6379"
	@echo ""
	@echo "🔗 OTLP Endpoints for Redis test apps:"
	@echo "  • gRPC: http://localhost:4317"
	@echo "  • HTTP: http://localhost:4318"

stop: ## Stop all services
	@echo "🛑 Stopping Redis Metrics Stack..."
	docker compose down
	@echo "✅ All services stopped"

restart: ## Restart all services
	@echo "🔄 Restarting Redis Metrics Stack..."
	docker compose restart
	@echo "✅ All services restarted"

status: ## Check service status
	@echo "📊 Redis Metrics Stack Status:"
	@echo ""
	@docker compose ps
	@echo ""
	@echo "🔍 Service Health:"
	@echo -n "  Redis:      "; redis-cli -h localhost -p 6379 ping > /dev/null 2>&1 && echo "✅ Running" || echo "❌ Not accessible"
	@echo -n "  Prometheus: "; curl -s http://localhost:9090/-/healthy > /dev/null 2>&1 && echo "✅ Running" || echo "❌ Not accessible"
	@echo -n "  Grafana:    "; curl -s http://localhost:3000/api/health > /dev/null 2>&1 && echo "✅ Running" || echo "❌ Not accessible"
	@echo -n "  OTLP gRPC:  "; nc -z localhost 4317 > /dev/null 2>&1 && echo "✅ Running" || echo "❌ Not accessible"

logs: ## View logs from all services
	@echo "📋 Viewing logs from all services..."
	@if docker compose ps -q | grep -q .; then \
		docker compose logs -f; \
	else \
		echo "No docker compose services running. Showing logs from individual containers:"; \
		echo ""; \
		docker logs redis-test-db --tail 20 2>/dev/null || echo "Redis container not found"; \
		echo ""; \
		docker logs prometheus --tail 20 2>/dev/null || echo "Prometheus container not found"; \
		echo ""; \
		docker logs grafana --tail 20 2>/dev/null || echo "Grafana container not found"; \
		echo ""; \
		docker logs otel-collector --tail 20 2>/dev/null || echo "OpenTelemetry container not found"; \
	fi

logs-prometheus: ## View Prometheus logs only
	@echo "📋 Viewing Prometheus logs..."
	@if docker compose ps prometheus -q | grep -q .; then \
		docker compose logs -f prometheus; \
	else \
		docker logs prometheus -f 2>/dev/null || echo "❌ Prometheus container not found"; \
	fi

logs-grafana: ## View Grafana logs only
	@echo "📋 Viewing Grafana logs..."
	@if docker compose ps grafana -q | grep -q .; then \
		docker compose logs -f grafana; \
	else \
		docker logs grafana -f 2>/dev/null || echo "❌ Grafana container not found"; \
	fi

logs-otel: ## View OpenTelemetry Collector logs only
	@echo "📋 Viewing OpenTelemetry Collector logs..."
	@if docker compose ps otel-collector -q | grep -q .; then \
		docker compose logs -f otel-collector; \
	else \
		docker logs otel-collector -f 2>/dev/null || echo "❌ OpenTelemetry container not found"; \
	fi

clean: ## Stop services and remove volumes (WARNING: deletes all data)
	@echo "⚠️  WARNING: This will delete all stored metrics data for this Redis stack!"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	echo ""; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		echo "🧹 Cleaning up Redis Metrics Stack..."; \
		docker compose down -v --remove-orphans; \
		echo "✅ Cleanup complete"; \
	else \
		echo "❌ Cleanup cancelled"; \
	fi