package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
)

// Producer handles publishing user activity events to Kafka
type Producer struct {
	writer *kafka.Writer
}

// NewProducer creates a new Producer instance
func NewProducer(brokers []string, topic string) *Producer {
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  topic,
		Balancer:               &kafka.LeastBytes{},
		AllowAutoTopicCreation: true,
		WriteTimeout:           10 * time.Second,
		ReadTimeout:            10 * time.Second,
		MaxAttempts:            3,
	}

	return &Producer{writer: writer}
}

// PublishActivity publishes a user activity event to Kafka
func (p *Producer) PublishActivity(ctx context.Context, activity UserActivity) error {
	messageBytes, err := json.Marshal(activity)
	if err != nil {
		return err
	}

	err = p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(activity.UserID),
		Value: messageBytes,
	})

	return err
}

// Close closes the Kafka writer connection
func (p *Producer) Close() error {
	return p.writer.Close()
}
