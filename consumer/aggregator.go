package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"
)

// Aggregator handles aggregation of user activity data
type Aggregator struct {
	mu            sync.Mutex
	uniqueUsers   map[string]bool
	pageViewCount int64
	db            *sql.DB
	flushInterval time.Duration
	batchSize     int
	messageCount  int
}

// NewAggregator creates a new Aggregator instance
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

// ProcessActivity processes a single user activity event
func (a *Aggregator) ProcessActivity(activity UserActivity) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.uniqueUsers[activity.UserID] = true

	if activity.ActivityType == "page_view" {
		a.pageViewCount++
	}

	a.messageCount++
}

// Flush writes aggregated metrics to the database
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

// insertMetrics inserts aggregated metrics into the database
func (a *Aggregator) insertMetrics(ctx context.Context, uniqueUsers int, pageViews int64) error {
	query := `
		INSERT INTO user_activity_metrics (timestamp, unique_users, page_view_count)
		VALUES ($1, $2, $3)
	`

	_, err := a.db.ExecContext(ctx, query, time.Now(), uniqueUsers, pageViews)
	return err
}

// ShouldFlush returns true if the aggregator should flush based on batch size
func (a *Aggregator) ShouldFlush() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.messageCount >= a.batchSize
}
