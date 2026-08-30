# syntax=docker/dockerfile:1

# Stage 1: Build Vue Frontend (Runs natively on build host for maximum speed)
FROM --platform=$BUILDPLATFORM oven/bun:latest AS frontend-builder
WORKDIR /app/frontend

# Cache Bun dependency installation layer
COPY frontend/package.json frontend/bun.lock ./
RUN --mount=type=cache,target=/root/.bun/install/cache \
    bun install --frozen-lockfile

# Build frontend assets
COPY frontend/ ./
RUN bun run build

# Stage 2: Build Go Backend (Cross-compiles natively with 0 QEMU emulation overhead)
FROM --platform=$BUILDPLATFORM golang:alpine AS backend-builder
WORKDIR /app/backend

ARG TARGETOS
ARG TARGETARCH

# Download modules with Go module cache mount
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Compile statically linked Go binary for the target architecture
COPY backend/ ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w -extldflags '-static'" -o /app/readr .

# Stage 3: Runtime Container (Target platform)
FROM alpine:3.24
WORKDIR /app

# Install runtime SSL certs and timezone data using apk cache
RUN --mount=type=cache,target=/var/cache/apk \
    apk add --no-cache ca-certificates tzdata

# Copy artifacts from builder stages
COPY --from=backend-builder /app/readr /app/readr
COPY --from=frontend-builder /app/frontend/dist /app/dist

# Default environment configuration
ENV PORT=8080
ENV DIST_DIR=/app/dist
ENV DATA_DIR=/app/data

VOLUME ["/app/data"]
EXPOSE 8080

ENTRYPOINT ["/app/readr"]
