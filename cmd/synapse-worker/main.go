package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/chunhou/synapse/internal/config"
	"github.com/chunhou/synapse/internal/job"
	"github.com/chunhou/synapse/internal/metadata"
	"github.com/chunhou/synapse/internal/transfer"
	"github.com/chunhou/synapse/internal/worker"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()

	queue, err := job.NewQueue(cfg.RabbitMQURL)
	if err != nil {
		log.Error("failed to connect to RabbitMQ", "error", err)
		os.Exit(1)
	}
	defer queue.Close()
	log.Info("connected to RabbitMQ", "url", cfg.RabbitMQURL)

	s3, err := transfer.NewS3Client(transfer.S3Config{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		UseSSL:    cfg.S3UseSSL,
	}, log)
	if err != nil {
		log.Error("failed to create S3 client", "error", err)
		os.Exit(1)
	}
	log.Info("S3 client ready", "endpoint", cfg.S3Endpoint)

	var meta *metadata.Client
	if cfg.EngramAPIURL != "" {
		meta = metadata.NewClient(cfg.EngramAPIURL)
		log.Info("metadata client configured", "url", cfg.EngramAPIURL)
	} else {
		log.Info("no ENGRAM_API_URL set, metadata updates disabled")
	}

	executor := worker.NewExecutor(queue, s3, meta, cfg.MaxRetries, log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := executor.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", "error", err)
		os.Exit(1)
	}

	log.Info("worker stopped")
}
