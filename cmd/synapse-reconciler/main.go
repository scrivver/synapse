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
	"github.com/chunhou/synapse/internal/reconciler"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()

	if cfg.EngramAPIURL == "" {
		log.Error("ENGRAM_API_URL is required for the reconciler")
		os.Exit(1)
	}

	queue, err := job.NewQueue(cfg.RabbitMQURL)
	if err != nil {
		log.Error("failed to connect to RabbitMQ", "error", err)
		os.Exit(1)
	}
	defer queue.Close()
	log.Info("connected to RabbitMQ", "url", cfg.RabbitMQURL)

	meta := metadata.NewClient(cfg.EngramAPIURL)
	log.Info("metadata client configured", "url", cfg.EngramAPIURL)

	rules := reconciler.DefaultRules(cfg.HotBucket, cfg.ColdBucket)
	r := reconciler.New(meta, queue, rules, cfg.ReconcileInterval, log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := r.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("reconciler exited with error", "error", err)
		os.Exit(1)
	}

	log.Info("reconciler stopped")
}
