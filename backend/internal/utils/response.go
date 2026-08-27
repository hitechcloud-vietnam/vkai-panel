package utils

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type APIResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     *APIError   `json:"error,omitempty"`
	RequestID string      `json:"request_id"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

type PaginatedResponse struct {
	Items      interface{} `json:"items"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PerPage    int         `json:"per_page"`
	TotalPages int         `json:"total_pages"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{
		Success:   true,
		Data:      data,
		RequestID: GetRequestID(c),
	})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, APIResponse{
		Success:   true,
		Data:      data,
		RequestID: GetRequestID(c),
	})
}

func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func BadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, APIResponse{
		Success: false,
		Error: &APIError{
			Code:    "BAD_REQUEST",
			Message: message,
		},
		RequestID: GetRequestID(c),
	})
}

func Unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, APIResponse{
		Success: false,
		Error: &APIError{
			Code:    "UNAUTHORIZED",
			Message: message,
		},
		RequestID: GetRequestID(c),
	})
}

func Forbidden(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, APIResponse{
		Success: false,
		Error: &APIError{
			Code:    "FORBIDDEN",
			Message: message,
		},
		RequestID: GetRequestID(c),
	})
}

func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, APIResponse{
		Success: false,
		Error: &APIError{
			Code:    "NOT_FOUND",
			Message: message,
		},
		RequestID: GetRequestID(c),
	})
}

func Conflict(c *gin.Context, message string) {
	c.JSON(http.StatusConflict, APIResponse{
		Success: false,
		Error: &APIError{
			Code:    "CONFLICT",
			Message: message,
		},
		RequestID: GetRequestID(c),
	})
}

func InternalError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, APIResponse{
		Success: false,
		Error: &APIError{
			Code:    "INTERNAL_ERROR",
			Message: "An internal error occurred",
		},
		RequestID: GetRequestID(c),
	})
}

func Paginated(c *gin.Context, items interface{}, total int64, page, perPage int) {
	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	Success(c, PaginatedResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	})
}

func GetRequestID(c *gin.Context) string {
	if rid, exists := c.Get("request_id"); exists {
		return rid.(string)
	}
	return uuid.New().String()
}

// ValidationError sends a validation error response
func ValidationError(c *gin.Context, errors map[string]string) {
	c.JSON(http.StatusUnprocessableEntity, APIResponse{
		Success: false,
		Error: &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "Validation failed",
			Details: formatValidationErrors(errors),
		},
		RequestID: GetRequestID(c),
	})
}

// formatValidationErrors formats validation errors into a string
func formatValidationErrors(errors map[string]string) string {
	var details []string
	for field, message := range errors {
		details = append(details, field+": "+message)
	}
	return strings.Join(details, "; ")
}

// RateLimit sends a rate limit response
func RateLimit(c *gin.Context) {
	c.JSON(http.StatusTooManyRequests, APIResponse{
		Success: false,
		Error: &APIError{
			Code:    "RATE_LIMIT_EXCEEDED",
			Message: "Rate limit exceeded",
		},
		RequestID: GetRequestID(c),
	})
}

// ServiceUnavailable sends a service unavailable response
func ServiceUnavailable(c *gin.Context, message string) {
	c.JSON(http.StatusServiceUnavailable, APIResponse{
		Success: false,
		Error: &APIError{
			Code:    "SERVICE_UNAVAILABLE",
			Message: message,
		},
		RequestID: GetRequestID(c),
	})
}

// HealthCheck sends a health check response
func HealthCheck(c *gin.Context, status string, checks map[string]string) {
	c.JSON(http.StatusOK, gin.H{
		"status": status,
		"checks": checks,
	})
}

// Message sends a message response
func Message(c *gin.Context, message string) {
	Success(c, gin.H{"message": message})
}

// Status sends a status response
func Status(c *gin.Context, status string) {
	Success(c, gin.H{"status": status})
}
