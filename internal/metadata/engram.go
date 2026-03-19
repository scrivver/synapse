package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// EngramClient is an adapter that implements Provider by talking to the
// Engram metadata HTTP API.
type EngramClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewEngramClient(baseURL string) *EngramClient {
	return &EngramClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *EngramClient) ListFiles(ctx context.Context) ([]File, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/files", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list files: status %d", resp.StatusCode)
	}

	var files []File
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("decode files: %w", err)
	}
	return files, nil
}

func (c *EngramClient) GetFile(ctx context.Context, id string) (*File, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/files/"+id, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get file %s: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get file %s: status %d", id, resp.StatusCode)
	}

	var f File
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		return nil, fmt.Errorf("decode file: %w", err)
	}
	return &f, nil
}

func (c *EngramClient) AddLocation(ctx context.Context, fileID, location string) error {
	body := fmt.Sprintf(`{"location":%q}`, location)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/files/"+fileID+"/locations",
		strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("add location to %s: %w", fileID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("add location to %s: status %d", fileID, resp.StatusCode)
	}
	return nil
}
