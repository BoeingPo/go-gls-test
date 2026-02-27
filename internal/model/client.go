package model

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"

	"github.com/boeing/go-gls-test/internal/models"
	"github.com/boeing/go-gls-test/internal/repository"
)

// ErrModelInference is returned when model inference randomly fails.
var ErrModelInference = errors.New("model inference failed")

// Client is a mock ML model client that scores content recommendations.
type Client struct {
	repo *repository.Repository
	rng  *rand.Rand
}

// New creates a new model Client.
func New(repo *repository.Repository) *Client {
	return &Client{
		repo: repo,
		rng:  rand.New(rand.NewSource(42)),
	}
}

// GenerateRecommendations generates personalized recommendations for a user.
// It fetches data through the repository, applies scoring, and returns ranked results.
func (c *Client) GenerateRecommendations(ctx context.Context, userID int64, limit int) (*models.User, []models.Recommendation, error) {
	// 1. Fetch user data
	user, err := c.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		return nil, nil, nil // user not found
	}

	// 2. Fetch watch history with genres
	watchHistory, err := c.repo.GetUserWatchHistoryWithGenres(ctx, userID)
	if err != nil {
		return user, nil, err
	}

	// 3. Calculate genre preferences
	genrePreferences := c.calculateGenrePreferences(watchHistory)

	// 4. Fetch candidate content (excluding watched)
	candidates, err := c.repo.GetUnwatchedContent(ctx, userID, 100)
	if err != nil {
		return user, nil, err
	}

	// 5. Get filtering data
	restrictedGenres, err := c.repo.GetRestrictedGenresByCountry(ctx, user.Country)
	if err != nil {
		return user, nil, err
	}
	allowedGenres, err := c.repo.GetAllowedGenresBySubscription(ctx, user.SubscriptionType)
	if err != nil {
		return user, nil, err
	}

	// 6. Simulate model latency (30-50ms)
	delay := time.Duration(30+c.rng.Intn(21)) * time.Millisecond
	time.Sleep(delay)

	// 7. Simulate random failure (1.5% failure rate)
	if c.rng.Float64() < 0.015 {
		return user, nil, ErrModelInference
	}

	// 8. Score each candidate
	var scored []models.Recommendation
	now := time.Now()

	for _, content := range candidates {
		// Filter: skip restricted genres for user's country
		if restrictedGenres[content.Genre] {
			continue
		}
		// Filter: skip genres not allowed by subscription
		if allowedGenres != nil && !allowedGenres[content.Genre] {
			continue
		}

		score := c.calculateScore(content, genrePreferences, now)
		scored = append(scored, models.Recommendation{
			ContentID:       content.ID,
			Title:           content.Title,
			Genre:           content.Genre,
			PopularityScore: content.PopularityScore,
			Score:           math.Round(score*100) / 100,
		})
	}

	// 9. Sort by score descending
	sortRecommendations(scored)

	// 10. Return top N
	if len(scored) > limit {
		scored = scored[:limit]
	}

	return user, scored, nil
}

// GenerateRecommendationsWithData generates recommendations using pre-fetched data.
// Used by batch processing to avoid N+1 queries.
func (c *Client) GenerateRecommendationsWithData(
	user *models.User,
	watchHistory []models.WatchHistoryWithGenre,
	candidates []models.Content,
	restrictedGenres map[string]bool,
	allowedGenres map[string]bool,
	limit int,
) ([]models.Recommendation, error) {
	// Calculate genre preferences
	genrePreferences := c.calculateGenrePreferences(watchHistory)

	// Simulate model latency (30-50ms)
	delay := time.Duration(30+c.rng.Intn(21)) * time.Millisecond
	time.Sleep(delay)

	// Simulate random failure (1.5% failure rate)
	if c.rng.Float64() < 0.015 {
		return nil, ErrModelInference
	}

	// Score each candidate
	var scored []models.Recommendation
	now := time.Now()

	for _, content := range candidates {
		// Filter: skip restricted genres
		if restrictedGenres[content.Genre] {
			continue
		}
		// Filter: skip genres not allowed by subscription
		if allowedGenres != nil && !allowedGenres[content.Genre] {
			continue
		}

		score := c.calculateScore(content, genrePreferences, now)
		scored = append(scored, models.Recommendation{
			ContentID:       content.ID,
			Title:           content.Title,
			Genre:           content.Genre,
			PopularityScore: content.PopularityScore,
			Score:           math.Round(score*100) / 100,
		})
	}

	// Sort by score descending
	sortRecommendations(scored)

	// Return top N
	if len(scored) > limit {
		scored = scored[:limit]
	}

	return scored, nil
}

func (c *Client) calculateGenrePreferences(watchHistory []models.WatchHistoryWithGenre) map[string]float64 {
	genreCounts := make(map[string]int)
	for _, h := range watchHistory {
		genreCounts[h.Genre]++
	}
	totalWatches := len(watchHistory)
	if totalWatches == 0 {
		return make(map[string]float64)
	}

	preferences := make(map[string]float64)
	for genre, count := range genreCounts {
		preferences[genre] = float64(count) / float64(totalWatches)
	}
	return preferences
}

func (c *Client) calculateScore(content models.Content, genrePreferences map[string]float64, now time.Time) float64 {
	// Popularity component (40%)
	popularityComponent := content.PopularityScore * 0.4

	// Genre match component (35%)
	genreBoost := 0.1
	if pref, ok := genrePreferences[content.Genre]; ok {
		genreBoost = pref
	}
	genreComponent := genreBoost * 0.35

	// Recency component (15%)
	daysSinceCreation := now.Sub(content.CreatedAt).Hours() / 24.0
	recencyFactor := 1.0 / (1.0 + daysSinceCreation/365.0)
	recencyComponent := recencyFactor * 0.15

	// Exploration component (10%) - controlled randomness
	randomNoise := (c.rng.Float64()*0.1 - 0.05) * 0.1

	return popularityComponent + genreComponent + recencyComponent + randomNoise
}

func sortRecommendations(recs []models.Recommendation) {
	// Simple insertion sort (good enough for <100 items)
	for i := 1; i < len(recs); i++ {
		for j := i; j > 0 && recs[j].Score > recs[j-1].Score; j-- {
			recs[j], recs[j-1] = recs[j-1], recs[j]
		}
	}
}
