package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/boeing/go-gls-test/internal/cache"
	"github.com/boeing/go-gls-test/internal/model"
	"github.com/boeing/go-gls-test/internal/models"
	"github.com/boeing/go-gls-test/internal/repository"
)

// Service handles the business logic for recommendations.
type Service struct {
	repo        *repository.Repository
	modelClient *model.Client
	cache       *cache.Cache
}

// New creates a new Service.
func New(repo *repository.Repository, modelClient *model.Client, cache *cache.Cache) *Service {
	return &Service{
		repo:        repo,
		modelClient: modelClient,
		cache:       cache,
	}
}

// GetUserRecommendations returns recommendations for a single user.
func (s *Service) GetUserRecommendations(ctx context.Context, userID int64, limit int) (*models.RecommendationResponse, error) {
	// 1. Check cache
	cached, err := s.cache.GetUserRecommendations(ctx, userID, limit)
	if err != nil {
		log.Printf("cache get error: %v", err)
		// Continue without cache on error
	}
	if cached != nil {
		cached.Metadata.CacheHit = true
		return cached, nil
	}

	// 2. Generate recommendations via model
	user, recommendations, err := s.modelClient.GenerateRecommendations(ctx, userID, limit)
	if err != nil {
		if errors.Is(err, model.ErrModelInference) {
			return nil, fmt.Errorf("model_unavailable: %w", err)
		}
		return nil, err
	}
	if user == nil {
		return nil, nil // user not found
	}

	if recommendations == nil {
		recommendations = []models.Recommendation{}
	}

	// 3. Build response
	resp := &models.RecommendationResponse{
		UserID:          userID,
		Recommendations: recommendations,
		Metadata: models.Metadata{
			CacheHit:    false,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			TotalCount:  len(recommendations),
		},
	}

	// 4. Store in cache
	if err := s.cache.SetUserRecommendations(ctx, userID, limit, resp); err != nil {
		log.Printf("cache set error: %v", err)
	}

	return resp, nil
}

// GetBatchRecommendations returns recommendations for a batch of users with pagination.
func (s *Service) GetBatchRecommendations(ctx context.Context, page, limit int) (*models.BatchResponse, error) {
	start := time.Now()

	// Check cache for the batch
	cached, err := s.cache.GetBatchRecommendations(ctx, page, limit)
	if err != nil {
		log.Printf("batch cache get error: %v", err)
	}
	if cached != nil {
		return cached, nil
	}

	// Get total user count
	totalUsers, err := s.repo.GetTotalUserCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("get total user count: %w", err)
	}

	// Get paginated user IDs
	userIDs, err := s.repo.GetPaginatedUserIDs(ctx, page, limit)
	if err != nil {
		return nil, fmt.Errorf("get paginated user ids: %w", err)
	}

	if len(userIDs) == 0 {
		resp := &models.BatchResponse{
			Page:       page,
			Limit:      limit,
			TotalUsers: totalUsers,
			Results:    []models.BatchResult{},
			Summary: models.BatchSummary{
				SuccessCount:     0,
				FailedCount:      0,
				ProcessingTimeMs: time.Since(start).Milliseconds(),
			},
		}
		resp.Metadata.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
		return resp, nil
	}

	// Prefetch data in bulk to avoid N+1 queries
	users, err := s.repo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("get users by ids: %w", err)
	}

	watchHistories, err := s.repo.GetBatchWatchHistoryWithGenres(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("get batch watch histories: %w", err)
	}

	unwatchedContent, err := s.repo.GetBatchUnwatchedContent(ctx, userIDs, 100)
	if err != nil {
		return nil, fmt.Errorf("get batch unwatched content: %w", err)
	}

	// Prefetch restriction/permission data for all unique countries/subscriptions
	countrySet := make(map[string]bool)
	subscriptionSet := make(map[string]bool)
	for _, u := range users {
		countrySet[u.Country] = true
		subscriptionSet[u.SubscriptionType] = true
	}

	restrictedByCountry := make(map[string]map[string]bool)
	for country := range countrySet {
		restricted, err := s.repo.GetRestrictedGenresByCountry(ctx, country)
		if err != nil {
			return nil, fmt.Errorf("get restricted genres for %s: %w", country, err)
		}
		restrictedByCountry[country] = restricted
	}

	allowedBySub := make(map[string]map[string]bool)
	for sub := range subscriptionSet {
		allowed, err := s.repo.GetAllowedGenresBySubscription(ctx, sub)
		if err != nil {
			return nil, fmt.Errorf("get allowed genres for %s: %w", sub, err)
		}
		allowedBySub[sub] = allowed
	}

	// Process users concurrently with bounded worker pool
	// Server has 2 cores, DB pool = 10 connections, use concurrency of 5
	const maxConcurrency = 5
	defaultRecommendationLimit := 10

	type result struct {
		userID int64
		result models.BatchResult
	}

	resultsChan := make(chan result, len(userIDs))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for _, uid := range userIDs {
		wg.Add(1)
		go func(userID int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			user, ok := users[userID]
			if !ok {
				resultsChan <- result{
					userID: userID,
					result: models.BatchResult{
						UserID:  userID,
						Status:  "failed",
						Error:   "user_not_found",
						Message: fmt.Sprintf("User with ID %d not found", userID),
					},
				}
				return
			}

			history := watchHistories[userID]
			candidates := unwatchedContent[userID]
			restricted := restrictedByCountry[user.Country]
			allowed := allowedBySub[user.SubscriptionType]

			recs, err := s.modelClient.GenerateRecommendationsWithData(
				user, history, candidates, restricted, allowed, defaultRecommendationLimit,
			)
			if err != nil {
				resultsChan <- result{
					userID: userID,
					result: models.BatchResult{
						UserID:  userID,
						Status:  "failed",
						Error:   "model_inference_timeout",
						Message: "Recommendation generation exceeded timeout limit",
					},
				}
				return
			}

			if recs == nil {
				recs = []models.Recommendation{}
			}

			resultsChan <- result{
				userID: userID,
				result: models.BatchResult{
					UserID:          userID,
					Recommendations: recs,
					Status:          "success",
				},
			}
		}(uid)
	}

	wg.Wait()
	close(resultsChan)

	// Collect results in order
	resultMap := make(map[int64]models.BatchResult)
	for r := range resultsChan {
		resultMap[r.userID] = r.result
	}

	var results []models.BatchResult
	successCount := 0
	failedCount := 0
	for _, uid := range userIDs {
		br := resultMap[uid]
		results = append(results, br)
		if br.Status == "success" {
			successCount++
		} else {
			failedCount++
		}
	}

	resp := &models.BatchResponse{
		Page:       page,
		Limit:      limit,
		TotalUsers: totalUsers,
		Results:    results,
		Summary: models.BatchSummary{
			SuccessCount:     successCount,
			FailedCount:      failedCount,
			ProcessingTimeMs: time.Since(start).Milliseconds(),
		},
	}
	resp.Metadata.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	// Cache the batch result
	if err := s.cache.SetBatchRecommendations(ctx, page, limit, resp); err != nil {
		log.Printf("batch cache set error: %v", err)
	}

	return resp, nil
}

// WarmBatchCache pre-populates cache for all batch pages.
// Called by cron job every 30 minutes.
func (s *Service) WarmBatchCache(ctx context.Context) {
	log.Println("Starting batch cache warming...")
	start := time.Now()

	totalUsers, err := s.repo.GetTotalUserCount(ctx)
	if err != nil {
		log.Printf("cache warming: get total users error: %v", err)
		return
	}

	pageSize := 20
	totalPages := (totalUsers + pageSize - 1) / pageSize

	for page := 1; page <= totalPages; page++ {
		_, err := s.GetBatchRecommendations(ctx, page, pageSize)
		if err != nil {
			log.Printf("cache warming: page %d error: %v", page, err)
		}
	}

	log.Printf("Batch cache warming completed in %v", time.Since(start))
}
