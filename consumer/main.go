package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/segmentio/kafka-go"
	_ "github.com/lib/pq"
)

type UserActivity struct {
	UserID       string   `json:"user_id"`
	ActivityType string   `json:"activity_type"`
	Timestamp    string   `json:"timestamp"`
	Metadata     Metadata `json:"metadata"`
}

type Metadata struct {
	PageURL  string `json:"page_url"`
	Referrer string `json:"referrer"`
}

type Aggregator struct {
	mu            sync.Mutex
	uniqueUsers   map[string]bool
	pageViewCount int64
	db            *sql.DB
	flushInterval time.Duration
	batchSize     int
	messageCount  int
}

func NewAggregator(db *sql.DB, flushInterval time.Duration, batchSize int) *Aggregator {
	return &Aggregator{
		uniqueUsers:   make(map[string]bool),
		pageViewCount: 0,
		db:            db,
		flushInterval: flushInterval,
		batchSize:     batchSize,
		messageCount:  0,
	}
}

func (a *Aggregator) ProcessActivity(activity UserActivity) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.uniqueUsers[activity.UserID] = true

	if activity.ActivityType == "page_view" {
		a.pageViewCount++
	}

	a.messageCount++
}

func (a *Aggregator) Flush(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.uniqueUsers) == 0 && a.pageViewCount == 0 {
		return nil
	}

	uniqueUserCount := len(a.uniqueUsers)
	pageViews := a.pageViewCount

	log.Printf("Flushing metrics: unique_users=%d, page_views=%d", uniqueUserCount, pageViews)

	// Retry logic for database insert
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		err = a.insertMetrics(ctx, uniqueUserCount, pageViews)
		if err == nil {
			break
		}
		log.Printf("Failed to insert metrics (attempt %d/3): %v", attempt, err)
		time.Sleep(time.Duration(attempt) * time.Second)
	}

	if err != nil {
		return fmt.Errorf("failed to insert metrics after 3 attempts: %w", err)
	}

	// Reset aggregator
	a.uniqueUsers = make(map[string]bool)
	a.pageViewCount = 0
	a.messageCount = 0

	return nil
}

func (a *Aggregator) insertMetrics(ctx context.Context, uniqueUsers int, pageViews int64) error {
	query := `
		INSERT INTO user_activity_metrics (timestamp, unique_users, page_view_count)
		VALUES ($1, $2, $3)
	`

	_, err := a.db.ExecContext(ctx, query, time.Now(), uniqueUsers, pageViews)
	return err
}

func (a *Aggregator) ShouldFlush() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.messageCount >= a.batchSize
}

type Consumer struct {
	reader     *kafka.Reader
	aggregator *Aggregator
	numWorkers int
}

func NewConsumer(brokers []string, topic string, groupID string, aggregator *Aggregator, numWorkers int) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:         brokers,
		Topic:           topic,
		GroupID:         groupID,
		MinBytes:        10e3, // 10KB
		MaxBytes:        10e6, // 10MB
		MaxWait:         1 * time.Second,
		ReadLagInterval: -1,
		StartOffset:     kafka.FirstOffset,
	})

	return &Consumer{
		reader:     reader,
		aggregator: aggregator,
		numWorkers: numWorkers,
	}
}

func (c *Consumer) Start(ctx context.Context) error {
	var wg sync.WaitGroup

	// Start worker goroutines for horizontal scaling
	for i := 0; i < c.numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			c.worker(ctx, workerID)
		}(i)
	}

	// Start periodic flush
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.periodicFlush(ctx)
	}()

	wg.Wait()
	return nil
}

func (c *Consumer) worker(ctx context.Context, workerID int) {
	log.Printf("Worker %d started", workerID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d shutting down", workerID)
			return
		default:
			// Read message with timeout
			readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			m, err := c.reader.FetchMessage(readCtx)
			cancel()

			if err != nil {
				if err == context.Canceled || err == context.DeadlineExceeded {
					continue
				}
				log.Printf("Worker %d: Error fetching message: %v", workerID, err)
				time.Sleep(1 * time.Second)
				continue
			}

			// Process message
			var activity UserActivity
			if err := json.Unmarshal(m.Value, &activity); err != nil {
				log.Printf("Worker %d: Error unmarshaling message: %v", workerID, err)
				// Commit even if parsing fails to avoid reprocessing
				c.reader.CommitMessages(ctx, m)
				continue
			}

			log.Printf("Worker %d: Processing activity for user %s, type %s", workerID, activity.UserID, activity.ActivityType)

			// Aggregate data
			c.aggregator.ProcessActivity(activity)

			// Commit message
			if err := c.reader.CommitMessages(ctx, m); err != nil {
				log.Printf("Worker %d: Error committing message: %v", workerID, err)
			}

			// Flush if batch size reached
			if c.aggregator.ShouldFlush() {
				if err := c.aggregator.Flush(ctx); err != nil {
					log.Printf("Worker %d: Error flushing aggregator: %v", workerID, err)
				}
			}
		}
	}
}

func (c *Consumer) periodicFlush(ctx context.Context) {
	ticker := time.NewTicker(c.aggregator.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Periodic flush shutting down")
			// Final flush before exit
			if err := c.aggregator.Flush(context.Background()); err != nil {
				log.Printf("Error during final flush: %v", err)
			}
			return
		case <-ticker.C:
			if err := c.aggregator.Flush(ctx); err != nil {
				log.Printf("Error during periodic flush: %v", err)
			}
		}
	}
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}

func initDB(connStr string) (*sql.DB, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	// Retry connection with backoff
	for i := 0; i < 10; i++ {
		err = db.Ping()
		if err == nil {
			break
		}
		log.Printf("Failed to connect to database (attempt %d/10): %v", i+1, err)
		time.Sleep(time.Duration(i+1) * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to database after 10 attempts: %w", err)
	}

	// Create table if not exists
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS user_activity_metrics (
			id SERIAL PRIMARY KEY,
			timestamp TIMESTAMP NOT NULL,
			unique_users INTEGER NOT NULL,
			page_view_count BIGINT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	log.Println("Database initialized successfully")
	return db, nil
}

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

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	var value int
	fmt.Sscanf(valueStr, "%d", &value)
	return value
}
