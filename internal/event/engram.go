package event

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// EngramEmitter publishes move events to an Engram-compatible RabbitMQ exchange.
// It owns its own AMQP connection, separate from the Synapse job queue.
type EngramEmitter struct {
	conn       *amqp.Connection
	ch         *amqp.Channel
	exchange   string
	routingKey string
}

func NewEngramEmitter(amqpURL, exchange, routingKey string) (*EngramEmitter, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("dial engram amqp: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open engram channel: %w", err)
	}
	return &EngramEmitter{
		conn:       conn,
		ch:         ch,
		exchange:   exchange,
		routingKey: routingKey,
	}, nil
}

func (e *EngramEmitter) EmitMoveCompleted(ctx context.Context, evt MoveCompleted) error {
	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return e.ch.PublishWithContext(ctx, e.exchange, e.routingKey, false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
}

func (e *EngramEmitter) Close() error {
	if err := e.ch.Close(); err != nil {
		e.conn.Close()
		return err
	}
	return e.conn.Close()
}
