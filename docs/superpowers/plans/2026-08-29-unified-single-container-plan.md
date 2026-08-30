# Implementation Plan: Unified Single Container Architecture

**Goal:** Consolidate Readr into a single multi-stage Docker container that serves both the compiled Vue frontend SPA and the Go Fiber backend on port `8080`.

---

## Proposed Changes

### 1. Backend (`backend/main.go` & `backend/main_test.go`)
- Support configurable `PORT` env var (default `:8080`, supporting `:3000` via env).
- Detect and serve static files from `DIST_DIR` (e.g. `/app/dist` or `../frontend/dist`).
- Add SPA fallback handler (`app.Get("*")`) to serve `index.html` for client-side routes.
- Add test in `backend/main_test.go` verifying static file serving and SPA route fallback.

### 2. Multi-Stage Dockerfile (`Dockerfile`)
- Create root `Dockerfile` with 3 stages:
  1. `frontend-builder` (`oven/bun:latest` -> `bun install --frozen-lockfile && bun run build`)
  2. `backend-builder` (`golang:1.24-alpine` -> `CGO_ENABLED=0 go build`)
  3. `runtime` (`alpine:latest` -> copies `/app/readr` and `/app/dist`, exposes port `8080`)

### 3. Docker Compose (`docker-compose.yml`)
- Simplify `docker-compose.yml` to a single service `readr` exposing port `8080:8080` with volume `./data:/app/data`.

### 4. Vite Proxy Configuration (`frontend/vite.config.ts`)
- Support `process.env.BACKEND_PORT || 8080` (or `3000`) for seamless local dev.

### 5. Cleanup
- Remove `frontend/server.go` and `frontend/dockerfile`.

---

## Verification Steps
1. Run backend tests (`cd backend && go test -count=1 ./...`)
2. Run frontend tests & build (`cd frontend && bun test && bun run build`)
3. Build and test unified Docker image (`docker build -t readr .`)
