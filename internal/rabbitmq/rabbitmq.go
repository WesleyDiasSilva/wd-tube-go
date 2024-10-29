package rabbitmq

import (
	"fmt"
	"log/slog"

	"github.com/streadway/amqp"
)

type RabbitClient struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	url     string
}

func newConnection(url string) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	return conn, ch, nil
}

func NewRabbitClient(url string) (*RabbitClient, error) {
	fmt.Println("Connecting to RabbitMQ")
	conn, channel, err := newConnection(url)
	fmt.Println("Connected to RabbitMQ")
	if err != nil {
		return nil, err
	}

	return &RabbitClient{conn: conn, channel: channel, url: url}, nil
}

func (r *RabbitClient) DeclareAndBindExchangeWithQueue(exchange, routingKey, queueName string) error {
	err := r.channel.ExchangeDeclare(exchange, "direct", true, true, false, false, nil)

	if err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	queue, err := r.channel.QueueDeclare(queueName, true, true, false, false, nil)

	if err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	err = r.channel.QueueBind(queue.Name, routingKey, exchange, false, nil)

	if err != nil {
		return fmt.Errorf("failed to bind exchange: %w", err)
	}
	return nil
}

func (r *RabbitClient) ConsumeMessages(exchange, routingKey, queueName string) (<-chan amqp.Delivery, error) {

	err := r.DeclareAndBindExchangeWithQueue(exchange, routingKey, queueName)

	if err != nil {
		return nil, fmt.Errorf("failed to declare and bind exchange with queue: %w", err)
	}

	msgs, err := r.channel.Consume(
		queueName,
		"goapp",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to consume messages: %w", err)
	}
	return msgs, nil
}

func (r *RabbitClient) PublishMessage(exchange, routingKey, queueName string, message []byte) error {

	err := r.DeclareAndBindExchangeWithQueue(exchange, routingKey, queueName)

	if err != nil {
		return fmt.Errorf("failed to declare and bind exchange with queue: %w", err)
	}

	err = r.channel.Publish(
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        message,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}
	slog.Info("\033[32m"+"Message published"+"\033[0m", slog.String("exchange", exchange), slog.String("routingKey", routingKey), slog.String("queueName", queueName))
	return nil
}

func (r *RabbitClient) Close() {
	r.channel.Close()
	r.conn.Close()
}
