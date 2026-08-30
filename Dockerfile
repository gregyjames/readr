# Stage 1: Build Vue Frontend
FROM oven/bun:latest AS frontend-builder
WORKDIR /app/frontend

COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile

COPY frontend/ ./
RUN bun run build

# Stage 2: Build Go Backend
FROM golang:alpine AS backend-builder
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
