package event

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// LogFileEmitter appends JSON-lines to a file on disk.
type LogFileEmitter struct {
	path string
	mu   sync.Mutex
}

func NewLogFileEmitter(path string) *LogFileEmitter {
	return &LogFileEmitter{path: path}
}

func (e *LogFileEmitter) EmitMoveCompleted(_ context.Context, evt MoveCompleted) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	f, err := os.OpenFile(e.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open event log: %w", err)
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(evt)
}

func (e *LogFileEmitter) Close() error {
	return nil
}
