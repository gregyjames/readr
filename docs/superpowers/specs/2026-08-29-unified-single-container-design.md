# Unified Single Container Design Specification

## 1. Overview
Consolidate Readr's frontend and backend into a single, unified Docker container and single-service `docker-compose.yml`. The Go backend will directly serve static frontend assets (`dist/`) and handle client-side SPA routing fallbacks alongside its existing REST and SSE streaming APIs on a single port (`8080`).

---

## 2. Goals & Benefits
* **Single Container Deployment:** One Docker container (`readr`) instead of two separate containers.
* **Single Port:** Exposes only one port (`8080`), eliminating port collisions and reverse-proxy overhead.
* **Zero Reverse Proxy:** Eliminates `frontend/server.go` and `frontend/dockerfile`.
* **Simplified Docker Compose:** Replaces multi-service compose with a straightforward single-service definition.
* **Dev Workflow Parity:** Retains Vite hot-module replacement on `:5173` proxying to Go backend during local development.

---

## 3. Architecture & Container Strategy

### 3.1 Multi-Stage Root `Dockerfile`
A single multi-stage `Dockerfile` in the root repository directory:

```dockerfile
# Stage 1: Build Vue Frontend
FROM oven/bun:latest AS frontend-builder
WORKDIR /app/frontend

COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile

COPY frontend/ ./
RUN bun run build

# Stage 2: Build Go Backend
FROM golang:1.24-alpine AS backend-builder
WORKDIR /app/backend

RUN apk add --no-cache ca-certificates

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/readr .

# Stage 3: Runtime Container
FROM alpine:latest
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=backend-builder /app/readr /app/readr
COPY --from=frontend-builder /app/frontend/dist /app/dist

ENV PORT=8080
ENV DIST_DIR=/app/dist
ENV DATA_DIR=/app/data

VOLUME ["/app/data"]
EXPOSE 8080

ENTRYPOINT ["/app/readr"]
```

### 3.2 Simplified `docker-compose.yml`
```yaml
version: '3.8'

services:
  readr:
    build: .
    image: gjames8/readr:latest
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/data
    restart: unless-stopped
```

---

## 4. Backend Routing & SPA Fallback (`backend/main.go`)

### 4.1 Configuration via Environment Variables
* `PORT`: Default to `8080` (with fallback to `3000` if specified).
* `DIST_DIR`: Path to compiled static files. Checked in order:
  1. `os.Getenv("DIST_DIR")`
  2. `/app/dist` (Docker standard)
  3. `../frontend/dist` or `./dist` (Local dev paths)
* `DATA_DIR`: Path to data directory. Checked in order:
  1. `os.Getenv("DATA_DIR")`
  2. `./data` or `/app/data`

### 4.2 Route Handling
1. **API Endpoints (`/api/*`):** High precedence, handled by Fiber routes.
2. **Article & Image Static Paths (`/articles/*`, `/images/*`):** Handled by data storage handlers.
3. **Static File Serving:** If `DIST_DIR` exists:
   * `app.Static("/", distDir)` serves CSS, JS, favicon, assets.
4. **SPA Fallback Handler:**
   * Any `GET` request not matching API or static files serves `${DIST_DIR}/index.html` with status `200 OK` (so `/chat`, `/graph`, `/articles/:id`, `/settings` reload cleanly).

---

## 5. Cleanup & Deprecations
* Delete `frontend/server.go`.
* Delete `frontend/dockerfile`.
* Update `README.md` Docker compose instructions to reflect the single-container setup.

---

## 6. Testing & Verification Plan
1. **Frontend Build Verification:** `cd frontend && bun test && bun run build`.
2. **Backend Unit & Integration Tests:** `cd backend && go test -count=1 ./...`.
3. **Docker Build Verification:** `docker build -t readr .` builds cleanly.
4. **End-to-End Container Verification:**
   * Container starts and binds to port `8080`.
   * `GET http://localhost:8080/` returns `index.html`.
   * `GET http://localhost:8080/chat` returns `index.html` (SPA fallback).
   * `GET http://localhost:8080/api/getarticles` returns `[]` (API route).
