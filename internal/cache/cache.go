package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/boeing/go-gls-test/internal/models"
	"github.com/redis/go-redis/v9"
)

const defaultTTL = 10 * time.Minute

// Cache provides Redis caching for recommendations.
type Cache struct {
	client *redis.Client
}

// New creates a new Cache.
func New(client *redis.Client) *Cache {
	return &Cache{client: client}
}

// userRecommendationKey creates a cache key for single user recommendations.
func userRecommendationKey(userID int64, limit int) string {
	return fmt.Sprintf("rec:user:%d:limit:%d", userID, limit)
}

// batchKey creates a cache key for batch recommendations.
func batchKey(page, limit int) string {
	return fmt.Sprintf("rec:batch:page:%d:limit:%d", page, limit)
}

// GetUserRecommendations retrieves cached recommendations for a user.
func (c *Cache) GetUserRecommendations(ctx context.Context, userID int64, limit int) (*models.RecommendationResponse, error) {
	key := userRecommendationKey(userID, limit)
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var resp models.RecommendationResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal cache: %w", err)
	}
	return &resp, nil
}

// SetUserRecommendations stores recommendations in cache.
func (c *Cache) SetUserRecommendations(ctx context.Context, userID int64, limit int, resp *models.RecommendationResponse) error {
	key := userRecommendationKey(userID, limit)
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}
	return c.client.Set(ctx, key, data, defaultTTL).Err()
}

// GetBatchRecommendations retrieves cached batch results.
func (c *Cache) GetBatchRecommendations(ctx context.Context, page, limit int) (*models.BatchResponse, error) {
	key := batchKey(page, limit)
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get batch: %w", err)
	}

	var resp models.BatchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal batch cache: %w", err)
	}
	return &resp, nil
}

// SetBatchRecommendations stores batch results in cache.
func (c *Cache) SetBatchRecommendations(ctx context.Context, page, limit int, resp *models.BatchResponse) error {
	key := batchKey(page, limit)
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal batch cache: %w", err)
	}
	return c.client.Set(ctx, key, data, defaultTTL).Err()
}
