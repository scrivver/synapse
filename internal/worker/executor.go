package worker

import (
	"context"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/chunhou/synapse/internal/event"
	"github.com/chunhou/synapse/internal/job"
	"github.com/chunhou/synapse/internal/transfer"
)

type Executor struct {
	queue      *job.Queue
	s3         *transfer.S3Client
	emitter    event.Emitter
	maxRetries int
	log        *slog.Logger
}

func NewExecutor(queue *job.Queue, s3 *transfer.S3Client, emitter event.Emitter, maxRetries int, log *slog.Logger) *Executor {
	return &Executor{
		queue:      queue,
		s3:         s3,
		emitter:    emitter,
		maxRetries: maxRetries,
		log:        log,
	}
}

// Run consumes jobs from the queue and executes them until the context is cancelled.
func (e *Executor) Run(ctx context.Context) error {
	deliveries, err := e.queue.Consume(ctx)
	if err != nil {
		return err
	}

	e.log.Info("worker listening for jobs")

	for {
		select {
		case <-ctx.Done():
			e.log.Info("worker shutting down")
			return ctx.Err()
		case d, ok := <-deliveries:
			if !ok {
				e.log.Info("delivery channel closed")
				return nil
			}
			e.handle(ctx, d)
		}
	}
}

func (e *Executor) handle(ctx context.Context, d amqp.Delivery) {
	j, err := job.Unmarshal(d.Body)
	if err != nil {
		e.log.Error("unmarshal job failed, dropping message", "error", err)
		_ = d.Nack(false, false)
		return
	}

	log := e.log.With("job_id", j.ID, "type", j.Type, "retry", j.Retry)
	log.Info("processing job")

	switch j.Type {
	case job.TypeMoveFile:
		err = e.s3.MoveFile(ctx, j.Payload.FileID, j.Payload.From, j.Payload.To)
	default:
		log.Error("unknown job type, dropping")
		_ = d.Nack(false, false)
		return
	}

	if err != nil {
		log.Error("job failed", "error", err)
		j.Retry++
		if j.Retry > e.maxRetries {
			log.Warn("max retries exceeded, sending to DLQ")
			if dlqErr := e.queue.PublishDLQ(ctx, j); dlqErr != nil {
				log.Error("failed to publish to DLQ", "error", dlqErr)
			}
		} else {
			log.Info("requeueing with incremented retry")
			if pubErr := e.queue.Publish(ctx, j); pubErr != nil {
				log.Error("failed to requeue job", "error", pubErr)
			}
		}
		_ = d.Ack(false)
		return
	}

	// Emit event after successful transfer.
	if e.emitter != nil {
		evt := event.MoveCompleted{
			JobID:     j.ID,
			FileID:    j.Payload.FileID,
			From:      j.Payload.From,
			To:        j.Payload.To,
			Timestamp: time.Now(),
		}
		if emitErr := e.emitter.EmitMoveCompleted(ctx, evt); emitErr != nil {
			log.Warn("failed to emit event (non-fatal)", "error", emitErr)
		}
	}

	log.Info("job completed successfully")
	_ = d.Ack(false)
}
