.PHONY: all check test test-backend test-frontend build dev fmt fmt-check tidy help

# Default target runs complete verification
all: check

help:
	@echo "Readr Development & Verification Targets:"
	@echo "  make check          - Full workspace validation (backend vet/tests + frontend tests/typecheck)"
	@echo "  make test           - Run backend race tests and frontend unit tests"
	@echo "  make test-backend   - Run Go backend tests with race detection"
	@echo "  make test-frontend  - Run frontend unit tests"
	@echo "  make build          - Build backend binary and frontend production bundle"
	@echo "  make dev            - Start local backend and frontend development servers"
	@echo "  make fmt            - Format Go backend code"
	@echo "  make tidy           - Run go mod tidy on backend dependencies"

# Full workspace check
check: fmt-check vet test-backend test-frontend typecheck

fmt:
	cd backend && go fmt ./...

fmt-check:
	@test -z "$$(cd backend && gofmt -l .)" || (echo "Unformatted Go files detected; run 'make fmt'" && exit 1)

tidy:
	cd backend && go mod tidy

vet:
	cd backend && go vet ./...

test: test-backend test-frontend

test-backend:
	cd backend && go test -race -count=1 ./...

test-frontend:
	@if command -v bun >/dev/null 2>&1; then \
		(cd frontend && bun test); \
	else \
		(cd frontend && npx bun test); \
	fi

typecheck:
	@if command -v bun >/dev/null 2>&1; then \
		(cd frontend && bun x vue-tsc -b); \
	else \
		(cd frontend && npx vue-tsc -b); \
	fi

build: build-frontend build-backend

build-frontend:
	@if command -v bun >/dev/null 2>&1; then \
		(cd frontend && bun run build); \
	else \
		(cd frontend && npm run build); \
	fi

build-backend:
	cd backend && go build -v -o readr .

dev:
	./dev.sh
