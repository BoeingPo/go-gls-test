package handler

import (
	"errors"
	"strconv"
	"strings"

	"github.com/boeing/go-gls-test/internal/models"
	"github.com/boeing/go-gls-test/internal/service"
	"github.com/gofiber/fiber/v2"
)

// Handler handles HTTP requests.
type Handler struct {
	svc *service.Service
}

// New creates a new Handler.
func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all routes on the Fiber app.
func (h *Handler) RegisterRoutes(app *fiber.App) {
	app.Get("/users/:user_id/recommendations", h.GetUserRecommendations)
	app.Get("/recommendations/batch", h.GetBatchRecommendations)
}

// GetUserRecommendations handles GET /users/:user_id/recommendations
func (h *Handler) GetUserRecommendations(c *fiber.Ctx) error {
	// Parse user_id
	userIDStr := c.Params("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "invalid_parameter",
			Message: "Invalid user_id parameter",
		})
	}

	// Parse limit (optional, default 10)
	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "invalid_parameter",
				Message: "Invalid limit parameter",
			})
		}
		limit = l
	}

	// Get recommendations
	resp, err := h.svc.GetUserRecommendations(c.Context(), userID, limit)
	if err != nil {
		if strings.Contains(err.Error(), "model_unavailable") {
			return c.Status(fiber.StatusServiceUnavailable).JSON(models.ErrorResponse{
				Error:   "model_unavailable",
				Message: "Recommendation model is temporarily unavailable",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "internal_error",
			Message: "An unexpected error occurred",
		})
	}

	if resp == nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "user_not_found",
			Message: "User with ID " + userIDStr + " does not exist",
		})
	}

	return c.JSON(resp)
}

// GetBatchRecommendations handles GET /recommendations/batch
func (h *Handler) GetBatchRecommendations(c *fiber.Ctx) error {
	// Parse page (optional, default 1)
	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err != nil || p < 1 {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "invalid_parameter",
				Message: "Invalid page parameter",
			})
		}
		page = p
	}

	// Parse limit (optional, default 20, min 1, max 100)
	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l < 1 || l > 100 {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "invalid_parameter",
				Message: "Invalid limit parameter",
			})
		}
		limit = l
	}

	resp, err := h.svc.GetBatchRecommendations(c.Context(), page, limit)
	if err != nil {
		if strings.Contains(err.Error(), "model_unavailable") || errors.Is(err, errors.New("model_unavailable")) {
			return c.Status(fiber.StatusServiceUnavailable).JSON(models.ErrorResponse{
				Error:   "model_unavailable",
				Message: "Recommendation model is temporarily unavailable",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "internal_error",
			Message: "An unexpected error occurred",
		})
	}

	return c.JSON(resp)
}
