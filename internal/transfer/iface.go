package transfer

import (
	"context"
	"io"
	"time"
)

// Mover moves files between named locations (buckets or directories).
type Mover interface {
	MoveFile(ctx context.Context, fileID, src, dst string) error
}

// ObjectInfo is a backend-agnostic description of a stored object.
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// Scanner lists and reads objects from a named location.
type Scanner interface {
	ListObjects(ctx context.Context, location string) ([]ObjectInfo, error)
	GetObject(ctx context.Context, location, key string) (io.ReadCloser, error)
}
