#!/bin/bash

# Exit on any error
set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Print with color
print_green() {
    echo -e "${GREEN}$1${NC}"
}

print_red() {
    echo -e "${RED}$1${NC}"
}

# Check if .env file exists
if [ ! -f .env ]; then
    print_red "Error: .env file not found!"
    exit 1
fi

# Check if migrations directory exists
if [ ! -d migrations ]; then
    print_red "Error: migrations directory not found!"
    exit 1
fi

# Run database migrations
print_green "Running database migrations..."
go run cmd/migrate/main.go
if [ $? -ne 0 ]; then
    print_red "Migration failed!"
    exit 1
fi
print_green "Migrations completed successfully."

# Build and run the application
print_green "Starting TaskBot..."
go run cmd/taskbot/main.go 