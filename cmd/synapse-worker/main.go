package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/chunhou/synapse/internal/config"
	"github.com/chunhou/synapse/internal/event"
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

	var mover transfer.Mover
	switch cfg.StorageBackend {
	case "fs":
		mover = transfer.NewFSMover(cfg.StorageFSRoot, log)
		log.Info("storage backend: filesystem", "root", cfg.StorageFSRoot)
	case "s3":
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
		mover = s3
		log.Info("storage backend: S3", "endpoint", cfg.S3Endpoint)
	default:
		log.Error("unknown storage backend", "backend", cfg.StorageBackend)
		os.Exit(1)
	}

	var emitter event.Emitter
	if cfg.EngramAMQPURL != "" {
		em, err := event.NewEngramEmitter(cfg.EngramAMQPURL, cfg.EngramExchange, cfg.EngramRoutingKey)
		if err != nil {
			log.Error("failed to create engram emitter", "error", err)
			os.Exit(1)
		}
		defer em.Close()
		emitter = em
		log.Info("event emitter: engram", "exchange", cfg.EngramExchange, "routing_key", cfg.EngramRoutingKey)
	} else {
		meta := metadata.NewJSONFileProvider(cfg.MetadataFile)
		emitter = event.NewDevEmitter(cfg.EventLogFile, meta)
		log.Info("event emitter: dev", "log", cfg.EventLogFile, "metadata", cfg.MetadataFile)
	}

	executor := worker.NewExecutor(queue, mover, emitter, cfg.MaxRetries, log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := executor.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("worker exited with error", "error", err)
		os.Exit(1)
	}

	log.Info("worker stopped")
}
