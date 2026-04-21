package broker

import (
	"context"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const rabbitQueue = "bench"

type RabbitMQ struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

func NewRabbitMQ(url string) (*RabbitMQ, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, err
	}
	_, err = ch.QueueDeclare(rabbitQueue, true, false, false, false, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &RabbitMQ{conn: conn, ch: ch}, nil
}

func (r *RabbitMQ) Publish(_ context.Context, payload []byte) error {
	return r.ch.Publish(
		"",
		rabbitQueue,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/octet-stream",
			DeliveryMode: amqp.Transient,
			Body:         payload,
		},
	)
}

func (r *RabbitMQ) Subscribe(ctx context.Context, handler func([]byte, time.Time)) error {
	msgs, err := r.ch.Consume(
		rabbitQueue,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			handler(msg.Body, msg.Timestamp)
		}
	}
}

func (r *RabbitMQ) Close() error {
	r.ch.Close()
	return r.conn.Close()
}
