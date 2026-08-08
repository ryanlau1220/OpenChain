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

# Auto-load .env file if available
if [ -f .env ]; then
    set -a
    source .env
    set +a
fi

case "$1" in
    dev)
        # Free ports 8081 and 3000 from lingering background processes
        fuser -k 8081/tcp 3000/tcp 2>/dev/null || true

        trap cleanup_dev INT TERM

        echo -e "${YELLOW}Checking infrastructure containers (PostgreSQL + Apache AGE, TrueBlocks)...${RESET}"
        if command -v docker >/dev/null 2>&1 && [ -f infra/docker-compose.yml ]; then
            docker compose -f infra/docker-compose.yml up -d
        fi

        if command -v docker >/dev/null 2>&1; then
            echo -e "${YELLOW}Waiting for PostgreSQL + Apache AGE container to become ready...${RESET}"
            until [ "$(docker inspect --format='{{.State.Health.Status}}' openchain-postgres 2>/dev/null)" = "healthy" ]; do
                sleep 1
            done
            echo -e "${GREEN}✓ [OK] PostgreSQL + Apache AGE Service is READY!${RESET}"

            echo -e "${YELLOW}Waiting for TrueBlocks indexing service to become ready on port 8080...${RESET}"
            until curl -s -f http://localhost:8080/status?fmt=json >/dev/null 2>&1; do
                sleep 1
            done
            echo -e "${GREEN}✓ [OK] TrueBlocks Indexing Engine is READY on port 8080!${RESET}"
        fi

        echo -e "${CYAN}Launching OpenChain Go Backend (Port ${PORT:-8081} with Air Live Reload)...${RESET}"
        if command -v air >/dev/null 2>&1; then
            (cd apps/backend && air -c .air.toml 2>&1 | stdbuf -oL sed "s/^/$(printf "${CYAN}[backend]${RESET}") /") &
        else
            (cd apps/backend && go run github.com/air-verse/air@v1.61.0 -c .air.toml 2>&1 | stdbuf -oL sed "s/^/$(printf "${CYAN}[backend]${RESET}") /") &
        fi

        echo -e "${YELLOW}Waiting for Go backend to become healthy on port ${PORT:-8081}...${RESET}"
        until curl -s -f http://localhost:${PORT:-8081}/api/v1/health >/dev/null 2>&1; do
            sleep 1
        done

        echo -e "${GREEN}✓ [OK] OpenChain Go Backend is 100% HEALTHY & READY on port ${PORT:-8081}!${RESET}"
        echo -e "${MAGENTA}Launching OpenChain TanStack Start Web App (Port 3000)...${RESET}"
        (pnpm --filter @openchain/web dev 2>&1 | stdbuf -oL sed "s/^/$(printf "${MAGENTA}[web]${RESET}") /") &
        wait

        ;;


    docker)
        echo -e "${YELLOW}Building and starting infrastructure Docker Compose containers...${RESET}"
        docker compose -f infra/docker-compose.yml up -d --remove-orphans
        echo -e "${GREEN}✓ [OK] Infrastructure containers started successfully.${RESET}"
        ;;


    docker:down)
        echo -e "${YELLOW}Stopping Docker Compose stack...${RESET}"
        docker compose -f infra/docker-compose.yml down
        echo -e "${GREEN}✓ [OK] Docker containers stopped.${RESET}"
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
        npx biome check --write apps/web
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
        npx biome check apps/web
        echo -e "${CYAN}Running TypeScript typecheck on web...${RESET}"
        pnpm --filter @openchain/web build
        echo -e "${GREEN}✓ Quality checks complete.${RESET}"
        ;;

    test)
        echo -e "${CYAN}Running Go backend unit, integration & E2E tests...${RESET}"
        (cd apps/backend && go test -v ./...)
        echo -e "${MAGENTA}Running web frontend unit tests (Vitest)...${RESET}"
        pnpm --filter @openchain/web test
        echo -e "${GREEN}✓ Full-stack test suite complete.${RESET}"
        ;;

    clean)
        echo -e "${YELLOW}Cleaning build assets and cache...${RESET}"
        rm -rf apps/web/dist .turbo apps/backend/bin apps/backend/tmp
        echo -e "${GREEN}✓ Clean complete.${RESET}"
        ;;

    *)
        echo "Usage: ./manage.sh {dev|docker|docker:down|build|lint|check|test|clean}"
        exit 1
        ;;
esac
