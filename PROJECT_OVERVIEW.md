# CDC Platform Overview

## Project Structure

- .env: Default environment variables for local runs.
- .env.*: Per-subscriber environment files (filters, names, etc.).
- cmd/
  - emitter/: MySQL binlog emitter → Redis Pub/Sub.
  - subscribers/: Multi-subscriber runner (one binary).
  - subscribers/console/: Single-subscriber console runner.
- internal/
  - event/: Shared event schema.
  - subscriber/: Shared subscriber config, env parsing, filters, and runner.
- go.mod / go.sum: Go module and dependencies.

## Flow (End-to-End)

1. Emitter connects to MySQL, reads binlog events, builds row-change events.
2. Emitter publishes each event to a Redis Pub/Sub channel.
3. Subscriber runner loads one or more env files, each representing a subscriber.
4. Each subscriber applies its own filters and processes matching events.

## Components

### Emitter

- Reads MySQL binlog in real time.
- Emits RowEvent payloads to Redis Pub/Sub.

### Subscribers

- Each subscriber has its own env file with filters.
- The multi-subscriber binary runs multiple subscribers concurrently.

## Configuration (Key Vars)

Emitter:
- DB_USER, DB_PASS, DB_HOST, DB_PORT, DB_NAME, SERVER_ID
- REDIS_ADDR, REDIS_PASS, REDIS_DB, REDIS_CHANNEL

Subscribers:
- SUBSCRIBER_NAME
- REDIS_ADDR, REDIS_PASS, REDIS_DB, REDIS_CHANNEL
- FILTER_DBS, FILTER_TABLES, FILTER_IDS, FILTER_OPS
- FILTER_CHANGE_ANY, FILTER_CHANGE_ALL

## Run Summary

- Build multi-subscriber binary: go build -o cdc-subscribers ./cmd/subscribers
- Run with env files: ENV_FILES=./.env.lead_events,./.env.user_events ./cdc-subscribers
