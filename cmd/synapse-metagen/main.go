package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chunhou/synapse/internal/config"
	"github.com/chunhou/synapse/internal/metadata"
	"github.com/chunhou/synapse/internal/transfer"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()

	source := flag.String("source", "s3", "source type: dir or s3")
	path := flag.String("path", ".", "directory path (when source=dir)")
	bucket := flag.String("bucket", cfg.HotBucket, "S3 bucket name")
	tags := flag.String("tags", "", "comma-separated tags to apply")
	out := flag.String("out", cfg.MetadataFile, "output file path")
	merge := flag.Bool("merge", false, "merge with existing file instead of overwriting")
	flag.Parse()

	var parsedTags []string
	if *tags != "" {
		for _, t := range strings.Split(*tags, ",") {
			if t = strings.TrimSpace(t); t != "" {
				parsedTags = append(parsedTags, t)
			}
		}
	}

	ctx := context.Background()
	var files []metadata.File
	var err error

	switch *source {
	case "s3":
		files, err = scanS3(ctx, cfg, *bucket, parsedTags, log)
	case "dir":
		files, err = scanDir(*path, *bucket, parsedTags, log)
	default:
		fmt.Fprintf(os.Stderr, "unknown source: %s (use dir or s3)\n", *source)
		os.Exit(1)
	}
	if err != nil {
		log.Error("scan failed", "error", err)
		os.Exit(1)
	}

	if *merge {
		files, err = mergeFiles(*out, files)
		if err != nil {
			log.Error("merge failed", "error", err)
			os.Exit(1)
		}
	}

	if err := writeOutput(*out, files); err != nil {
		log.Error("write failed", "error", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "%d files written to %s\n", len(files), *out)
}

func scanS3(ctx context.Context, cfg config.Config, bucket string, tags []string, log *slog.Logger) ([]metadata.File, error) {
	s3, err := transfer.NewS3Client(transfer.S3Config{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		UseSSL:    cfg.S3UseSSL,
	}, log)
	if err != nil {
		return nil, fmt.Errorf("create S3 client: %w", err)
	}

	objects, err := s3.ListObjects(ctx, bucket)
	if err != nil {
		return nil, err
	}

	var files []metadata.File
	for _, obj := range objects {
		reader, err := s3.GetObject(ctx, bucket, obj.Key)
		if err != nil {
			return nil, err
		}
		checksum, err := transfer.SHA256Hash(reader)
		reader.Close()
		if err != nil {
			return nil, fmt.Errorf("checksum %s: %w", obj.Key, err)
		}

		files = append(files, metadata.File{
			ID:           obj.Key,
			Locations:    []string{bucket},
			Tags:         tags,
			Checksum:     checksum,
			Size:         obj.Size,
			LastAccessed: obj.LastModified,
		})
		log.Info("scanned", "file", obj.Key, "size", obj.Size)
	}
	return files, nil
}

func scanDir(dir, bucket string, tags []string, log *slog.Logger) ([]metadata.File, error) {
	var files []metadata.File

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil {
			return err
		}

		checksum, err := transfer.SHA256Hash(f)
		if err != nil {
			return fmt.Errorf("checksum %s: %w", rel, err)
		}

		files = append(files, metadata.File{
			ID:           rel,
			Locations:    []string{bucket},
			Tags:         tags,
			Checksum:     checksum,
			Size:         info.Size(),
			LastAccessed: time.Now(),
		})
		log.Info("scanned", "file", rel, "size", info.Size())
		return nil
	})
	return files, err
}

func mergeFiles(path string, scanned []metadata.File) ([]metadata.File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return scanned, nil
		}
		return nil, err
	}

	var existing []metadata.File
	if err := json.Unmarshal(data, &existing); err != nil {
		return nil, fmt.Errorf("parse existing: %w", err)
	}

	index := make(map[string]*metadata.File, len(existing))
	for i := range existing {
		index[existing[i].ID] = &existing[i]
	}

	for _, s := range scanned {
		if e, ok := index[s.ID]; ok {
			// Update checksum/size if changed, keep locations.
			e.Checksum = s.Checksum
			e.Size = s.Size
			e.Tags = s.Tags
			e.LastAccessed = s.LastAccessed
		} else {
			existing = append(existing, s)
		}
	}
	return existing, nil
}

func writeOutput(path string, files []metadata.File) error {
	data, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
