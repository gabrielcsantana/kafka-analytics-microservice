package main

// UserActivity represents a user activity event
type UserActivity struct {
	UserID       string   `json:"user_id" binding:"required"`
	ActivityType string   `json:"activity_type" binding:"required"`
	Timestamp    string   `json:"timestamp" binding:"required"`
	Metadata     Metadata `json:"metadata" binding:"required"`
}

// Metadata contains additional information about the activity
type Metadata struct {
	PageURL  string `json:"page_url" binding:"required"`
	Referrer string `json:"referrer"`
}
