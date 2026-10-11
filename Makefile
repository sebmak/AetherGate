.PHONY: all build build-web build-server run test clean docker-build docker-up

all: build

# Build React frontend
build-web:
	@echo "==> Building React frontend..."
	cd web && npm install && npm run build

# Build Go backend
build-server:
	@echo "==> Building Go backend..."
	go build -o ./bin/proxyserver ./cmd/proxyserver

# Build both
build: build-web build-server
	@echo "==> Build complete: ./bin/proxyserver"

# Run tests
test:
	@echo "==> Running Go unit tests..."
	go test -v ./...

# Run server locally
run: build
	@echo "==> Running ProxyServer Gateway..."
	./bin/proxyserver

# Clean build artifacts
clean:
	@echo "==> Cleaning artifacts..."
	rm -rf bin/ web/dist/

# Docker build and run
docker-build:
	docker build -t proxyserver:latest .

docker-up:
	docker compose up -d
