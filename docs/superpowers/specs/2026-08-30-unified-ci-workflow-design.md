# Unified CI Workflow Design Specification

## 1. Overview
Consolidate separate, outdated CI workflows (`go.yml` and `node.js.yml`) into a single, high-performance GitHub Actions pipeline (`.github/workflows/ci.yml`) featuring parallel backend and frontend jobs, automatic concurrency cancellation, race condition detection, and unified status reporting.

---

## 2. Problems Addressed
1. **Outdated Actions:** `setup-go@v4` with manual `actions/cache@v3` replaced by `setup-go@v5` with automatic `go.sum` cache management.
2. **Hardcoded Go Version:** Eliminated hardcoded version in favor of dynamic `go-version-file: 'backend/go.mod'`.
3. **Missing Verification Steps:** Added `go vet ./...` and `go test -race` for backend, and `bun test` for frontend.
4. **Wasted Runner Minutes:** Added `concurrency: cancel-in-progress` to cancel stale build queues on new commits.
5. **Mismatched Naming:** Removed `node.js.yml` artifact.

---

## 3. Workflow Architecture (`.github/workflows/ci.yml`)

### 3.1 Pipeline Definition
```yaml
name: CI

on:
  push:
    branches: [ "main" ]
  pull_request:
    branches: [ "main" ]

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  backend:
    name: Backend (Go)
    runs-on: ubuntu-latest
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: 'backend/go.mod'
          cache-dependency-path: 'backend/go.sum'

      - name: Vet & Static Analysis
        working-directory: ./backend
        run: go vet ./...

      - name: Run Tests
        working-directory: ./backend
        run: go test -v -race -count=1 ./...

      - name: Build Binary
        working-directory: ./backend
        run: go build -v .

  frontend:
    name: Frontend (Vue / Bun)
    runs-on: ubuntu-latest
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Set up Bun
        uses: oven-sh/setup-bun@v2
        with:
          bun-version: latest

      - name: Install dependencies
        working-directory: ./frontend
        run: bun install --frozen-lockfile

      - name: Run Tests
        working-directory: ./frontend
        run: bun test

      - name: Build Production Bundle
        working-directory: ./frontend
        run: bun run build
```

---

## 4. Repository Cleanup
* Delete `.github/workflows/go.yml`.
* Delete `.github/workflows/node.js.yml`.
* Update `README.md` badge to reference `ci.yml`.
