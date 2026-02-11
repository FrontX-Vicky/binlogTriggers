.PHONY: build clean run-emitter run-subscribers docker-build docker-up docker-down install-systemd

# Build binaries
build:
	@echo "Building binaries..."
	@mkdir -p bin
	go build -o bin/emitter ./cmd/emitter
	go build -o bin/subscribers ./cmd/subscribers
	go build -o bin/console-subscriber ./cmd/subscribers/console
	@echo "Build complete!"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	@echo "Clean complete!"

# Run emitter locally
run-emitter: build
	@echo "Running emitter..."
	./bin/emitter

# Run subscribers locally
run-subscribers: build
	@echo "Running subscribers..."
	ENV_FILES=./.env.lead_events,./.env.user_events ./bin/subscribers

# Run console subscriber (single)
run-console: build
	@echo "Running console subscriber..."
	./bin/console-subscriber

# Docker commands
docker-build:
	@echo "Building Docker images..."
	docker-compose build

docker-up:
	@echo "Starting services with Docker Compose..."
	docker-compose up -d

docker-down:
	@echo "Stopping Docker services..."
	docker-compose down

docker-logs:
	docker-compose logs -f

docker-restart:
	@echo "Restarting Docker services..."
	docker-compose restart

# Install systemd services (Linux only)
install-systemd: build
	@echo "Installing systemd services..."
	sudo mkdir -p /opt/cdc-platform/bin
	sudo mkdir -p /opt/cdc-platform/logs
	sudo cp bin/emitter /opt/cdc-platform/bin/
	sudo cp bin/subscribers /opt/cdc-platform/bin/
	sudo cp .env* /opt/cdc-platform/
	sudo cp systemd/*.service /etc/systemd/system/
	sudo useradd -r -s /bin/false cdc || true
	sudo chown -R cdc:cdc /opt/cdc-platform
	sudo systemctl daemon-reload
	@echo "Installation complete! Enable services with:"
	@echo "  sudo systemctl enable cdc-emitter"
	@echo "  sudo systemctl enable cdc-subscribers"
	@echo "  sudo systemctl start cdc-emitter"
	@echo "  sudo systemctl start cdc-subscribers"

# Systemd management
start-systemd:
	sudo systemctl start cdc-emitter
	sudo systemctl start cdc-subscribers

stop-systemd:
	sudo systemctl stop cdc-emitter
	sudo systemctl stop cdc-subscribers

status-systemd:
	sudo systemctl status cdc-emitter
	sudo systemctl status cdc-subscribers

# Development
dev-setup:
	@echo "Setting up development environment..."
	cp .env.example .env
	@echo "Created .env file. Please update with your configuration."

# Testing
test:
	go test ./... -v

# Formatting and linting
fmt:
	go fmt ./...

lint:
	golangci-lint run

# Help
help:
	@echo "Available targets:"
	@echo "  build             - Build all binaries"
	@echo "  clean             - Clean build artifacts"
	@echo "  run-emitter       - Run emitter locally"
	@echo "  run-subscribers   - Run subscribers locally"
	@echo "  run-console       - Run console subscriber"
	@echo "  docker-build      - Build Docker images"
	@echo "  docker-up         - Start Docker services"
	@echo "  docker-down       - Stop Docker services"
	@echo "  docker-logs       - View Docker logs"
	@echo "  install-systemd   - Install systemd services (Linux)"
	@echo "  start-systemd     - Start systemd services"
	@echo "  stop-systemd      - Stop systemd services"
	@echo "  status-systemd    - Check systemd services status"
	@echo "  test              - Run tests"
	@echo "  fmt               - Format code"
	@echo "  lint              - Lint code"
