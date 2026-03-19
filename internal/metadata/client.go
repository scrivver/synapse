package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

// Client talks to the Engram metadata API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) ListFiles(ctx context.Context) ([]File, error) {
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

func (c *Client) GetFile(ctx context.Context, id string) (*File, error) {
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

func (c *Client) AddLocation(ctx context.Context, fileID, location string) error {
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
