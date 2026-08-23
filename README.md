# OpenChain

OpenChain is a local-first blockchain investigation workspace for tracing public on-chain activity. It turns provider responses into a navigable transfer graph while keeping the evidence, retrieval scope, and known limitations visible.

The production instance is available at [openchain.duckdns.org](https://openchain.duckdns.org).

## What it does

- Looks up addresses and traces native, token, and—in supported EVM networks—internal transfers.
- Builds an interactive, multi-hop fund-flow graph with direction, depth, time, asset, amount, and transfer-type filters.
- Persists observed transfers in PostgreSQL and uses Apache AGE for graph traversal.
- Preserves immutable provider acquisition snapshots, page/cursor scope, response hashes, and retrieval times for evidence review and replay.
- Shows trace coverage separately for the freshly retrieved page and the durable graph projection.
- Identifies deterministic graph patterns such as fan-in, fan-out, rapid onward transfers, repeated equal-amount dispersal, sequential decreasing transfers, and brief intermediary pass-through. These are findings, not risk scores.
- Displays reviewed, versioned entity assertions with their source evidence; investigator annotations and deterministic findings remain separate from verified labels.
- Creates local browser-stored investigation cases and exports/imports frozen JSON evidence packages for offline verification and replay.
- Resolves exact Ethereum-to-Base and Ethereum-to-Optimism Standard Bridge continuations when the message ID, canonical token route, confirmation policy, and on-chain evidence match. It does not infer ownership, control, or intent.
- Shows the exact capabilities available for the selected network. Unsupported data is shown as unavailable or unknown rather than inferred.

## Supported networks

| Network | Implemented tracing |
| --- | --- |
| Ethereum, Base, Polygon, Arbitrum, Optimism, BNB Chain | Native and token transfers, internal transfers, historical pagination, finality, entity assertions, and provenance. Ethereum, Base, and Optimism also expose verified Standard Bridge evidence where applicable. |
| Solana, TRON | Native and token historical tracing, with adapter-specific capability limits shown in the UI. |
| TON, Cardano | Native historical tracing only. |

Transaction execution status is deliberately `unknown` until a provider can establish it exactly.

## Architecture

- **Backend:** Go, ConnectRPC, Protobuf, Buf
- **Web:** TanStack Start, TypeScript, Cytoscape.js
- **Data:** PostgreSQL with Apache AGE
- **Operations:** Docker Compose, Prometheus, Grafana, Telegraf, Cloudflare R2 backups

The PostgreSQL transfer store is the authoritative observation record. Apache AGE is a traversal projection, not a replacement for the underlying evidence.

## Run locally

Prerequisites: Docker with Compose, Go, Node.js, and pnpm.

```bash
cp .env.example .env
# Fill the required provider credentials and RPC URLs in .env.

./manage.sh docker  # PostgreSQL + Apache AGE
./manage.sh dev     # migrations, backend, and web app
```

The default local web origin is `http://localhost:3002`; the API health endpoint is `http://localhost:8091/api/v1/health`.

## Verify changes

```bash
./manage.sh check      # formatting and web build
./manage.sh test       # Go, web, and available database integration tests
./manage.sh test:e2e   # deterministic browser tests against a running local stack
./manage.sh smoke      # local web/API and controlled queue checks
```

Live provider validation is intentionally separate from CI:

```bash
./manage.sh providers:acceptance:prod
```

It is a read-only production check that rebuilds its manual image first, then validates configured provider credentials, pagination, capabilities, and evidence expectations against known addresses.

## Production operations

Production uses `.env.prod`, which must remain outside Git. The management script includes commands for deployment, backups, and edge configuration:

```bash
./manage.sh docker:prod
./manage.sh backup:prod
./manage.sh edge:prod
```

Production backups are compressed PostgreSQL logical dumps, verified before upload to Cloudflare R2. Prometheus, Grafana, node exporter, and Telegraf monitor service health, provider failures, queue state, disk pressure, and backup age. Nginx Proxy Manager rate-limits public API and RPC routes before they reach the backend.
