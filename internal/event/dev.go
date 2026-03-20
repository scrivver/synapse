package event

import (
	"context"

	"github.com/chunhou/synapse/internal/metadata"
)

// DevEmitter wraps LogFileEmitter and also updates the JSONFileProvider
// to close the feedback loop in dev mode. This ensures the reconciler
// sees updated locations and does not re-enqueue completed jobs.
type DevEmitter struct {
	log  *LogFileEmitter
	meta *metadata.JSONFileProvider
}

func NewDevEmitter(logPath string, meta *metadata.JSONFileProvider) *DevEmitter {
	return &DevEmitter{
		log:  NewLogFileEmitter(logPath),
		meta: meta,
	}
}

func (e *DevEmitter) EmitMoveCompleted(ctx context.Context, evt MoveCompleted) error {
	if err := e.log.EmitMoveCompleted(ctx, evt); err != nil {
		return err
	}
	if err := e.meta.RemoveLocation(ctx, evt.FileID, evt.From); err != nil {
		return err
	}
	return e.meta.AddLocation(ctx, evt.FileID, evt.To)
}

func (e *DevEmitter) Close() error {
	return nil
}
