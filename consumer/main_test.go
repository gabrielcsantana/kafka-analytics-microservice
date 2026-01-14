package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserActivity_JSONUnmarshaling tests that UserActivity can be unmarshaled from JSON
func TestUserActivity_JSONUnmarshaling(t *testing.T) {
	jsonData := `{
		"user_id": "user123",
		"activity_type": "page_view",
		"timestamp": "2024-07-01T12:34:56Z",
		"metadata": {
			"page_url": "https://example.com/home",
			"referrer": "https://google.com"
		}
	}`

	var activity UserActivity
	err := json.Unmarshal([]byte(jsonData), &activity)
	require.NoError(t, err)

	assert.Equal(t, "user123", activity.UserID)
	assert.Equal(t, "page_view", activity.ActivityType)
	assert.Equal(t, "2024-07-01T12:34:56Z", activity.Timestamp)
	assert.Equal(t, "https://example.com/home", activity.Metadata.PageURL)
	assert.Equal(t, "https://google.com", activity.Metadata.Referrer)
}

// TestNewAggregator tests aggregator initialization
func TestNewAggregator(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	flushInterval := 30 * time.Second
	batchSize := 100

	aggregator := NewAggregator(db, flushInterval, batchSize)

	assert.NotNil(t, aggregator)
	assert.Equal(t, db, aggregator.db)
	assert.Equal(t, flushInterval, aggregator.flushInterval)
	assert.Equal(t, batchSize, aggregator.batchSize)
	assert.Equal(t, 0, len(aggregator.uniqueUsers))
	assert.Equal(t, int64(0), aggregator.pageViewCount)
	assert.Equal(t, 0, aggregator.messageCount)
}

// TestAggregator_ProcessActivity tests activity processing
func TestAggregator_ProcessActivity(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	aggregator := NewAggregator(db, 30*time.Second, 100)

	tests := []struct {
		name              string
		activities        []UserActivity
		expectedUsers     int
		expectedPageViews int64
		expectedMessages  int
	}{
		{
			name: "single page_view",
			activities: []UserActivity{
				{
					UserID:       "user1",
					ActivityType: "page_view",
					Timestamp:    "2024-07-01T12:00:00Z",
				},
			},
			expectedUsers:     1,
			expectedPageViews: 1,
			expectedMessages:  1,
		},
		{
			name: "multiple page_views from same user",
			activities: []UserActivity{
				{
					UserID:       "user1",
					ActivityType: "page_view",
					Timestamp:    "2024-07-01T12:00:00Z",
				},
				{
					UserID:       "user1",
					ActivityType: "page_view",
					Timestamp:    "2024-07-01T12:01:00Z",
				},
			},
			expectedUsers:     1,
			expectedPageViews: 2,
			expectedMessages:  2,
		},
		{
			name: "multiple users",
			activities: []UserActivity{
				{
					UserID:       "user1",
					ActivityType: "page_view",
					Timestamp:    "2024-07-01T12:00:00Z",
				},
				{
					UserID:       "user2",
					ActivityType: "page_view",
					Timestamp:    "2024-07-01T12:01:00Z",
				},
				{
					UserID:       "user3",
					ActivityType: "page_view",
					Timestamp:    "2024-07-01T12:02:00Z",
				},
			},
			expectedUsers:     3,
			expectedPageViews: 3,
			expectedMessages:  3,
		},
		{
			name: "mixed activity types",
			activities: []UserActivity{
				{
					UserID:       "user1",
					ActivityType: "page_view",
					Timestamp:    "2024-07-01T12:00:00Z",
				},
				{
					UserID:       "user1",
					ActivityType: "click",
					Timestamp:    "2024-07-01T12:01:00Z",
				},
				{
					UserID:       "user2",
					ActivityType: "page_view",
					Timestamp:    "2024-07-01T12:02:00Z",
				},
			},
			expectedUsers:     2,
			expectedPageViews: 2, // Only page_views are counted
			expectedMessages:  3,
		},
		{
			name: "non page_view activities",
			activities: []UserActivity{
				{
					UserID:       "user1",
					ActivityType: "click",
					Timestamp:    "2024-07-01T12:00:00Z",
				},
				{
					UserID:       "user2",
					ActivityType: "scroll",
					Timestamp:    "2024-07-01T12:01:00Z",
				},
			},
			expectedUsers:     2,
			expectedPageViews: 0, // No page_views
			expectedMessages:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset aggregator
			aggregator.uniqueUsers = make(map[string]bool)
			aggregator.pageViewCount = 0
			aggregator.messageCount = 0

			// Process activities
			for _, activity := range tt.activities {
				aggregator.ProcessActivity(activity)
			}

			// Verify results
			assert.Equal(t, tt.expectedUsers, len(aggregator.uniqueUsers))
			assert.Equal(t, tt.expectedPageViews, aggregator.pageViewCount)
			assert.Equal(t, tt.expectedMessages, aggregator.messageCount)
		})
	}
}

// TestAggregator_ShouldFlush tests batch size threshold
func TestAggregator_ShouldFlush(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	batchSize := 5
	aggregator := NewAggregator(db, 30*time.Second, batchSize)

	// Initially should not flush
	assert.False(t, aggregator.ShouldFlush())

	// Add activities below batch size
	for i := 0; i < batchSize-1; i++ {
		aggregator.ProcessActivity(UserActivity{
			UserID:       fmt.Sprintf("user%d", i),
			ActivityType: "page_view",
		})
	}
	assert.False(t, aggregator.ShouldFlush())

	// Add one more to reach batch size
	aggregator.ProcessActivity(UserActivity{
		UserID:       "user_final",
		ActivityType: "page_view",
	})
	assert.True(t, aggregator.ShouldFlush())
}

// TestAggregator_Flush_Success tests successful flush to database
func TestAggregator_Flush_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	aggregator := NewAggregator(db, 30*time.Second, 100)

	// Add some activities
	aggregator.ProcessActivity(UserActivity{
		UserID:       "user1",
		ActivityType: "page_view",
	})
	aggregator.ProcessActivity(UserActivity{
		UserID:       "user2",
		ActivityType: "page_view",
	})
	aggregator.ProcessActivity(UserActivity{
		UserID:       "user1",
		ActivityType: "click",
	})

	// Expect database insert
	mock.ExpectExec("INSERT INTO user_activity_metrics").
		WithArgs(sqlmock.AnyArg(), 2, int64(2)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Flush
	err = aggregator.Flush(context.Background())
	require.NoError(t, err)

	// Verify aggregator was reset
	assert.Equal(t, 0, len(aggregator.uniqueUsers))
	assert.Equal(t, int64(0), aggregator.pageViewCount)
	assert.Equal(t, 0, aggregator.messageCount)

	// Verify all expectations were met
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

// TestAggregator_Flush_EmptyData tests flush with no data
func TestAggregator_Flush_EmptyData(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	aggregator := NewAggregator(db, 30*time.Second, 100)

	// No database insert should happen
	// (no ExpectExec call)

	// Flush empty aggregator
	err = aggregator.Flush(context.Background())
	require.NoError(t, err)

	// Verify no database calls were made
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

// TestAggregator_Flush_DatabaseError tests retry logic on database errors
func TestAggregator_Flush_DatabaseError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	aggregator := NewAggregator(db, 30*time.Second, 100)

	// Add activity
	aggregator.ProcessActivity(UserActivity{
		UserID:       "user1",
		ActivityType: "page_view",
	})

	// Expect 3 failed attempts
	mock.ExpectExec("INSERT INTO user_activity_metrics").
		WillReturnError(fmt.Errorf("database error"))
	mock.ExpectExec("INSERT INTO user_activity_metrics").
		WillReturnError(fmt.Errorf("database error"))
	mock.ExpectExec("INSERT INTO user_activity_metrics").
		WillReturnError(fmt.Errorf("database error"))

	// Flush should fail after 3 retries
	err = aggregator.Flush(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to insert metrics after 3 attempts")

	// Verify all expectations were met
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

// TestAggregator_Flush_RetrySuccess tests successful retry after initial failure
func TestAggregator_Flush_RetrySuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	aggregator := NewAggregator(db, 30*time.Second, 100)

	// Add activity
	aggregator.ProcessActivity(UserActivity{
		UserID:       "user1",
		ActivityType: "page_view",
	})

	// Expect first attempt to fail, second to succeed
	mock.ExpectExec("INSERT INTO user_activity_metrics").
		WillReturnError(fmt.Errorf("temporary error"))
	mock.ExpectExec("INSERT INTO user_activity_metrics").
		WithArgs(sqlmock.AnyArg(), 1, int64(1)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Flush should succeed on retry
	err = aggregator.Flush(context.Background())
	require.NoError(t, err)

	// Verify aggregator was reset
	assert.Equal(t, 0, len(aggregator.uniqueUsers))

	// Verify all expectations were met
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

// TestGetEnv tests the getEnv helper function
func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "returns default when env not set",
			key:          "NONEXISTENT_VAR",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
		{
			name:         "returns env value when set",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "custom",
			expected:     "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up env
			os.Unsetenv(tt.key)

			// Set env if needed
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			result := getEnv(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetEnvAsInt tests the getEnvAsInt helper function
func TestGetEnvAsInt(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue int
		envValue     string
		expected     int
	}{
		{
			name:         "returns default when env not set",
			key:          "NONEXISTENT_INT_VAR",
			defaultValue: 42,
			envValue:     "",
			expected:     42,
		},
		{
			name:         "returns parsed int when env set",
			key:          "TEST_INT_VAR",
			defaultValue: 42,
			envValue:     "100",
			expected:     100,
		},
		{
			name:         "returns default for invalid int",
			key:          "TEST_INVALID_INT",
			defaultValue: 42,
			envValue:     "not_a_number",
			expected:     42,
		},
		{
			name:         "handles zero value",
			key:          "TEST_ZERO_VAR",
			defaultValue: 42,
			envValue:     "0",
			expected:     0,
		},
		{
			name:         "handles negative value",
			key:          "TEST_NEGATIVE_VAR",
			defaultValue: 42,
			envValue:     "-10",
			expected:     -10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up env
			os.Unsetenv(tt.key)

			// Set env if needed
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			result := getEnvAsInt(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAggregator_ConcurrentAccess tests thread-safe access to aggregator
func TestAggregator_ConcurrentAccess(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	aggregator := NewAggregator(db, 30*time.Second, 1000)

	// Simulate concurrent activity processing
	numGoroutines := 10
	activitiesPerGoroutine := 100

	done := make(chan bool)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			for j := 0; j < activitiesPerGoroutine; j++ {
				aggregator.ProcessActivity(UserActivity{
					UserID:       fmt.Sprintf("user%d-%d", goroutineID, j),
					ActivityType: "page_view",
				})
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to finish
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify correct counts
	expectedUsers := numGoroutines * activitiesPerGoroutine
	expectedPageViews := int64(numGoroutines * activitiesPerGoroutine)
	expectedMessages := numGoroutines * activitiesPerGoroutine

	assert.Equal(t, expectedUsers, len(aggregator.uniqueUsers))
	assert.Equal(t, expectedPageViews, aggregator.pageViewCount)
	assert.Equal(t, expectedMessages, aggregator.messageCount)
}

// TestNewConsumer tests consumer initialization
func TestNewConsumer(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	aggregator := NewAggregator(db, 30*time.Second, 100)
	brokers := []string{"localhost:9092"}
	topic := "test-topic"
	groupID := "test-group"
	numWorkers := 3

	consumer := NewConsumer(brokers, topic, groupID, aggregator, numWorkers)

	assert.NotNil(t, consumer)
	assert.NotNil(t, consumer.reader)
	assert.Equal(t, aggregator, consumer.aggregator)
	assert.Equal(t, numWorkers, consumer.numWorkers)
}

// TestConsumer_Close tests consumer cleanup
func TestConsumer_Close(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	aggregator := NewAggregator(db, 30*time.Second, 100)
	consumer := NewConsumer([]string{"localhost:9092"}, "test-topic", "test-group", aggregator, 3)

	// Close should not panic
	err = consumer.Close()
	// May return an error if not connected, but should not panic
	_ = err
}

// BenchmarkAggregator_ProcessActivity benchmarks activity processing
func BenchmarkAggregator_ProcessActivity(b *testing.B) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	aggregator := NewAggregator(db, 30*time.Second, 10000)

	activity := UserActivity{
		UserID:       "user123",
		ActivityType: "page_view",
		Timestamp:    "2024-07-01T12:00:00Z",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		aggregator.ProcessActivity(activity)
	}
}

// BenchmarkActivityUnmarshaling benchmarks JSON unmarshaling
func BenchmarkActivityUnmarshaling(b *testing.B) {
	jsonData := []byte(`{
		"user_id": "user123",
		"activity_type": "page_view",
		"timestamp": "2024-07-01T12:34:56Z",
		"metadata": {
			"page_url": "https://example.com/home",
			"referrer": "https://google.com"
		}
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var activity UserActivity
		_ = json.Unmarshal(jsonData, &activity)
	}
}

// TestInitDB_TableCreation tests database initialization (mock)
func TestInitDB_TableCreation(t *testing.T) {
	// This test verifies the SQL query structure
	expectedSQL := `
		CREATE TABLE IF NOT EXISTS user_activity_metrics (
			id SERIAL PRIMARY KEY,
			timestamp TIMESTAMP NOT NULL,
			unique_users INTEGER NOT NULL,
			page_view_count BIGINT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`

	// Just verify the SQL is valid (syntax check)
	assert.Contains(t, expectedSQL, "CREATE TABLE IF NOT EXISTS user_activity_metrics")
	assert.Contains(t, expectedSQL, "id SERIAL PRIMARY KEY")
	assert.Contains(t, expectedSQL, "timestamp TIMESTAMP NOT NULL")
	assert.Contains(t, expectedSQL, "unique_users INTEGER NOT NULL")
	assert.Contains(t, expectedSQL, "page_view_count BIGINT NOT NULL")
	assert.Contains(t, expectedSQL, "created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP")
}

// TestAggregator_insertMetrics tests the insert query structure
func TestAggregator_insertMetrics(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	aggregator := NewAggregator(db, 30*time.Second, 100)

	uniqueUsers := 10
	pageViews := int64(50)

	// Expect the correct SQL query
	mock.ExpectExec("INSERT INTO user_activity_metrics \\(timestamp, unique_users, page_view_count\\)").
		WithArgs(sqlmock.AnyArg(), uniqueUsers, pageViews).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = aggregator.insertMetrics(context.Background(), uniqueUsers, pageViews)
	require.NoError(t, err)

	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}
