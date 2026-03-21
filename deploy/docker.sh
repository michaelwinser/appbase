#!/bin/sh
# deploy/docker.sh — Local Docker and TrueNAS Docker deployment
#
# Requires: deploy/config.sh sourced first
# Requires: docker and docker compose
#
# For local development and TrueNAS. Uses SQLite with a persistent volume.

# deploy_docker_up — build and start the app in Docker.
# Usage: deploy_docker_up [compose-file]
deploy_docker_up() {
    compose="${1:-deploy/docker-compose.yml}"
    echo "Starting app container..."
    docker compose -f "$compose" up -d --build
    echo "App started."
    echo "  Logs: docker compose -f $compose logs -f"
}

# deploy_docker_down — stop the app container.
# Usage: deploy_docker_down [compose-file]
deploy_docker_down() {
    compose="${1:-deploy/docker-compose.yml}"
    echo "Stopping app container..."
    docker compose -f "$compose" down
    echo "App stopped."
}

# deploy_docker_logs — tail the app container logs.
# Usage: deploy_docker_logs [compose-file]
deploy_docker_logs() {
    compose="${1:-deploy/docker-compose.yml}"
    docker compose -f "$compose" logs -f
}
