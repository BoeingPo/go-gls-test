package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/boeing/go-gls-test/internal/models"
	"github.com/lib/pq"
)

// Repository provides access to the database.
type Repository struct {
	db *sql.DB
}

// New creates a new Repository.
func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// GetUserByID fetches a user by their ID.
func (r *Repository) GetUserByID(ctx context.Context, userID int64) (*models.User, error) {
	var user models.User
	err := r.db.QueryRowContext(ctx,
		`SELECT id, age, country, subscription_type, created_at FROM users WHERE id = $1`,
		userID,
	).Scan(&user.ID, &user.Age, &user.Country, &user.SubscriptionType, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &user, nil
}

// GetUserWatchHistoryWithGenres fetches the last 50 watch history items with genre info.
func (r *Repository) GetUserWatchHistoryWithGenres(ctx context.Context, userID int64) ([]models.WatchHistoryWithGenre, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT c.id, c.genre, uwh.watched_at
		FROM user_watch_history uwh
		JOIN content c ON uwh.content_id = c.id
		WHERE uwh.user_id = $1
		ORDER BY uwh.watched_at DESC
		LIMIT 50`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get watch history: %w", err)
	}
	defer rows.Close()

	var history []models.WatchHistoryWithGenre
	for rows.Next() {
		var h models.WatchHistoryWithGenre
		if err := rows.Scan(&h.ContentID, &h.Genre, &h.WatchedAt); err != nil {
			return nil, fmt.Errorf("scan watch history: %w", err)
		}
		history = append(history, h)
	}
	return history, rows.Err()
}

// GetUnwatchedContent fetches content not watched by the user, ordered by popularity.
func (r *Repository) GetUnwatchedContent(ctx context.Context, userID int64, limit int) ([]models.Content, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, title, genre, popularity_score, created_at
		FROM content
		WHERE id NOT IN (
			SELECT content_id
			FROM user_watch_history
			WHERE user_id = $1
		)
		ORDER BY popularity_score DESC
		LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get unwatched content: %w", err)
	}
	defer rows.Close()

	var contents []models.Content
	for rows.Next() {
		var c models.Content
		if err := rows.Scan(&c.ID, &c.Title, &c.Genre, &c.PopularityScore, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan content: %w", err)
		}
		contents = append(contents, c)
	}
	return contents, rows.Err()
}

// GetRestrictedGenresByCountry returns genres restricted for a given country.
func (r *Repository) GetRestrictedGenresByCountry(ctx context.Context, country string) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT genre FROM restrict_country_genre WHERE country = $1`,
		country,
	)
	if err != nil {
		return nil, fmt.Errorf("get restricted genres: %w", err)
	}
	defer rows.Close()

	restricted := make(map[string]bool)
	for rows.Next() {
		var genre string
		if err := rows.Scan(&genre); err != nil {
			return nil, fmt.Errorf("scan restricted genre: %w", err)
		}
		restricted[genre] = true
	}
	return restricted, rows.Err()
}

// GetAllowedGenresBySubscription returns genres allowed for a subscription type.
// If no records exist for the subscription, all genres are allowed (returns nil).
func (r *Repository) GetAllowedGenresBySubscription(ctx context.Context, subscription string) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT genre FROM subscription_genre WHERE subscription = $1`,
		subscription,
	)
	if err != nil {
		return nil, fmt.Errorf("get allowed genres: %w", err)
	}
	defer rows.Close()

	allowed := make(map[string]bool)
	for rows.Next() {
		var genre string
		if err := rows.Scan(&genre); err != nil {
			return nil, fmt.Errorf("scan allowed genre: %w", err)
		}
		allowed[genre] = true
	}
	if len(allowed) == 0 {
		return nil, rows.Err() // nil means all genres allowed
	}
	return allowed, rows.Err()
}

// GetTotalUserCount returns the total number of users.
func (r *Repository) GetTotalUserCount(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

// GetPaginatedUserIDs returns a page of user IDs.
func (r *Repository) GetPaginatedUserIDs(ctx context.Context, page, limit int) ([]int64, error) {
	offset := (page - 1) * limit
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM users ORDER BY id LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get paginated user ids: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan user id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetUsersByIDs fetches multiple users by their IDs (batch query to avoid N+1).
func (r *Repository) GetUsersByIDs(ctx context.Context, userIDs []int64) (map[int64]*models.User, error) {
	if len(userIDs) == 0 {
		return make(map[int64]*models.User), nil
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, age, country, subscription_type, created_at FROM users WHERE id = ANY($1)`,
		pq.Array(userIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("get users by ids: %w", err)
	}
	defer rows.Close()

	users := make(map[int64]*models.User)
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Age, &u.Country, &u.SubscriptionType, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users[u.ID] = &u
	}
	return users, rows.Err()
}

// GetBatchWatchHistoryWithGenres fetches watch histories for multiple users in one query.
func (r *Repository) GetBatchWatchHistoryWithGenres(ctx context.Context, userIDs []int64) (map[int64][]models.WatchHistoryWithGenre, error) {
	if len(userIDs) == 0 {
		return make(map[int64][]models.WatchHistoryWithGenre), nil
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT uwh.user_id, c.id, c.genre, uwh.watched_at
		FROM user_watch_history uwh
		JOIN content c ON uwh.content_id = c.id
		WHERE uwh.user_id = ANY($1)
		ORDER BY uwh.user_id, uwh.watched_at DESC`,
		pq.Array(userIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("get batch watch history: %w", err)
	}
	defer rows.Close()

	result := make(map[int64][]models.WatchHistoryWithGenre)
	for rows.Next() {
		var userID int64
		var h models.WatchHistoryWithGenre
		if err := rows.Scan(&userID, &h.ContentID, &h.Genre, &h.WatchedAt); err != nil {
			return nil, fmt.Errorf("scan batch watch history: %w", err)
		}
		// Limit to 50 per user
		if len(result[userID]) < 50 {
			result[userID] = append(result[userID], h)
		}
	}
	return result, rows.Err()
}

// GetBatchUnwatchedContent fetches unwatched content for multiple users.
func (r *Repository) GetBatchUnwatchedContent(ctx context.Context, userIDs []int64, limit int) (map[int64][]models.Content, error) {
	if len(userIDs) == 0 {
		return make(map[int64][]models.Content), nil
	}

	// Get all content once
	contentRows, err := r.db.QueryContext(ctx,
		`SELECT id, title, genre, popularity_score, created_at FROM content ORDER BY popularity_score DESC`)
	if err != nil {
		return nil, fmt.Errorf("get all content: %w", err)
	}
	defer contentRows.Close()

	var allContent []models.Content
	for contentRows.Next() {
		var c models.Content
		if err := contentRows.Scan(&c.ID, &c.Title, &c.Genre, &c.PopularityScore, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan content: %w", err)
		}
		allContent = append(allContent, c)
	}
	if err := contentRows.Err(); err != nil {
		return nil, err
	}

	// Get watched content IDs per user
	watchRows, err := r.db.QueryContext(ctx,
		`SELECT user_id, content_id FROM user_watch_history WHERE user_id = ANY($1)`,
		pq.Array(userIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("get watched content ids: %w", err)
	}
	defer watchRows.Close()

	watchedByUser := make(map[int64]map[int64]bool)
	for watchRows.Next() {
		var userID, contentID int64
		if err := watchRows.Scan(&userID, &contentID); err != nil {
			return nil, fmt.Errorf("scan watched: %w", err)
		}
		if watchedByUser[userID] == nil {
			watchedByUser[userID] = make(map[int64]bool)
		}
		watchedByUser[userID][contentID] = true
	}
	if err := watchRows.Err(); err != nil {
		return nil, err
	}

	// Build per-user unwatched content
	result := make(map[int64][]models.Content)
	for _, uid := range userIDs {
		watched := watchedByUser[uid]
		var unwatched []models.Content
		for _, c := range allContent {
			if watched != nil && watched[c.ID] {
				continue
			}
			unwatched = append(unwatched, c)
			if len(unwatched) >= limit {
				break
			}
		}
		result[uid] = unwatched
	}
	return result, nil
}
