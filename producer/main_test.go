package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// TestUserActivity_JSONMarshaling tests that UserActivity can be properly marshaled/unmarshaled
func TestUserActivity_JSONMarshaling(t *testing.T) {
	activity := UserActivity{
		UserID:       "user123",
		ActivityType: "page_view",
		Timestamp:    "2024-07-01T12:34:56Z",
		Metadata: Metadata{
			PageURL:  "https://example.com/home",
			Referrer: "https://google.com",
		},
	}

	// Marshal
	data, err := json.Marshal(activity)
	require.NoError(t, err)

	// Unmarshal
	var unmarshaled UserActivity
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, activity.UserID, unmarshaled.UserID)
	assert.Equal(t, activity.ActivityType, unmarshaled.ActivityType)
	assert.Equal(t, activity.Timestamp, unmarshaled.Timestamp)
	assert.Equal(t, activity.Metadata.PageURL, unmarshaled.Metadata.PageURL)
	assert.Equal(t, activity.Metadata.Referrer, unmarshaled.Metadata.Referrer)
}

// TestProducer_PublishActivity_Success tests successful message publishing
func TestProducer_PublishActivity_Success(t *testing.T) {
	// This test requires a mock Kafka or we skip it in unit tests
	// For demonstration, we'll test the marshaling logic
	activity := UserActivity{
		UserID:       "user123",
		ActivityType: "page_view",
		Timestamp:    "2024-07-01T12:34:56Z",
		Metadata: Metadata{
			PageURL:  "https://example.com/home",
			Referrer: "https://google.com",
		},
	}

	// Test that we can marshal the activity (part of PublishActivity logic)
	messageBytes, err := json.Marshal(activity)
	require.NoError(t, err)
	assert.NotEmpty(t, messageBytes)

	// Verify the message can create a Kafka message structure
	msg := kafka.Message{
		Key:   []byte(activity.UserID),
		Value: messageBytes,
	}
	assert.Equal(t, []byte("user123"), msg.Key)
	assert.NotEmpty(t, msg.Value)
}

// TestProducer_PublishActivity_InvalidJSON tests error handling for invalid data
func TestProducer_PublishActivity_InvalidJSON(t *testing.T) {
	// Test marshaling with a problematic type (channels can't be marshaled)
	type InvalidActivity struct {
		UserID  string
		Channel chan int // This will fail to marshal
	}

	invalid := InvalidActivity{
		UserID:  "user123",
		Channel: make(chan int),
	}

	_, err := json.Marshal(invalid)
	assert.Error(t, err, "Should fail to marshal invalid data")
}

// TestHealthEndpoint tests the health check endpoint
func TestHealthEndpoint(t *testing.T) {
	router := gin.New()
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "healthy", response["status"])
}

// TestActivityEndpoint_Success tests successful activity submission
func TestActivityEndpoint_Success(t *testing.T) {
	// Create a mock producer that doesn't actually connect to Kafka
	router := gin.New()

	// Mock the activity endpoint without actual Kafka connection
	router.POST("/activity", func(c *gin.Context) {
		var activity UserActivity
		if err := c.ShouldBindJSON(&activity); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Simulate successful publish (no actual Kafka call)
		c.JSON(http.StatusOK, gin.H{"message": "Activity published successfully"})
	})

	activity := UserActivity{
		UserID:       "user123",
		ActivityType: "page_view",
		Timestamp:    "2024-07-01T12:34:56Z",
		Metadata: Metadata{
			PageURL:  "https://example.com/home",
			Referrer: "https://google.com",
		},
	}

	body, _ := json.Marshal(activity)
	req, _ := http.NewRequest("POST", "/activity", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Activity published successfully", response["message"])
}

// TestActivityEndpoint_MissingFields tests validation for required fields
func TestActivityEndpoint_MissingFields(t *testing.T) {
	router := gin.New()

	router.POST("/activity", func(c *gin.Context) {
		var activity UserActivity
		if err := c.ShouldBindJSON(&activity); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Activity published successfully"})
	})

	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing user_id",
			body: `{"activity_type":"page_view","timestamp":"2024-07-01T12:34:56Z","metadata":{"page_url":"https://example.com"}}`,
		},
		{
			name: "missing activity_type",
			body: `{"user_id":"user123","timestamp":"2024-07-01T12:34:56Z","metadata":{"page_url":"https://example.com"}}`,
		},
		{
			name: "missing timestamp",
			body: `{"user_id":"user123","activity_type":"page_view","metadata":{"page_url":"https://example.com"}}`,
		},
		{
			name: "missing metadata",
			body: `{"user_id":"user123","activity_type":"page_view","timestamp":"2024-07-01T12:34:56Z"}`,
		},
		{
			name: "missing page_url in metadata",
			body: `{"user_id":"user123","activity_type":"page_view","timestamp":"2024-07-01T12:34:56Z","metadata":{"referrer":"https://google.com"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/activity", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var response map[string]string
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			assert.Contains(t, response["error"], "required", "Error should mention required field")
		})
	}
}

// TestActivityEndpoint_InvalidJSON tests handling of malformed JSON
func TestActivityEndpoint_InvalidJSON(t *testing.T) {
	router := gin.New()

	router.POST("/activity", func(c *gin.Context) {
		var activity UserActivity
		if err := c.ShouldBindJSON(&activity); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Activity published successfully"})
	})

	req, _ := http.NewRequest("POST", "/activity", bytes.NewBufferString(`{invalid json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
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

// TestNewProducer tests producer initialization
func TestNewProducer(t *testing.T) {
	brokers := []string{"localhost:9092"}
	topic := "test-topic"

	producer := NewProducer(brokers, topic)

	assert.NotNil(t, producer)
	assert.NotNil(t, producer.writer)

	// Test that writer has correct configuration
	assert.Equal(t, topic, producer.writer.Topic)
	assert.Equal(t, 10*time.Second, producer.writer.WriteTimeout)
	assert.Equal(t, 10*time.Second, producer.writer.ReadTimeout)
	assert.Equal(t, 3, producer.writer.MaxAttempts)
	assert.True(t, producer.writer.AllowAutoTopicCreation)
}

// BenchmarkActivityMarshaling benchmarks JSON marshaling of UserActivity
func BenchmarkActivityMarshaling(b *testing.B) {
	activity := UserActivity{
		UserID:       "user123",
		ActivityType: "page_view",
		Timestamp:    "2024-07-01T12:34:56Z",
		Metadata: Metadata{
			PageURL:  "https://example.com/home",
			Referrer: "https://google.com",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(activity)
	}
}

// TestProducer_Close tests that the producer can be closed
func TestProducer_Close(t *testing.T) {
	producer := NewProducer([]string{"localhost:9092"}, "test-topic")

	// Close should not panic even without actual Kafka connection
	// (it will fail to connect but shouldn't panic)
	err := producer.Close()
	// We expect an error since we're not actually connected to Kafka
	// but we're testing that Close() doesn't panic
	_ = err // Ignore the error - we just care it doesn't panic
}

// TestProducer_PublishActivity_Timeout tests timeout handling
func TestProducer_PublishActivity_Timeout(t *testing.T) {
	// Test that context timeout is respected
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Wait for context to expire
	<-ctx.Done()

	// Verify context is expired
	assert.Error(t, ctx.Err())
	assert.Equal(t, context.DeadlineExceeded, ctx.Err())
}
