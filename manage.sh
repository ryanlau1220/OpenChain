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
        if [ -z "${ETHEREUM_MAINNET_RPC_URL:-}" ] || [ -z "${BASE_MAINNET_RPC_URL:-}" ] || [ -z "${ETHERSCAN_API_KEY:-}" ]; then
            echo -e "${RED}ETHEREUM_MAINNET_RPC_URL, BASE_MAINNET_RPC_URL, and ETHERSCAN_API_KEY are required.${RESET}"
            exit 1
        fi

        export GOCACHE="${GOCACHE:-/tmp/openchain-go-cache}"

        web_port="${WEB_ORIGIN##*:}"
        if ! [[ "${web_port}" =~ ^[0-9]+$ ]]; then
            web_port=3000
        fi

        # Free configured ports from lingering development processes
        fuser -k "${PORT:-8081}/tcp" "${web_port}/tcp" 2>/dev/null || true

        trap cleanup_dev INT TERM

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
        echo -e "${MAGENTA}Launching OpenChain TanStack Start Web App (Port ${web_port})...${RESET}"
        (cd apps/web && pnpm exec vite dev --strictPort --port "${web_port}" 2>&1 | stdbuf -oL sed "s/^/$(printf "${MAGENTA}[web]${RESET}") /") &
        wait

        ;;


    docker)
        echo -e "${YELLOW}Building and starting infrastructure Docker Compose containers...${RESET}"
        docker compose --env-file .env -f infra/docker-compose.yml up -d --build --remove-orphans
        echo -e "${GREEN}✓ [OK] Infrastructure containers started successfully.${RESET}"
        ;;


    docker:down)
        echo -e "${YELLOW}Stopping Docker Compose stack...${RESET}"
        docker compose --env-file .env -f infra/docker-compose.yml down
        echo -e "${GREEN}✓ [OK] Docker containers stopped.${RESET}"
        ;;

    docker:prod)
        echo -e "${YELLOW}Building and starting the production Docker Compose stack...${RESET}"
        docker compose --env-file .env -f infra/docker-compose.production.yml up -d --build --remove-orphans
        echo -e "${GREEN}✓ [OK] Production stack started successfully.${RESET}"
        ;;

    docker:prod:down)
        echo -e "${YELLOW}Stopping production Docker Compose stack...${RESET}"
        docker compose --env-file .env -f infra/docker-compose.production.yml down
        echo -e "${GREEN}✓ [OK] Production containers stopped. Persistent volumes were kept.${RESET}"
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
        echo -e "${CYAN}Running Go backend unit and integration tests...${RESET}"
        (cd apps/backend && GOCACHE="${GOCACHE:-/tmp/openchain-go-cache}" go test -v ./...)
        echo -e "${MAGENTA}Running web frontend unit tests (Vitest)...${RESET}"
        env -u NODE_OPTIONS pnpm --filter @openchain/web test
        echo -e "${GREEN}✓ Full-stack test suite complete.${RESET}"
        ;;

    test:e2e)
        api_url="http://localhost:${PORT:-8081}/api/v1/health"
        web_url="${WEB_ORIGIN:-http://localhost:3000}"
        echo -e "${CYAN}Checking the running backend and web application...${RESET}"
        if ! curl --fail --silent --show-error "${api_url}" | rg -q '"status":"healthy"'; then
            echo -e "${RED}Backend health check failed. Start the stack with ./manage.sh docker and ./manage.sh dev first.${RESET}"
            exit 1
        fi
        if ! curl --fail --silent --show-error "${web_url}" | rg -q '<title>OpenChain'; then
            echo -e "${RED}Web smoke check failed. Start the stack with ./manage.sh dev first.${RESET}"
            exit 1
        fi
        echo -e "${GREEN}✓ Live backend and web smoke checks passed.${RESET}"
        ;;

    clean)
        echo -e "${YELLOW}Cleaning build assets and cache...${RESET}"
        rm -rf apps/web/dist .turbo apps/backend/bin apps/backend/tmp
        echo -e "${GREEN}✓ Clean complete.${RESET}"
        ;;

    *)
        echo "Usage: ./manage.sh {dev|docker|docker:down|docker:prod|docker:prod:down|build|lint|check|test|test:e2e|clean}"
        exit 1
        ;;
esac
