# Synapse

The reconciliation engine for the mind palace. Synapse moves data across storage nodes based on metadata-defined conditions, ensuring files are always in the right place.

## Architecture

```
Reconciler                              Worker
    |                                       |
    | reads                                 | consumes
    v                                       v
metadata.Provider               job.Queue (synapse.jobs)
(Engram API or JSON file)               |
    |                                       | executes
    | detects drift                         v
    v                               transfer.S3Client
job.Queue (synapse.jobs)            (move file between buckets)
    |                                       |
    | publishes move_file jobs              | on success
    v                                       v
    +--> RabbitMQ ---->             event.Emitter
                                    (log file or Engram queue)
```

- **Reconciler**: reads file metadata, detects drift (e.g. file tagged "cold" not in cold storage), publishes `move_file` jobs.
- **Worker**: consumes jobs from RabbitMQ, transfers files between S3 buckets with checksum verification, emits events on completion.
- **Metadata**: adapter pattern — `EngramClient` (HTTP) for production, `JSONFileProvider` (local file) for standalone dev.
- **Events**: adapter pattern — `EngramEmitter` (RabbitMQ) for production, `DevEmitter` (log file + metadata update) for dev.

## Project Structure

```
cmd/
  synapse-worker/          worker entrypoint
  synapse-reconciler/      reconciler entrypoint
  synapse-metagen/         CLI to generate metadata from files
internal/
  config/                  env-based configuration
  job/                     job model + RabbitMQ queue
  transfer/                S3 file transfer + checksum
  metadata/                Provider interface + adapters (Engram, JSON file)
  event/                   Emitter interface + adapters (Engram, log file, dev)
  worker/                  job executor
  reconciler/              reconciliation loop + rules
infra/                     nix service definitions (RabbitMQ, MinIO)
shells/                    nix dev shell definitions
bin/                       dev scripts
```

## Prerequisites

- [Nix](https://nixos.org/) with flakes enabled

## Quick Start

```bash
# Enter the dev shell
nix develop

# Start everything (infra + reconciler + worker) in tmux
dev
```

## Step-by-Step

```bash
nix develop

# 1. Start infra (RabbitMQ + MinIO)
start-infra

# 2. Load dynamic ports into your shell
source load-infra-env

# 3. Upload test files to the hot bucket
mc alias set synapse-test "http://127.0.0.1:$MINIO_API_PORT" minioadmin minioadmin --api S3v4
mc cp ./some-file.txt synapse-test/synapse-hot/

# 4. Generate metadata (tags files as "cold" so reconciler moves them)
metagen -source s3 -bucket synapse-hot -tags cold

# 5. Start worker and reconciler
start-worker       # in one terminal
start-reconciler   # in another terminal

# 6. Check results after ~30s
cat .data/events.log       # move events
cat .data/metadata.json    # updated locations
mc ls synapse-test/synapse-cold/  # files moved here
```

## Dev Scripts

| Script | Description |
|--------|-------------|
| `dev` | Start infra + reconciler + worker in tmux |
| `start-infra` | Start RabbitMQ + MinIO via process-compose |
| `shutdown-infra` | Stop all infra services |
| `source load-infra-env` | Export dynamic ports into current shell |
| `start-worker` | Launch worker in a tmux window |
| `start-reconciler` | Launch reconciler in a tmux window |
| `metagen` | Generate metadata JSON from S3 or local files |
| `test-job` | Submit a test move_file job manually |

## Configuration

All configuration is via environment variables. `load-infra-env` sets these automatically for local dev.

| Variable | Default | Description |
|----------|---------|-------------|
| `RABBITMQ_URL` | `amqp://guest:guest@127.0.0.1:5672` | AMQP connection |
| `S3_ENDPOINT` | `http://127.0.0.1:9000` | MinIO/S3 endpoint |
| `S3_ACCESS_KEY` | `minioadmin` | S3 access key |
| `S3_SECRET_KEY` | `minioadmin` | S3 secret key |
| `S3_HOT_BUCKET` | `synapse-hot` | Hot storage bucket |
| `S3_COLD_BUCKET` | `synapse-cold` | Cold storage bucket |
| `ENGRAM_API_URL` | _(empty)_ | Engram HTTP API (enables Engram metadata adapter) |
| `ENGRAM_AMQP_URL` | _(empty)_ | Engram RabbitMQ (enables Engram event emitter) |
| `METADATA_FILE` | `.data/metadata.json` | JSON file for standalone metadata |
| `EVENT_LOG_FILE` | `.data/events.log` | Event log for dev mode |
| `RECONCILE_INTERVAL` | `30s` | How often the reconciler runs |
