package metadata

import (
	"context"
	"time"
)

type File struct {
	ID           string    `json:"file_id"`
	Locations    []string  `json:"locations"`
	Tags         []string  `json:"tags"`
	Checksum     string    `json:"checksum"`
	Size         int64     `json:"size"`
	LastAccessed time.Time `json:"last_accessed"`
}

// Provider is the port through which Synapse reads and updates file metadata.
// Concrete adapters (e.g. EngramClient) implement this interface.
type Provider interface {
	ListFiles(ctx context.Context) ([]File, error)
	GetFile(ctx context.Context, id string) (*File, error)
	AddLocation(ctx context.Context, fileID, location string) error
}
