package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"wd-tube/internal/converter"
	"wd-tube/internal/rabbitmq"

	_ "github.com/lib/pq"
	"github.com/streadway/amqp"
)

func connectPostgres() (*sql.DB, error) {
	user := getEnvOrDefault("POSTGRES_USER", "postgres")
	password := getEnvOrDefault("POSTGRES_PASSWORD", "postgres")
	dbname := getEnvOrDefault("POSTGRES_DB", "converter")
	host := getEnvOrDefault("POSTGRES_HOST", "postgres")
	sslmode := getEnvOrDefault("POSTGRES_SSLMODE", "disable")

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s host=%s sslmode=%s", user, password, dbname, host, sslmode)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		slog.Error("Error connecting to database", slog.String("connStr", connStr))
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		slog.Error("Error pinging database", slog.String("connStr", connStr))
		return nil, err
	}
	slog.Info("Connected to database")
	return db, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func connectRabbitMQ() (*rabbitmq.RabbitClient, error) {
	url := getEnvOrDefault("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/")
	client, err := rabbitmq.NewRabbitClient(url)
	if err != nil {
		slog.Error("Error connecting to RabbitMQ", slog.String("url", url))
		return nil, err
	}
	slog.Info("Connected to RabbitMQ")
	return client, nil
}

func main() {
	println("Starting video converter")
	db, err := connectPostgres()
	if err != nil {
		panic(err)
	}
	rabbitmqClient, err := connectRabbitMQ()
	if err != nil {
		panic(err)
	}
	defer rabbitmqClient.Close()

	convertionExchange := getEnvOrDefault("CONVERTION_EXCHANGE", "conversion_exchange")
	queueName := getEnvOrDefault("CONVERTION_QUEUE", "video_conversion_queue")
	convertionKey := getEnvOrDefault("CONVERTION_KEY", "conversion")

	confirmationKey := getEnvOrDefault("CONFIRMATION_KEY", "finish-conversion")
	confirmationQueue := getEnvOrDefault("CONFIRMATION_QUEUE", "video_confirmation_queue")

	vc := converter.NewVideoConverter(db, *rabbitmqClient)
	msgs, err := rabbitmqClient.ConsumeMessages(convertionExchange, convertionKey, queueName)
	if err != nil {
		slog.Error("Error consuming messages", slog.String("error", err.Error()))
	}

	for d := range msgs {
		go func(delivery amqp.Delivery) {
			vc.Handle(delivery, convertionExchange, confirmationQueue, confirmationKey)
		}(d)
	}
	// vc.Handle([]byte(`{"video_id": 2, "path": "mediatest/media/uploads/2"}`))
}
