package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// JSONFileProvider implements Provider by reading/writing a local JSON file.
// It re-reads the file on every call so external edits (e.g. from metagen)
// are picked up without restarting the process.
type JSONFileProvider struct {
	path string
	mu   sync.RWMutex
}

func NewJSONFileProvider(path string) *JSONFileProvider {
	return &JSONFileProvider{path: path}
}

func (p *JSONFileProvider) ListFiles(_ context.Context) ([]File, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.readAll()
}

func (p *JSONFileProvider) GetFile(_ context.Context, id string) (*File, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	files, err := p.readAll()
	if err != nil {
		return nil, err
	}
	for i := range files {
		if files[i].ID == id {
			return &files[i], nil
		}
	}
	return nil, fmt.Errorf("file %s not found", id)
}

func (p *JSONFileProvider) AddLocation(_ context.Context, fileID, location string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	files, err := p.readAll()
	if err != nil {
		return err
	}

	found := false
	for i := range files {
		if files[i].ID == fileID {
			if !slices.Contains(files[i].Locations, location) {
				files[i].Locations = append(files[i].Locations, location)
			}
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("file %s not found", fileID)
	}

	return p.writeAll(files)
}

func (p *JSONFileProvider) readAll() ([]File, error) {
	data, err := os.ReadFile(p.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []File{}, nil
		}
		return nil, fmt.Errorf("read metadata file: %w", err)
	}
	if len(data) == 0 {
		return []File{}, nil
	}
	var files []File
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("parse metadata file: %w", err)
	}
	return files, nil
}

func (p *JSONFileProvider) writeAll(files []File) error {
	data, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	dir := filepath.Dir(p.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create metadata dir: %w", err)
	}

	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, p.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
