package transfer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

type S3Client struct {
	client *minio.Client
	log    *slog.Logger
}

func NewS3Client(cfg S3Config, log *slog.Logger) (*S3Client, error) {
	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return &S3Client{client: mc, log: log}, nil
}

// Exists checks if an object exists in a bucket.
func (s *S3Client) Exists(ctx context.Context, bucket, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		resp := minio.ToErrorResponse(err)
		if resp.Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("stat %s/%s: %w", bucket, key, err)
	}
	return true, nil
}

// MoveFile transfers a file between buckets with checksum verification.
// It is idempotent: if the file already exists at the destination with the
// correct checksum, the transfer is skipped.
func (s *S3Client) MoveFile(ctx context.Context, fileID, srcBucket, dstBucket string) error {
	log := s.log.With("file", fileID, "from", srcBucket, "to", dstBucket)
	log.Info("starting transfer")

	// Idempotency: check if already at destination.
	exists, err := s.Exists(ctx, dstBucket, fileID)
	if err != nil {
		return err
	}
	if exists {
		log.Info("file already at destination, verifying checksum")
		if err := s.verify(ctx, fileID, srcBucket, dstBucket); err != nil {
			return fmt.Errorf("verify existing: %w", err)
		}
		log.Info("checksum verified, skipping transfer")
		return nil
	}

	// Download source object metadata for size/content-type.
	srcObj, err := s.client.GetObject(ctx, srcBucket, fileID, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("get %s/%s: %w", srcBucket, fileID, err)
	}
	defer srcObj.Close()

	srcInfo, err := srcObj.Stat()
	if err != nil {
		return fmt.Errorf("stat source object: %w", err)
	}

	// Stream source through a hash and pipe it to the upload.
	pr, pw := io.Pipe()
	h := sha256.New()

	// Writer goroutine: reads source, writes to hash + pipe.
	errCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.MultiWriter(h, pw), srcObj)
		pw.CloseWithError(err)
		errCh <- err
	}()

	// Upload to destination from the pipe reader.
	_, uploadErr := s.client.PutObject(ctx, dstBucket, fileID, pr, srcInfo.Size,
		minio.PutObjectOptions{ContentType: srcInfo.ContentType})

	copyErr := <-errCh
	if copyErr != nil {
		return fmt.Errorf("read source: %w", copyErr)
	}
	if uploadErr != nil {
		return fmt.Errorf("upload to %s/%s: %w", dstBucket, fileID, uploadErr)
	}

	srcChecksum := hex.EncodeToString(h.Sum(nil))

	// Verify the uploaded object's checksum.
	dstObj, err := s.client.GetObject(ctx, dstBucket, fileID, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("get uploaded %s/%s: %w", dstBucket, fileID, err)
	}
	defer dstObj.Close()

	dstChecksum, err := SHA256Hash(dstObj)
	if err != nil {
		return fmt.Errorf("checksum destination: %w", err)
	}

	if srcChecksum != dstChecksum {
		_ = s.client.RemoveObject(ctx, dstBucket, fileID, minio.RemoveObjectOptions{})
		return fmt.Errorf("checksum mismatch: src=%s dst=%s", srcChecksum, dstChecksum)
	}

	log.Info("transfer complete", "checksum", srcChecksum, "size", srcInfo.Size)
	return nil
}

// verify checks that the same file in two buckets has matching checksums.
func (s *S3Client) verify(ctx context.Context, fileID, srcBucket, dstBucket string) error {
	srcObj, err := s.client.GetObject(ctx, srcBucket, fileID, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("get src: %w", err)
	}
	defer srcObj.Close()

	dstObj, err := s.client.GetObject(ctx, dstBucket, fileID, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("get dst: %w", err)
	}
	defer dstObj.Close()

	srcHash, err := SHA256Hash(srcObj)
	if err != nil {
		return fmt.Errorf("hash src: %w", err)
	}

	dstHash, err := SHA256Hash(dstObj)
	if err != nil {
		return fmt.Errorf("hash dst: %w", err)
	}

	if srcHash != dstHash {
		return fmt.Errorf("checksum mismatch: src=%s dst=%s", srcHash, dstHash)
	}
	return nil
}
