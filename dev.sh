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
# Ensure dependencies are installed
if [ ! -d "node_modules" ]; then
    echo "Installing frontend dependencies..."
    npm install
fi
npm run dev &
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
