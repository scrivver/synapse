# Synapse Implementation Plan

## Overview

This plan adds three features to Synapse:

1. **JSONFileProvider** -- a local-file metadata adapter so the reconciler can run without the Engram HTTP API.
2. **synapse-metagen** -- a CLI tool that scans real S3 objects (or local directories) and generates the metadata JSON file.
3. **Event Emitter** -- an adapter-pattern event system so the worker can emit structured events after successful file moves.

All three features follow the existing adapter/port pattern established by `metadata.Provider` and `metadata.EngramClient`.

---

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

Key responsibility boundaries:

- **Reconciler**: reads metadata, detects drift, publishes jobs to the synapse queue. Owns the metadata.Provider dependency.
- **Worker**: consumes jobs from the synapse queue, executes file transfers, emits "job done" events. Does NOT talk to the metadata provider. The worker has no knowledge of metadata -- it only knows about queues, storage, and events.
- **Event Emitter**: the worker's only output beyond the transfer itself. In dev mode, events go to a log file. With Engram, events go to Engram's RabbitMQ queue. Engram is responsible for updating its own metadata when it receives the event.

---

## Feature 1: JSONFileProvider (metadata adapter)

### 1.1 New file: `internal/metadata/jsonfile.go`

Implements `metadata.Provider` (defined in `internal/metadata/metadata.go`) by reading and writing a local JSON file. Used only by the reconciler.

```go
type JSONFileProvider struct {
    path string
    mu   sync.RWMutex
}

func NewJSONFileProvider(path string) *JSONFileProvider

func (p *JSONFileProvider) ListFiles(ctx context.Context) ([]File, error)
func (p *JSONFileProvider) GetFile(ctx context.Context, id string) (*File, error)
func (p *JSONFileProvider) AddLocation(ctx context.Context, fileID, location string) error
```

Internal helpers:

```go
func (p *JSONFileProvider) readAll() ([]File, error)   // reads and parses JSON file
func (p *JSONFileProvider) writeAll(files []File) error // atomic write via temp + rename
```

Key design decisions:

- **Read from disk on every call.** The reconciler calls `ListFiles` once per interval (default 30s). Re-reading is cheap and ensures external edits from `metagen` are picked up without restart.
- **Atomic writes via temp file + `os.Rename`.** Write to a temp file in `filepath.Dir(p.path)` (same filesystem), then rename. Prevents partial reads.
- **`sync.RWMutex`** guards concurrent access within the process. `ListFiles`/`GetFile` take `RLock`; `AddLocation` takes full `Lock`.
- **Missing file returns empty slice**, not an error. This lets the reconciler start before any metadata exists.

Note: `AddLocation` is retained in the Provider interface for completeness (Engram adapter uses it), but in the JSON file workflow, location updates happen via `metagen -merge` or manual edits, not through the reconciler.

### 1.2 Modified file: `internal/config/config.go`

Add to `Config` struct:

```go
MetadataFile string  // env: METADATA_FILE, default: ".data/metadata.json"
```

In `Load()`:

```go
MetadataFile: envOr("METADATA_FILE", ".data/metadata.json"),
```

### 1.3 Modified file: `cmd/synapse-reconciler/main.go`

Currently lines 20-22 require `ENGRAM_API_URL` and exit if unset. Replace:

```go
var meta metadata.Provider
if cfg.EngramAPIURL != "" {
    meta = metadata.NewEngramClient(cfg.EngramAPIURL)
    log.Info("metadata: engram", "url", cfg.EngramAPIURL)
} else {
    meta = metadata.NewJSONFileProvider(cfg.MetadataFile)
    log.Info("metadata: json file", "path", cfg.MetadataFile)
}
```

The reconciler no longer requires `ENGRAM_API_URL` to start.

### 1.4 Modified file: `bin/load-infra-env`

Append after the MinIO block:

```bash
export METADATA_FILE="$DATA_DIR/metadata.json"
echo "METADATA_FILE=$METADATA_FILE"
```

---

## Feature 2: Metadata Generator CLI (synapse-metagen)

### 2.1 New methods on `transfer.S3Client`: `internal/transfer/transfer.go`

Two new methods needed by metagen to enumerate and read S3 objects:

```go
// ListObjects returns all objects in a bucket.
func (s *S3Client) ListObjects(ctx context.Context, bucket string) ([]minio.ObjectInfo, error)

// GetObject returns a reader for the given object. Caller must close it.
func (s *S3Client) GetObject(ctx context.Context, bucket, key string) (*minio.Object, error)
```

`ListObjects` collects from `s.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true})` into a slice. `GetObject` is a thin wrapper around `s.client.GetObject`.

### 2.2 New file: `cmd/synapse-metagen/main.go`

CLI tool using `flag` package.

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-source` | string | `"s3"` | Source type: `dir` or `s3` |
| `-path` | string | `"."` | Directory path (when source=dir) |
| `-bucket` | string | from env | S3 bucket name (defaults to `S3_HOT_BUCKET`) |
| `-tags` | string | `""` | Comma-separated tags to apply to all files |
| `-out` | string | from env | Output file path (defaults to `METADATA_FILE`) |
| `-merge` | bool | `false` | Merge with existing file instead of overwriting |

Behavior for `-source s3`:
- Build `S3Client` from env vars via `config.Load()`.
- Call `s3.ListObjects(ctx, bucket)`.
- For each object: `s3.GetObject(ctx, bucket, key)`, compute SHA-256 via `transfer.SHA256Hash`, capture size from object info.
- Build `metadata.File` with `Locations: []string{bucket}`, `Tags` from flag.

Behavior for `-source dir`:
- Walk directory with `filepath.WalkDir`.
- For each regular file: open, compute SHA-256, get size from `os.Stat`.
- Use `filepath.Rel(path, fullpath)` as file ID.

Merge logic (`-merge`):
- Read existing file at `-out`, index by ID.
- For scanned files: if ID exists and checksum matches, keep existing entry (preserving locations). If checksum differs, update checksum/size but keep locations. If new, add it.

Output: `json.MarshalIndent` to `-out` via atomic temp + rename.

### 2.3 New file: `bin/metagen`

```bash
#!/usr/bin/env bash
set -euo pipefail

if [ -z "${DATA_DIR:-}" ]; then
  echo "ERROR: DATA_DIR is not set. Are you in the dev shell? (nix develop)" >&2
  exit 1
fi

source load-infra-env
exec go run ./cmd/synapse-metagen "$@"
```

---

## Feature 3: Event Emitter (adapter pattern)

### 3.1 New file: `internal/event/event.go`

```go
package event

import (
    "context"
    "time"
)

// MoveCompleted is emitted after a successful file transfer.
type MoveCompleted struct {
    JobID     string    `json:"job_id"`
    FileID    string    `json:"file_id"`
    From      string    `json:"from"`
    To        string    `json:"to"`
    Timestamp time.Time `json:"timestamp"`
}

// Emitter is the port through which the worker publishes domain events.
type Emitter interface {
    EmitMoveCompleted(ctx context.Context, evt MoveCompleted) error
    Close() error
}
```

### 3.2 New file: `internal/event/logfile.go`

Default adapter. Appends JSON lines to a log file.

```go
type LogFileEmitter struct {
    path string
    mu   sync.Mutex
}

func NewLogFileEmitter(path string) *LogFileEmitter

func (e *LogFileEmitter) EmitMoveCompleted(ctx context.Context, evt MoveCompleted) error
// Opens file with O_CREATE|O_APPEND|O_WRONLY, encodes evt as JSON line, closes.

func (e *LogFileEmitter) Close() error
// No-op.
```

### 3.3 New file: `internal/event/engram.go`

Engram adapter. Publishes events to Engram's RabbitMQ queue using its own AMQP connection (separate from the Synapse job queue). Engram is responsible for processing these events and updating its own metadata accordingly.

```go
type EngramEmitter struct {
    conn       *amqp.Connection
    ch         *amqp.Channel
    exchange   string
    routingKey string
}

func NewEngramEmitter(amqpURL, exchange, routingKey string) (*EngramEmitter, error)
// Dials amqpURL, opens channel.

func (e *EngramEmitter) EmitMoveCompleted(ctx context.Context, evt MoveCompleted) error
// Translates MoveCompleted to Engram message format, publishes to exchange with routing key.

func (e *EngramEmitter) Close() error
// Closes channel and connection.
```

### 3.4 Modified file: `internal/config/config.go`

Add to `Config` struct:

```go
EventLogFile     string // env: EVENT_LOG_FILE, default: ".data/events.log"
EngramAMQPURL    string // env: ENGRAM_AMQP_URL, default: "" (empty = disabled)
EngramExchange   string // env: ENGRAM_EXCHANGE, default: "engram.events"
EngramRoutingKey string // env: ENGRAM_ROUTING_KEY, default: "synapse.move"
```

### 3.5 Modified file: `internal/worker/executor.go`

Remove `metadata.Provider` dependency entirely. The worker does not read or write metadata. It only consumes jobs, transfers files, and emits events.

Updated struct:

```go
type Executor struct {
    queue      *job.Queue
    s3         *transfer.S3Client
    emitter    event.Emitter
    maxRetries int
    log        *slog.Logger
}
```

Updated constructor:

```go
func NewExecutor(queue *job.Queue, s3 *transfer.S3Client, emitter event.Emitter, maxRetries int, log *slog.Logger) *Executor
```

In `handle()`, replace the metadata update block with event emission:

```go
// Emit event after successful transfer.
if e.emitter != nil {
    evt := event.MoveCompleted{
        JobID:     j.ID,
        FileID:    j.Payload.FileID,
        From:      j.Payload.From,
        To:        j.Payload.To,
        Timestamp: time.Now(),
    }
    if emitErr := e.emitter.EmitMoveCompleted(ctx, evt); emitErr != nil {
        log.Warn("failed to emit event (non-fatal)", "error", emitErr)
    }
}
```

Remove the `metadata` import entirely from this file.

### 3.6 Modified file: `cmd/synapse-worker/main.go`

Remove all metadata.Provider logic. The worker no longer instantiates or uses a metadata provider.

Add emitter instantiation:

```go
var emitter event.Emitter
if cfg.EngramAMQPURL != "" {
    em, err := event.NewEngramEmitter(cfg.EngramAMQPURL, cfg.EngramExchange, cfg.EngramRoutingKey)
    if err != nil {
        log.Error("failed to create engram emitter", "error", err)
        os.Exit(1)
    }
    defer em.Close()
    emitter = em
    log.Info("event emitter: engram", "exchange", cfg.EngramExchange)
} else {
    emitter = event.NewLogFileEmitter(cfg.EventLogFile)
    log.Info("event emitter: log file", "path", cfg.EventLogFile)
}
```

Updated `NewExecutor` call (no more `meta` parameter):

```go
executor := worker.NewExecutor(queue, s3, emitter, cfg.MaxRetries, log)
```

Remove the `metadata` import from this file.

### 3.7 Modified file: `bin/load-infra-env`

Add:

```bash
export EVENT_LOG_FILE="$DATA_DIR/events.log"
echo "EVENT_LOG_FILE=$EVENT_LOG_FILE"
```

---

## Implementation Order

1. `internal/config/config.go` -- add all new fields (leaf dependency)
2. `internal/metadata/jsonfile.go` -- new adapter
3. `internal/transfer/transfer.go` -- add ListObjects, GetObject
4. `internal/event/event.go` -- interface + struct
5. `internal/event/logfile.go` -- default emitter
6. `internal/event/engram.go` -- engram emitter
7. `cmd/synapse-metagen/main.go` -- CLI tool (depends on 1-3)
8. `internal/worker/executor.go` -- remove metadata, add emitter (depends on 4)
9. `cmd/synapse-worker/main.go` -- remove metadata wiring, add emitter wiring (depends on 4-6, 8)
10. `cmd/synapse-reconciler/main.go` -- add metadata fallback (depends on 2)
11. `bin/metagen`, `bin/load-infra-env` updates

Steps 1-3 are independent. Steps 4-6 are independent. Steps 9-10 are independent.

---

## Dev Workflow (end-to-end)

```bash
nix develop
start-infra
source load-infra-env

# Upload test files to S3
mc cp ./testfiles/* synapse-test/synapse-hot/

# Generate metadata from S3 bucket contents
metagen -source s3 -bucket synapse-hot -tags cold

# Verify metadata was written
cat .data/metadata.json

# Start worker and reconciler
start-worker
start-reconciler

# After reconciliation (default 30s interval), check results
cat .data/events.log    # move completed events emitted by worker
```

The reconciler reads `.data/metadata.json`, finds files tagged "cold" that are not in `synapse-cold`, and enqueues `move_file` jobs to the synapse RabbitMQ queue. The worker consumes those jobs, moves files from `synapse-hot` to `synapse-cold`, and emits a `MoveCompleted` event to `.data/events.log`.

The worker never touches metadata. In production with Engram, the `EngramEmitter` publishes the event to Engram's RabbitMQ queue, and Engram updates its own metadata (e.g., adding the new location).

---

## File Summary

### New files (7)

| File | Purpose |
|------|---------|
| `internal/metadata/jsonfile.go` | JSONFileProvider implementing metadata.Provider |
| `internal/event/event.go` | MoveCompleted struct + Emitter interface |
| `internal/event/logfile.go` | LogFileEmitter (JSON-lines to disk) |
| `internal/event/engram.go` | EngramEmitter (publish to Engram RabbitMQ) |
| `cmd/synapse-metagen/main.go` | Metadata generator CLI |
| `bin/metagen` | Shell wrapper for metagen |

### Modified files (6)

| File | Changes |
|------|---------|
| `internal/config/config.go` | Add MetadataFile, EventLogFile, EngramAMQPURL, EngramExchange, EngramRoutingKey |
| `internal/transfer/transfer.go` | Add ListObjects, GetObject methods |
| `internal/worker/executor.go` | Remove metadata.Provider, add event.Emitter, emit after success |
| `cmd/synapse-worker/main.go` | Remove metadata wiring, add emitter wiring |
| `cmd/synapse-reconciler/main.go` | Add metadata fallback (JSONFileProvider when no Engram) |
| `bin/load-infra-env` | Export METADATA_FILE, EVENT_LOG_FILE |
