package main

// UserActivity represents a user activity event
type UserActivity struct {
	UserID       string   `json:"user_id"`
	ActivityType string   `json:"activity_type"`
	Timestamp    string   `json:"timestamp"`
	Metadata     Metadata `json:"metadata"`
}

// Metadata contains additional information about the activity
type Metadata struct {
	PageURL  string `json:"page_url"`
	Referrer string `json:"referrer"`
}
