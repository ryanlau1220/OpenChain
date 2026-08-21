#!/bin/bash

# OpenChain Platform Management Script

set -e
set -o pipefail

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

backup_database() {
    local compose_file="$1"
    local env_file="$2"
    if [ ! -f "${env_file}" ]; then
        echo -e "${RED}Environment file ${env_file} was not found.${RESET}"
        exit 1
    fi
    set -a
    source "${env_file}"
    set +a
    local retention_days="${BACKUP_RETENTION_DAYS:-14}"
    if ! [[ "${retention_days}" =~ ^[0-9]+$ ]] || [ "${retention_days}" -lt 1 ]; then
        echo -e "${RED}BACKUP_RETENTION_DAYS must be a positive integer.${RESET}"
        exit 1
    fi
    local backup_dir="${BACKUP_DIR:-infra/backups}"
    mkdir -p "${backup_dir}"
    local timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
    local backup_file="${backup_dir}/openchain-${timestamp}.sql.gz"
    local partial_file="${backup_file}.partial"
    echo -e "${CYAN}Creating PostgreSQL logical backup...${RESET}"
    if ! docker compose --env-file "${env_file}" -f "${compose_file}" exec -T postgres sh -ec 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' | gzip -c > "${partial_file}"; then
        rm -f "${partial_file}"
        echo -e "${RED}Backup failed; no partial backup was retained.${RESET}"
        exit 1
    fi
    mv "${partial_file}" "${backup_file}"
    find "${backup_dir}" -maxdepth 1 -type f -name 'openchain-*.sql.gz' -mtime +"${retention_days}" -delete
    echo -e "${GREEN}✓ Backup created at ${backup_file}; backups older than ${retention_days} days were removed.${RESET}"
}

case "$1" in
    dev)
        if [ -z "${ETHEREUM_MAINNET_RPC_URL:-}" ] || [ -z "${BASE_MAINNET_RPC_URL:-}" ] || [ -z "${SOLANA_MAINNET_RPC_URL:-}" ] || [ -z "${ETHERSCAN_API_KEY:-}" ] || [ -z "${BLOCKSCOUT_API_KEY:-}" ] || [ -z "${ALCHEMY_API_KEY:-}" ] || [ -z "${TRONGRID_API_KEY:-}" ] || [ -z "${TONAPI_KEY:-}" ] || [ -z "${BLOCKFROST_PROJECT_ID:-}" ] || [ -z "${QUEUE_CLIENT_SECRET:-}" ]; then
            echo -e "${RED}ETHEREUM_MAINNET_RPC_URL, BASE_MAINNET_RPC_URL, SOLANA_MAINNET_RPC_URL, ETHERSCAN_API_KEY, BLOCKSCOUT_API_KEY, ALCHEMY_API_KEY, TRONGRID_API_KEY, TONAPI_KEY, BLOCKFROST_PROJECT_ID, and QUEUE_CLIENT_SECRET are required.${RESET}"
            exit 1
        fi

        export GOCACHE="${GOCACHE:-/tmp/openchain-go-cache}"

        echo -e "${CYAN}Applying database migrations...${RESET}"
        go run ./apps/backend/cmd/migrate

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
        docker compose --env-file .env.prod -f infra/docker-compose.production.yml up -d --build --remove-orphans
        echo -e "${GREEN}✓ [OK] Production stack started successfully.${RESET}"
        ;;

    docker:prod:down)
        echo -e "${YELLOW}Stopping production Docker Compose stack...${RESET}"
        docker compose --env-file .env.prod -f infra/docker-compose.production.yml down
        echo -e "${GREEN}✓ [OK] Production containers stopped. Persistent volumes were kept.${RESET}"
        ;;

    backup)
        backup_database infra/docker-compose.yml .env
        ;;

    backup:prod)
        backup_database infra/docker-compose.production.yml .env.prod
        ;;

    migrate)
        echo -e "${CYAN}Applying database migrations...${RESET}"
        GOCACHE="${GOCACHE:-/tmp/openchain-go-cache}" go run ./apps/backend/cmd/migrate
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
        npx biome check apps/web
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
        db_integration_test=0
        if docker compose --env-file .env -f infra/docker-compose.yml exec -T postgres pg_isready -q -U "${POSTGRES_USER:-openchain}" -d "${POSTGRES_DB:-openchain}" >/dev/null 2>&1; then
            db_integration_test=1
            echo -e "${CYAN}PostgreSQL is ready; including Apache AGE integration tests.${RESET}"
        else
            echo -e "${YELLOW}PostgreSQL is not running; skipping Apache AGE integration tests.${RESET}"
        fi
        (cd apps/backend && OPENCHAIN_DB_INTEGRATION_TEST="${db_integration_test}" GOCACHE="${GOCACHE:-/tmp/openchain-go-cache}" go test -v ./...)
        echo -e "${MAGENTA}Running web frontend unit tests (Vitest)...${RESET}"
        env -u NODE_OPTIONS pnpm --filter @openchain/web test
        echo -e "${GREEN}✓ Full-stack test suite complete.${RESET}"
        ;;

    test:e2e)
        api_url="http://localhost:${PORT:-8081}/api/v1/health"
        web_url="${WEB_ORIGIN:-http://localhost:3000}"
        echo -e "${CYAN}Checking the running backend and web application...${RESET}"
        if ! curl --connect-timeout 2 --max-time 10 --fail --silent --show-error "${api_url}" >/dev/null; then
            echo -e "${RED}Backend health check failed. Start the stack with ./manage.sh docker and ./manage.sh dev first.${RESET}"
            exit 1
        fi
        if ! curl --connect-timeout 2 --max-time 10 --fail --silent --show-error "${web_url}" | rg -q '<title>OpenChain'; then
            echo -e "${RED}Web smoke check failed. Start the stack with ./manage.sh dev first.${RESET}"
            exit 1
        fi
        OPENCHAIN_E2E_BASE_URL="${web_url}" pnpm --filter @openchain/web test:e2e
        echo -e "${GREEN}✓ Live backend and web end-to-end checks passed.${RESET}"
        ;;

    smoke)
        if [ -n "${OPENCHAIN_SMOKE_URL:-}" ]; then
            smoke_web_url="${OPENCHAIN_SMOKE_URL%/}"
            smoke_health_url="${smoke_web_url}/api/v1/health"
        else
            smoke_web_url="${WEB_ORIGIN:-http://localhost:3000}"
            smoke_health_url="http://localhost:${PORT:-8081}/api/v1/health"
        fi
        echo -e "${CYAN}Checking public web and API routes...${RESET}"
        if ! curl --connect-timeout 2 --max-time 10 --fail --silent --show-error "${smoke_web_url}" | rg -q '<title>OpenChain'; then
            echo -e "${RED}Public web smoke check failed at ${smoke_web_url}.${RESET}"
            exit 1
        fi
        smoke_health_file="$(mktemp)"
        if ! smoke_status="$(curl --connect-timeout 2 --max-time 10 --silent --show-error --output "${smoke_health_file}" --write-out '%{http_code}' "${smoke_health_url}")"; then
            rm -f "${smoke_health_file}"
            echo -e "${RED}Public API smoke check failed at ${smoke_health_url}.${RESET}"
            exit 1
        fi
        if [ "${smoke_status}" != "200" ] && [ "${smoke_status}" != "503" ]; then
            rm -f "${smoke_health_file}"
            echo -e "${RED}Public API smoke check failed at ${smoke_health_url} (HTTP ${smoke_status}).${RESET}"
            exit 1
        fi
        if ! rg -q '"service":"openchain-api"' "${smoke_health_file}"; then
            rm -f "${smoke_health_file}"
            echo -e "${RED}Public API returned an invalid health response.${RESET}"
            exit 1
        fi
        rm -f "${smoke_health_file}"

        if ! command -v k6 >/dev/null 2>&1; then
            echo -e "${RED}k6 is required for smoke tests. Install k6 locally, then rerun ./manage.sh smoke.${RESET}"
            exit 1
        fi

        if ! docker compose --env-file .env -f infra/docker-compose.yml exec -T postgres pg_isready -q -U "${POSTGRES_USER:-openchain}" -d "${POSTGRES_DB:-openchain}" >/dev/null 2>&1; then
            echo -e "${RED}PostgreSQL is required for controlled queue smoke tests. Start it with ./manage.sh docker first.${RESET}"
            exit 1
        fi

        load_test_port="${OPENCHAIN_LOAD_TEST_PORT:-18091}"
        load_vus="${OPENCHAIN_K6_LOAD_VUS:-5}"
        if ! [[ "${load_test_port}" =~ ^[1-9][0-9]*$ ]] || ! [[ "${load_vus}" =~ ^[1-9][0-9]*$ ]]; then
            echo -e "${RED}OPENCHAIN_LOAD_TEST_PORT and OPENCHAIN_K6_LOAD_VUS must be positive integers.${RESET}"
            exit 1
        fi
        load_test_binary="$(mktemp /tmp/openchain-loadtest.XXXXXX)"
        load_test_pid=""
        cleanup_smoke() {
            if [ -n "${load_test_pid}" ] && kill -0 "${load_test_pid}" 2>/dev/null; then
                kill "${load_test_pid}" 2>/dev/null || true
                wait "${load_test_pid}" 2>/dev/null || true
            fi
            rm -f "${load_test_binary}"
        }
        trap cleanup_smoke EXIT INT TERM

        echo -e "${CYAN}Building isolated controlled-provider test server...${RESET}"
        GOCACHE="${GOCACHE:-/tmp/openchain-go-cache}" go build -o "${load_test_binary}" ./apps/backend/cmd/loadtest
        OPENCHAIN_LOAD_TEST_PORT="${load_test_port}" \
            OPENCHAIN_LOAD_TEST_REQUESTS_PER_MINUTE=8 \
            OPENCHAIN_LOAD_TEST_MAX_QUEUED_JOBS_PER_NETWORK=4 \
            OPENCHAIN_LOAD_TEST_MAX_QUEUED_JOBS_PER_CLIENT_PER_NETWORK=2 \
            OPENCHAIN_LOAD_TEST_PROVIDER_DELAY_MS=250 \
            "${load_test_binary}" >/tmp/openchain-loadtest.log 2>&1 &
        load_test_pid="$!"
        for _ in {1..50}; do
            if curl --connect-timeout 1 --max-time 1 --fail --silent "http://127.0.0.1:${load_test_port}/api/v1/health" >/dev/null 2>&1; then
                break
            fi
            sleep 0.1
        done
        if ! curl --connect-timeout 1 --max-time 1 --fail --silent "http://127.0.0.1:${load_test_port}/api/v1/health" >/dev/null 2>&1; then
            echo -e "${RED}Controlled provider test server did not start. See /tmp/openchain-loadtest.log.${RESET}"
            exit 1
        fi

        echo -e "${CYAN}Running deterministic k6 queue, limiter, polling, API, and UI smoke...${RESET}"
        OPENCHAIN_K6_WEB_URL="${smoke_web_url}" \
            OPENCHAIN_K6_API_URL="http://127.0.0.1:${load_test_port}" \
            OPENCHAIN_K6_REQUEST_LIMIT=8 \
            OPENCHAIN_K6_PER_CLIENT_QUEUE_LIMIT=2 \
            k6 run infra/k6/smoke.js
        echo -e "${CYAN}Running controlled k6 latency profile (${load_vus} VUs; ${OPENCHAIN_K6_LOAD_DURATION:-10s})...${RESET}"
        OPENCHAIN_K6_WEB_URL="${smoke_web_url}" \
            OPENCHAIN_K6_API_URL="http://127.0.0.1:${load_test_port}" \
            OPENCHAIN_K6_LOAD_VUS="${load_vus}" \
            OPENCHAIN_K6_LOAD_DURATION="${OPENCHAIN_K6_LOAD_DURATION:-10s}" \
            k6 run infra/k6/load.js
        echo -e "${GREEN}✓ Native k6 checks passed: UI/API latency, error rate, shared-IP limits, queue fairness, saturation, polling, and provider-stub traces.${RESET}"
        ;;

    clean)
        echo -e "${YELLOW}Cleaning build assets and cache...${RESET}"
        rm -rf apps/web/dist .turbo apps/backend/bin apps/backend/tmp
        echo -e "${GREEN}✓ Clean complete.${RESET}"
        ;;

    *)
        echo "Usage: ./manage.sh {dev|docker|docker:down|docker:prod|docker:prod:down|backup|backup:prod|migrate|build|lint|check|test|test:e2e|smoke|clean}"
        exit 1
        ;;
esac
