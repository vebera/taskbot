#!/bin/bash

# Function to display usage
usage() {
    echo "Usage: $0 <command>"
    echo "Commands:"
    echo "  up, -u         - Start taskbot without database"
    echo "  up-db          - Start taskbot with database"
    echo "  down, -d       - Stop all services"
    echo "  db             - Start only the database"
    echo "  logs           - Show logs from all services"
    echo "  logs-db        - Show logs from database only"
    echo "  logs-bot       - Show logs from taskbot only"
    echo "  build, -b      - Rebuild taskbot image"
    echo "  restart, -r    - Restart all services"
    echo "  ps             - Show running services"
    exit 1
}

# Check if command is provided
if [ $# -eq 0 ]; then
    usage
fi

# Handle commands
case "$1" in
    "up"|"-u")
        docker compose up -d
        ;;
    "up-db")
        docker compose --profile database up -d
        ;;
    "down"|"-d")
        docker compose --profile database down
        ;;
    "db")
        docker compose --profile database up -d db
        ;;
    "logs")
        docker compose --profile database logs -f
        ;;
    "logs-db")
        docker compose --profile database logs -f db
        ;;
    "logs-bot")
        docker compose logs -f taskbot
        ;;
    "build"|"-b")
        docker compose build
        ;;
    "restart"|"-r")
        docker compose --profile database down
        docker compose --profile database up -d
        ;;
    "ps")
        docker compose --profile database ps
        ;;
    *)
        echo "Unknown command: $1"
        usage
        ;;
esac 