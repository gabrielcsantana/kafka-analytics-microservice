package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

// Consumer handles consuming user activity events from Kafka
type Consumer struct {
	reader     *kafka.Reader
	aggregator *Aggregator
	numWorkers int
}

// NewConsumer creates a new Consumer instance
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

// Start begins consuming messages with multiple worker goroutines
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

// worker is a goroutine that processes messages from Kafka
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

// periodicFlush flushes aggregated data at regular intervals
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

// Close closes the Kafka reader connection
func (c *Consumer) Close() error {
	return c.reader.Close()
}
