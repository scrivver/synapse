package job

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	QueueName    = "synapse.jobs"
	DLQName      = "synapse.jobs.dlq"
	ExchangeName = "amq.direct"
)

type Queue struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

func NewQueue(url string) (*Queue, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}

	if err := ch.Qos(1, 0, false); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("set qos: %w", err)
	}

	return &Queue{conn: conn, ch: ch}, nil
}

func (q *Queue) Publish(ctx context.Context, j Job) error {
	body, err := Marshal(j)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}

	return q.ch.PublishWithContext(ctx,
		ExchangeName,
		QueueName,
		false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
}

func (q *Queue) PublishDLQ(ctx context.Context, j Job) error {
	body, err := Marshal(j)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}

	return q.ch.PublishWithContext(ctx,
		ExchangeName,
		DLQName,
		false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
}

func (q *Queue) Consume(ctx context.Context) (<-chan amqp.Delivery, error) {
	return q.ch.ConsumeWithContext(ctx,
		QueueName,
		"",    // auto-generated consumer tag
		false, // manual ack
		false, // not exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
}

func (q *Queue) Close() error {
	if err := q.ch.Close(); err != nil {
		q.conn.Close()
		return err
	}
	return q.conn.Close()
}
