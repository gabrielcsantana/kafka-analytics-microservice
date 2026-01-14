package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// Configuration from environment variables
	kafkaBrokers := getEnv("KAFKA_BROKERS", "localhost:9092")
	kafkaTopic := getEnv("KAFKA_TOPIC", "incoming.user_activity")
	serverPort := getEnv("SERVER_PORT", "8080")

	// Create producer
	producer := NewProducer([]string{kafkaBrokers}, kafkaTopic)
	defer producer.Close()

	// Set up Gin router
	router := gin.Default()

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Activity endpoint
	router.POST("/activity", func(c *gin.Context) {
		var activity UserActivity
		if err := c.ShouldBindJSON(&activity); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		if err := producer.PublishActivity(ctx, activity); err != nil {
			log.Printf("Failed to publish activity: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to publish activity"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Activity published successfully"})
	})

	// Create HTTP server
	srv := &http.Server{
		Addr:    ":" + serverPort,
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Starting producer server on port %s", serverPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down producer server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Producer server exited")
}
