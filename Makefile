.PHONY: test test-unit test-e2e test-e2e-kind test-e2e-setup test-e2e-teardown run build clean

# Run all tests
test:
	go test -v ./...

# Run unit tests only (exclude e2e)
test-unit:
	go test -v ./internal/...

# Run E2E tests (Go tests only)
test-e2e:
	go test -v ./test/e2e/...

# Run full E2E integration test with kind cluster
test-e2e-kind:
	@echo "Running E2E integration test with kind cluster..."
	@./test/e2e/kind_integration_test.sh

# Setup E2E test environment (creates kind cluster)
test-e2e-setup:
	@echo "Setting up E2E test environment..."
	@if command -v kind > /dev/null; then \
		kind create cluster --name k8s-node-proxy-test --wait 5m || true; \
		echo "E2E test cluster ready"; \
	else \
		echo "kind is not installed. Install with: go install sigs.k8s.io/kind@latest"; \
		exit 1; \
	fi

# Teardown E2E test environment (deletes kind cluster)
test-e2e-teardown:
	@echo "Tearing down E2E test environment..."
	@kind delete cluster --name k8s-node-proxy-test || true
	@kind delete cluster --name k8s-proxy-e2e-test || true
	@echo "E2E test clusters deleted"

run:
	go run ./cmd/server

build:
	go build -o bin/k8s-node-proxy ./cmd/server

clean:
	rm -rf bin/
