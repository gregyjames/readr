#!/bin/bash

# Exit on error
set -e

echo "Starting Readr local development environment..."

# Trap CTRL+C to cleanly shut down both processes
trap 'echo -e "\nShutting down..."; kill $BACKEND_PID $FRONTEND_PID 2>/dev/null; exit' SIGINT SIGTERM

# Start Go backend in the background
echo "Starting backend (Go)..."
cd backend
go run main.go &
BACKEND_PID=$!
cd ..

# Start Vue frontend in the background
echo "Starting frontend (Vite)..."
cd frontend
# Detect package manager (prefer bun if available, fall back to npm)
if command -v bun >/dev/null 2>&1; then
    PM="bun"
    INSTALL_CMD="bun install"
    DEV_CMD="bun run dev"
else
    PM="npm"
    INSTALL_CMD="npm install"
    DEV_CMD="npm run dev"
fi

# Ensure dependencies are installed
if [ ! -d "node_modules" ]; then
    echo "Installing frontend dependencies with $PM..."
    $INSTALL_CMD
fi
$DEV_CMD &
FRONTEND_PID=$!
cd ..

echo "----------------------------------------"
echo "🚀 Development servers are running!"
echo "Backend API: http://localhost:8080"
echo "Frontend UI: http://localhost:5173 (Dev) / http://localhost:8080 (Prod)"
echo "Press Ctrl+C to stop both servers."
echo "----------------------------------------"

# Wait for both background processes
wait $BACKEND_PID $FRONTEND_PID
