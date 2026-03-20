# CLAUDE.md

## Project Overview

Synapse is a reconciliation-driven job system written in Go. It moves files between S3-compatible storage buckets based on metadata from Engram (an external metadata service). Part of a larger system: Engram (metadata), Reliquary (storage), Synapse (movement engine).

## Build & Run

```bash
nix develop          # enter dev shell with all dependencies
go build ./...       # build all packages
go vet ./...         # lint
go run ./cmd/synapse-worker      # run the worker
go run ./cmd/synapse-reconciler  # run the reconciler
go run ./cmd/synapse-metagen     # generate metadata from files
```

## Dev Environment

Infrastructure (RabbitMQ + MinIO) is managed via Nix + process-compose:
- `start-infra` / `shutdown-infra` to manage services
- `source load-infra-env` to export dynamic ports
- All runtime data lives in `.data/` (gitignored)

## Architecture Patterns

- **Adapter pattern** for metadata: `metadata.Provider` interface with `EngramClient` (HTTP) and `JSONFileProvider` (local JSON file) adapters. Reconciler uses this to read file state.
- **Adapter pattern** for events: `event.Emitter` interface with `EngramEmitter` (RabbitMQ), `LogFileEmitter` (JSON-lines), and `DevEmitter` (log + metadata update) adapters. Worker uses this to emit events after successful transfers.
- **Worker does NOT depend on metadata** — it only consumes jobs from the queue, transfers files, and emits events. The feedback loop to metadata is the event consumer's responsibility.
- **Reconciler does NOT transfer files** — it only reads metadata, detects drift, and publishes jobs to the queue.

## Key Design Decisions

- Idempotent file transfers: MoveFile checks destination exists + verifies checksum before skipping
- Source deletion: source file is removed from S3 after verified transfer
- Manual ack with retry: failed jobs are republished with incremented retry count, sent to DLQ after max retries
- Streaming transfers: source is piped through SHA-256 hash directly into upload, no temp files
- Dev mode feedback loop: DevEmitter updates JSONFileProvider locations so reconciler doesn't re-enqueue completed jobs

## Code Style

- Go standard library conventions
- `log/slog` for structured logging
- Environment variables for configuration (no config files)
- Minimal dependencies: minio-go, amqp091-go, google/uuid
