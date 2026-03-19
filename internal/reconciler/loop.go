package reconciler

import (
	"context"
	"log/slog"
	"time"

	"github.com/chunhou/synapse/internal/job"
	"github.com/chunhou/synapse/internal/metadata"
)

type Reconciler struct {
	meta     *metadata.Client
	queue    *job.Queue
	rules    []Rule
	interval time.Duration
	log      *slog.Logger
}

func New(meta *metadata.Client, queue *job.Queue, rules []Rule, interval time.Duration, log *slog.Logger) *Reconciler {
	return &Reconciler{
		meta:     meta,
		queue:    queue,
		rules:    rules,
		interval: interval,
		log:      log,
	}
}

// Run starts the reconciliation loop, running until the context is cancelled.
func (r *Reconciler) Run(ctx context.Context) error {
	r.log.Info("reconciler starting", "interval", r.interval, "rules", len(r.rules))

	// Run immediately on start, then on ticker.
	r.reconcile(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.log.Info("reconciler shutting down")
			return ctx.Err()
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context) {
	r.log.Info("reconciliation cycle starting")

	files, err := r.meta.ListFiles(ctx)
	if err != nil {
		r.log.Error("failed to list files from metadata", "error", err)
		return
	}

	r.log.Info("fetched file metadata", "count", len(files))

	var enqueued int
	for _, f := range files {
		for _, rule := range r.rules {
			if rule.Match(f) {
				j := rule.Action(f)
				if err := r.queue.Publish(ctx, j); err != nil {
					r.log.Error("failed to enqueue job",
						"rule", rule.Name,
						"file", f.ID,
						"error", err)
					continue
				}
				r.log.Info("enqueued job",
					"rule", rule.Name,
					"job_id", j.ID,
					"file", f.ID,
					"from", j.Payload.From,
					"to", j.Payload.To)
				enqueued++
			}
		}
	}

	r.log.Info("reconciliation cycle complete", "jobs_enqueued", enqueued)
}
