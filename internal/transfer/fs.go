package transfer

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// FSMover moves files between subdirectories under a root path.
// Each "bucket" (src/dst) maps to a subdirectory.
type FSMover struct {
	root string
	log  *slog.Logger
}

func NewFSMover(root string, log *slog.Logger) *FSMover {
	return &FSMover{root: root, log: log}
}

func (m *FSMover) MoveFile(_ context.Context, fileID, src, dst string) error {
	srcPath := filepath.Join(m.root, src, fileID)
	dstPath := filepath.Join(m.root, dst, fileID)
	log := m.log.With("file", fileID, "from", src, "to", dst)
	log.Info("starting transfer")

	// Idempotency: if destination exists, verify and clean up source.
	if _, err := os.Stat(dstPath); err == nil {
		log.Info("file already at destination, verifying checksum")
		if err := verifyFiles(srcPath, dstPath); err != nil {
			return fmt.Errorf("verify existing: %w", err)
		}
		if _, srcErr := os.Stat(srcPath); srcErr == nil {
			if err := os.Remove(srcPath); err != nil {
				return fmt.Errorf("remove source %s: %w", srcPath, err)
			}
			log.Info("removed source after verification")
		}
		log.Info("checksum verified, skipping transfer")
		return nil
	}

	// Ensure destination directory exists.
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	// Read source, compute checksum, write to temp file.
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source %s: %w", srcPath, err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	tmpPath := dstPath + ".tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	srcChecksum, err := copyWithHash(srcFile, tmpFile)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("copy to temp: %w", err)
	}

	// Verify the written file.
	verifyFile, err := os.Open(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("open temp for verify: %w", err)
	}
	dstChecksum, err := SHA256Hash(verifyFile)
	verifyFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("checksum temp: %w", err)
	}

	if srcChecksum != dstChecksum {
		os.Remove(tmpPath)
		return fmt.Errorf("checksum mismatch: src=%s dst=%s", srcChecksum, dstChecksum)
	}

	// Atomic move temp to final destination.
	if err := os.Rename(tmpPath, dstPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp to dest: %w", err)
	}

	// Remove source.
	if err := os.Remove(srcPath); err != nil {
		return fmt.Errorf("remove source %s: %w", srcPath, err)
	}

	log.Info("transfer complete", "checksum", srcChecksum, "size", srcInfo.Size())
	return nil
}

// copyWithHash copies from r to w, returning the SHA-256 hash of the data.
func copyWithHash(r io.Reader, w io.Writer) (string, error) {
	return SHA256Hash(io.TeeReader(r, w))
}

// verifyFiles checks that two files on disk have matching SHA-256 checksums.
func verifyFiles(pathA, pathB string) error {
	a, err := os.Open(pathA)
	if err != nil {
		return fmt.Errorf("open %s: %w", pathA, err)
	}
	defer a.Close()

	b, err := os.Open(pathB)
	if err != nil {
		return fmt.Errorf("open %s: %w", pathB, err)
	}
	defer b.Close()

	hashA, err := SHA256Hash(a)
	if err != nil {
		return fmt.Errorf("hash %s: %w", pathA, err)
	}
	hashB, err := SHA256Hash(b)
	if err != nil {
		return fmt.Errorf("hash %s: %w", pathB, err)
	}

	if hashA != hashB {
		return fmt.Errorf("checksum mismatch: %s=%s %s=%s", pathA, hashA, pathB, hashB)
	}
	return nil
}

// FSScanner lists and reads files from subdirectories under a root path.
type FSScanner struct {
	root string
	log  *slog.Logger
}

func NewFSScanner(root string, log *slog.Logger) *FSScanner {
	return &FSScanner{root: root, log: log}
}

func (s *FSScanner) ListObjects(_ context.Context, location string) ([]ObjectInfo, error) {
	dir := filepath.Join(s.root, location)
	var objects []ObjectInfo

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		objects = append(objects, ObjectInfo{
			Key:          rel,
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
		return nil
	})
	return objects, err
}

func (s *FSScanner) GetObject(_ context.Context, location, key string) (io.ReadCloser, error) {
	path := filepath.Join(s.root, location, key)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return f, nil
}
