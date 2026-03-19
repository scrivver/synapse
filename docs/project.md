# Synapse — Implementation Plan

## 🧠 Overview

**Synapse** is a reconciliation-driven job system responsible for moving data across nodes based on metadata-defined conditions.

It operates as part of a larger system:

* **Engram** → metadata (source of truth)
* **Reliquary** → storage layer
* **Synapse** → movement & consistency engine

---

## 🎯 Scope Definition (Critical)

### ✅ Synapse IS responsible for:

* Generating file movement jobs from metadata
* Executing file transfers reliably
* Ensuring data integrity (checksum, retries)
* Maintaining consistency between metadata and actual storage

### ❌ Synapse is NOT:

* A full orchestrator (no global cluster control)
* A storage system
* A policy DSL engine
* A user-facing system

---

## 🏗️ Architecture

```
Engram (metadata)
        ↓
Reconciler (Synapse brain)
        ↓
RabbitMQ (job queue)
        ↓
Workers (execution engine)
        ↓
File transfer + metadata update
```

---

## 🧩 Components

### 1. Metadata Layer (Engram)

Stores:

* file_id
* locations
* tags
* checksum
* size
* last_accessed

**Constraint:**

* Must remain passive (no decision logic)

---

### 2. Reconciler (Decision Engine)

Runs periodically:

* Scans metadata
* Detects mismatches between desired vs actual state
* Enqueues jobs

Example:

```pseudo
if file.tag == "cold" and not in cold_storage:
    enqueue(move_file)
```

---

### 3. Job Queue (RabbitMQ)

Queue:

```
synapse.jobs
```

Optional:

```
synapse.jobs.dlq
```

Message format:

```json
{
  "job_id": "uuid",
  "type": "move_file",
  "payload": {
    "file_id": "f1",
    "from": "node-a",
    "to": "node-b"
  },
  "retry": 0
}
```

---

### 4. Worker (Execution Engine)

Responsibilities:

* Consume jobs
* Execute file transfer
* Verify integrity
* Update metadata
* Handle retry logic

---

### 5. Transfer Layer

Handles:

* file download/upload
* checksum verification
* retry handling

---

## ⚙️ Tech Stack

### Core

* Language: **Go**
* Messaging: **RabbitMQ**
* Metadata: **Engram API (HTTP)**
* Storage: existing nodes or object storage

---

### Transfer Options

Start simple:

* HTTP-based transfer

Optional:

* S3-compatible API (for object storage)

---

### Infra

* RabbitMQ (already available)
* Multiple worker nodes (distributed)
* One or more reconciler instances
* Storage nodes (hot/cold)

---

## 📦 Repo Structure

```
/cmd
  /synapse-worker
  /synapse-reconciler

/internal
  /job
    model.go
    queue.go

  /worker
    executor.go

  /reconciler
    loop.go
    rules.go

  /transfer
    transfer.go
    checksum.go

  /metadata
    client.go
```

---

## 🔄 Core Job Flow

### move_file Execution

```
1. Fetch job
2. Check if destination already has file
   - If yes → verify checksum → exit (idempotent)
3. Download from source
4. Upload to destination
5. Verify checksum
6. Update metadata (add new location)
7. (Optional) remove old location
8. Ack job
```

---

## ⚠️ Critical Design Constraints

### 1. Idempotency

Jobs must be safe to run multiple times.

```
if file exists at destination:
    verify checksum
    skip or repair
```

---

### 2. At-least-once Delivery

RabbitMQ guarantees:

* message may be delivered multiple times

System must tolerate duplicates.

---

### 3. Metadata Consistency

Only update metadata AFTER successful transfer.

---

### 4. Retry Strategy

```
max_retry = 3–5
exponential backoff (optional)
```

---

### 5. Partial Failures

Start simple:

* restart transfer on failure

---

## 🚀 Implementation Phases

---

### Phase 1 — Single Job Execution

Goal: Move one file reliably

* Hardcode job
* Implement worker execution
* Add checksum verification
* Update metadata after success

---

### Phase 2 — Queue Integration

* Connect to RabbitMQ
* Consume jobs
* Manual ACK
* Basic retry logic

---

### Phase 3 — Reconciler

* Periodic loop
* Scan metadata
* Generate move jobs

---

### Phase 4 — Rules

* Tag-based movement (e.g. cold storage)
* Basic conditions

---

### Phase 5 — Distributed Workers

* Run workers on multiple nodes
* Enable remote transfer
* Handle failures across nodes

---

### Phase 6 — Reliability Improvements

* Dead letter queue
* Retry limits
* Logging + observability

---

## 🔒 RabbitMQ Configuration

### Queue

* durable: true

### Worker Settings

* manual ack
* prefetch_count: 1–5

### Retry

* increment retry in message
* republish on failure

---

## 📈 Future Enhancements (Optional)

* replication jobs
* bandwidth throttling
* priority queues
* job deduplication
* delay queues for retry
* monitoring dashboard

---

## ❌ Explicit Non-Goals (for now)

* policy DSL
* auto-scaling workers
* UI
* global orchestration
* complex scheduling

---

## 🧠 Design Principles

1. **Separation of concerns**

   * metadata = facts
   * reconciler = decisions
   * worker = execution

2. **Reconciliation over control**

   * detect → fix

3. **Idempotent operations**

   * safe retries

4. **Keep it minimal**

   * avoid premature abstraction

---

## 🧩 Summary

Synapse is:

> A lightweight, reconciliation-driven system that ensures files are in the right place by generating and executing movement jobs.

It works by:

* observing metadata
* producing jobs
* executing them reliably

---

## 🔥 Final Note

The hardest part is NOT:

* queue
* workers
* orchestration

The hardest part is:

> **correct, reliable, idempotent file movement**

Focus your effort there.

