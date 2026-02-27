package models

import "time"

// User represents a user record from the database.
type User struct {
	ID               int64     `json:"id" db:"id"`
	Age              int       `json:"age" db:"age"`
	Country          string    `json:"country" db:"country"`
	SubscriptionType string    `json:"subscription_type" db:"subscription_type"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

// Content represents a content record from the database.
type Content struct {
	ID              int64     `json:"id" db:"id"`
	Title           string    `json:"title" db:"title"`
	Genre           string    `json:"genre" db:"genre"`
	PopularityScore float64   `json:"popularity_score" db:"popularity_score"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

// WatchHistory represents a user watch history record.
type WatchHistory struct {
	ID        int64     `json:"id" db:"id"`
	UserID    int64     `json:"user_id" db:"user_id"`
	ContentID int64     `json:"content_id" db:"content_id"`
	WatchedAt time.Time `json:"watched_at" db:"watched_at"`
}

// WatchHistoryWithGenre represents watch history joined with content genre.
type WatchHistoryWithGenre struct {
	ContentID int64     `json:"content_id" db:"id"`
	Genre     string    `json:"genre" db:"genre"`
	WatchedAt time.Time `json:"watched_at" db:"watched_at"`
}

// RestrictCountryGenre represents a country-genre restriction.
type RestrictCountryGenre struct {
	ID      int    `json:"id" db:"id"`
	Country string `json:"country" db:"country"`
	Genre   string `json:"genre" db:"genre"`
}

// SubscriptionGenre represents subscription-genre permissions.
type SubscriptionGenre struct {
	ID           int    `json:"id" db:"id"`
	Subscription string `json:"subscription" db:"subscription"`
	Genre        string `json:"genre" db:"genre"`
}

// Recommendation represents a single recommendation item in the API response.
type Recommendation struct {
	ContentID       int64   `json:"content_id"`
	Title           string  `json:"title"`
	Genre           string  `json:"genre"`
	PopularityScore float64 `json:"popularity_score"`
	Score           float64 `json:"score"`
}

// RecommendationResponse is the response for single user recommendation.
type RecommendationResponse struct {
	UserID          int64            `json:"user_id"`
	Recommendations []Recommendation `json:"recommendations"`
	Metadata        Metadata         `json:"metadata"`
}

// Metadata holds metadata about the recommendation response.
type Metadata struct {
	CacheHit    bool   `json:"cache_hit"`
	GeneratedAt string `json:"generated_at"`
	TotalCount  int    `json:"total_count"`
}

// BatchResult represents a single user result in batch processing.
type BatchResult struct {
	UserID          int64            `json:"user_id"`
	Recommendations []Recommendation `json:"recommendations,omitempty"`
	Status          string           `json:"status"`
	Error           string           `json:"error,omitempty"`
	Message         string           `json:"message,omitempty"`
}

// BatchSummary holds summary stats for batch processing.
type BatchSummary struct {
	SuccessCount     int   `json:"success_count"`
	FailedCount      int   `json:"failed_count"`
	ProcessingTimeMs int64 `json:"processing_time_ms"`
}

// BatchResponse is the response for the batch recommendation endpoint.
type BatchResponse struct {
	Page       int           `json:"page"`
	Limit      int           `json:"limit"`
	TotalUsers int           `json:"total_users"`
	Results    []BatchResult `json:"results"`
	Summary    BatchSummary  `json:"summary"`
	Metadata   struct {
		GeneratedAt string `json:"generated_at"`
	} `json:"metadata"`
}

// ErrorResponse is a standard error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
