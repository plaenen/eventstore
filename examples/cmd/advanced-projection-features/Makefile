.PHONY: help basic rebuild interrupted monitor concurrent all clean nats-up nats-down

help:
	@echo "Advanced Projection Features Demo"
	@echo
	@echo "Available targets:"
	@echo "  make basic        - Run basic scenario"
	@echo "  make rebuild      - Run rebuild optimization scenario"
	@echo "  make interrupted  - Run interrupted rebuild scenario"
	@echo "  make monitor      - Run monitoring scenario"
	@echo "  make concurrent   - Run concurrent events scenario"
	@echo "  make all          - Run all scenarios"
	@echo "  make clean        - Clean up databases"
	@echo "  make nats-up      - Start NATS server (Docker)"
	@echo "  make nats-down    - Stop NATS server (Docker)"

basic:
	@./run.sh basic

rebuild:
	@./run.sh rebuild

interrupted:
	@./run.sh interrupted

monitor:
	@./run.sh monitor

concurrent:
	@./run.sh concurrent

all:
	@./run.sh all

clean:
	@echo "Cleaning up..."
	@rm -f demo_eventstore.db demo_projections.db
	@echo "✅ Done"

nats-up:
	@echo "Starting NATS server with JetStream..."
	@docker run -d -p 4222:4222 -p 8222:8222 --name nats-demo nats:latest -js
	@echo "✅ NATS server started"
	@echo "  NATS: nats://localhost:4222"
	@echo "  HTTP: http://localhost:8222"

nats-down:
	@echo "Stopping NATS server..."
	@docker stop nats-demo 2>/dev/null || true
	@docker rm nats-demo 2>/dev/null || true
	@echo "✅ NATS server stopped"
