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
