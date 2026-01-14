package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Configuration from environment variables
	kafkaBrokers := getEnv("KAFKA_BROKERS", "localhost:9092")
	kafkaTopic := getEnv("KAFKA_TOPIC", "incoming.user_activity")
	kafkaGroupID := getEnv("KAFKA_GROUP_ID", "user-activity-consumer-group")
	numWorkers := getEnvAsInt("NUM_WORKERS", 3)
	flushIntervalSec := getEnvAsInt("FLUSH_INTERVAL_SEC", 30)
	batchSize := getEnvAsInt("BATCH_SIZE", 100)

	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "analytics")

	// Initialize database
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := initDB(connStr)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Create aggregator
	aggregator := NewAggregator(db, time.Duration(flushIntervalSec)*time.Second, batchSize)

	// Create consumer
	consumer := NewConsumer(
		[]string{kafkaBrokers},
		kafkaTopic,
		kafkaGroupID,
		aggregator,
		numWorkers,
	)
	defer consumer.Close()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start consumer in goroutine
	go func() {
		log.Printf("Starting consumer with %d workers", numWorkers)
		if err := consumer.Start(ctx); err != nil {
			log.Printf("Consumer error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down consumer...")
	cancel()

	// Give some time for graceful shutdown
	time.Sleep(5 * time.Second)

	log.Println("Consumer exited")
}
