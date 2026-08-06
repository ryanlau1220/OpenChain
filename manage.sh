#!/bin/bash

# OpenChain Platform Management Script

set -e

# ANSI Color Tokens
CYAN='\033[36m'
MAGENTA='\033[35m'
YELLOW='\033[33m'
GREEN='\033[32m'
RED='\033[31m'
RESET='\033[0m'

# Cleanup handler for Ctrl+C signal in dev mode
cleanup_dev() {
    echo ""
    echo -e "${YELLOW}Stopping OpenChain backend and frontend dev processes...${RESET}"
    kill $(jobs -p) 2>/dev/null || true
    echo -e "${YELLOW}Done.${RESET}"
    exit 0
}

set_env_value() {
    local env_file="$1"
    local key="$2"
    local value="$3"

    if grep -q "^${key}=" "$env_file"; then
        sed -i "s|^${key}=.*|${key}=${value}|" "$env_file"
    else
        printf '\n%s=%s\n' "$key" "$value" >> "$env_file"
    fi
}

read_env_value() {
    local env_file="$1"
    local key="$2"
    sed -n "s/^${key}=//p" "$env_file" | tail -n 1
}

case "$1" in
    dev)
        if [ -f .env ]; then
            set -a
            source .env
            set +a
        fi

        # Fast health check for PostgreSQL port 5432
        if ! (nc -z localhost 5432 2>/dev/null || (echo > /dev/tcp/localhost/5432) 2>/dev/null); then
            echo -e "${RED}⚠️  [WARN] PostgreSQL database is not reachable on port 5432.${RESET}"
            echo -e "${YELLOW}Please start the Docker infrastructure in another terminal using:${RESET} ${CYAN}./manage.sh docker${RESET}"
            exit 1
        fi

        # Free ports 8081 and 3000 from lingering background processes
        fuser -k 8081/tcp 3000/tcp 2>/dev/null || true

        trap cleanup_dev INT TERM

        echo -e "${CYAN}Launching OpenChain Go Backend...${RESET}"
        (cd apps/backend && go run ./cmd/server 2>&1 | stdbuf -oL sed "s/^/$(printf "${CYAN}[backend]${RESET}") /") &

        echo -e "${YELLOW}Waiting for Go backend to become healthy on port 8081...${RESET}"
        until curl -s -f http://localhost:8081/api/v1/health >/dev/null 2>&1; do
            sleep 1
        done

        echo -e "${GREEN}✓ [OK] OpenChain Go Backend is 100% HEALTHY & READY on port 8081!${RESET}"
        echo -e "${MAGENTA}Launching OpenChain Web App...${RESET}"
        (pnpm --filter @openchain/web dev 2>&1 | stdbuf -oL sed "s/^/$(printf "${MAGENTA}[web]${RESET}") /") &
        wait
        ;;

    docker)
        echo -e "${YELLOW}Starting Docker Compose infrastructure stack (Postgres, Valkey, ZITADEL)...${RESET}"
        docker compose -f infra/docker-compose.yml up -d
        echo -e "${GREEN}✓ [OK] Docker infrastructure started successfully in detached mode.${RESET}"
        ;;

    docker:down)
        echo -e "${YELLOW}Stopping Docker Compose infrastructure stack...${RESET}"
        docker compose -f infra/docker-compose.yml down
        echo -e "${GREEN}✓ [OK] Docker containers stopped and removed.${RESET}"
        ;;

    build)
        echo -e "${CYAN}Building Go backend binary...${RESET}"
        (cd apps/backend && go build -o bin/server ./cmd/server)
        echo -e "${MAGENTA}Building web frontend...${RESET}"
        pnpm --filter @openchain/web build
        echo -e "${GREEN}✓ Full-stack build complete.${RESET}"
        ;;

    lint)
        echo -e "${CYAN}Running Biome code formatting...${RESET}"
        pnpm format
        echo -e "${CYAN}Running golangci-lint on Go backend...${RESET}"
        if command -v golangci-lint >/dev/null 2>&1; then
            (cd apps/backend && golangci-lint run)
        else
            echo -e "${YELLOW}⚠️ golangci-lint not installed in system PATH; skipping Go linting.${RESET}"
        fi
        echo -e "${GREEN}✓ Linting complete.${RESET}"
        ;;

    check)
        echo -e "${CYAN}Running Biome checks...${RESET}"
        pnpm check
        echo -e "${CYAN}Running TypeScript typecheck on web...${RESET}"
        pnpm --filter @openchain/web build
        echo -e "${GREEN}✓ Quality checks complete.${RESET}"
        ;;

    test)
        echo -e "${CYAN}Running Go unit tests...${RESET}"
        (cd apps/backend && go test -v ./...)
        echo -e "${GREEN}✓ Tests complete.${RESET}"
        ;;

    clean)
        echo -e "${YELLOW}Cleaning build assets and cache...${RESET}"
        rm -rf apps/web/dist .turbo apps/backend/bin
        echo -e "${GREEN}✓ Clean complete.${RESET}"
        ;;

    *)
        echo "Usage: ./manage.sh {dev|docker|docker:down|build|lint|check|test|clean}"
        exit 1
        ;;
esac
