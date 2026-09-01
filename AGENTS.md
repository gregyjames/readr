# Agent Guidelines & Repository Rules

## Verification Workflow
Before finalizing changes or reporting completion:
1. Run `go fmt ./...` and `go mod tidy`
2. Run `golangci-lint run ./...`
3. Run `go test -race ./...`
4. Run `go vet ./...`
