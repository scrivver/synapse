# Synapse

The reconciliation engine for the mind palace. Synapse moves data across storage nodes based on metadata-defined conditions, ensuring files are always in the right place.

> “Every movement leaves a trace; make yours worth following.”

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
- **Worker**: consumes jobs from RabbitMQ, transfers files between storage locations (S3 or filesystem) with checksum verification, emits events on completion.
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
  transfer/                Mover/Scanner interfaces + S3 and filesystem backends
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

## Step-by-Step (S3 Backend)

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

## Step-by-Step (Filesystem Backend)

```bash
nix develop
start-infra
source load-infra-env

# 1. Switch to filesystem backend
export STORAGE_BACKEND=fs

# 2. Place files in the hot storage directory
cp ./some-file.txt .data/storage/synapse-hot/

# 3. Generate metadata
metagen -source fs -bucket synapse-hot -tags cold

# 4. Start worker and reconciler
start-worker
start-reconciler

# 5. Check results after ~30s
cat .data/events.log
cat .data/metadata.json
ls .data/storage/synapse-cold/   # files moved here
ls .data/storage/synapse-hot/    # source files removed
```

With the filesystem backend, "buckets" are subdirectories under `STORAGE_FS_ROOT`
(default `.data/storage/`). The directories `synapse-hot/` and `synapse-cold/` are
created automatically by the dev shell.

## Storage Backends

Synapse supports two storage backends, selected via `STORAGE_BACKEND`:

| Backend | Value | Description |
|---------|-------|-------------|
| S3 | `s3` (default) | Uses MinIO or any S3-compatible service. Configured via `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`. |
| Filesystem | `fs` | Uses local directories under `STORAGE_FS_ROOT`. Each bucket maps to a subdirectory. No S3 required. |

Both backends provide the same guarantees: checksum-verified transfers, idempotent
moves, and source deletion after success.

## Metagen CLI

`metagen` (or `go run ./cmd/synapse-metagen`) scans files from a storage backend
or local directory and generates the metadata JSON that the reconciler reads.

### Usage

```bash
metagen [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-source` | `s3` | Source to scan: `s3`, `fs`, or `dir` |
| `-bucket` | `$S3_HOT_BUCKET` | Bucket/location name to scan |
| `-path` | `.` | Local directory path (only for `-source dir`) |
| `-tags` | _(empty)_ | Comma-separated tags to assign to all files |
| `-out` | `$METADATA_FILE` | Output file path |
| `-merge` | `false` | Merge with existing metadata instead of overwriting |

### Sources

- **`s3`** — Scans an S3 bucket. Requires running MinIO and `S3_*` env vars.
- **`fs`** — Scans a subdirectory under `STORAGE_FS_ROOT`. Uses the filesystem scanner.
- **`dir`** — Scans an arbitrary local directory. Useful for initial import of files.

### Examples

```bash
# Scan S3 hot bucket, tag everything as "cold"
metagen -source s3 -bucket synapse-hot -tags cold

# Scan filesystem hot directory
metagen -source fs -bucket synapse-hot -tags cold

# Import files from an arbitrary directory into metadata
metagen -source dir -path ~/documents -bucket synapse-hot -tags hot,important

# Update metadata without losing existing locations
metagen -source s3 -bucket synapse-hot -tags cold -merge

# Write to a custom output file
metagen -source s3 -bucket synapse-hot -tags cold -out /tmp/meta.json
```

### How It Works

1. Scans the source, computing SHA-256 checksums and recording file sizes.
2. Builds `metadata.File` entries with the specified tags and location.
3. Writes the JSON to `-out` (default `.data/metadata.json`).
4. With `-merge`, preserves existing entries and their locations — only
   updates checksums/sizes for files that changed, and adds new files.

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
| `STORAGE_BACKEND` | `s3` | Storage backend: `s3` or `fs` |
| `STORAGE_FS_ROOT` | `.data/storage` | Base path for filesystem backend |
| `RECONCILE_INTERVAL` | `30s` | How often the reconciler runs |
