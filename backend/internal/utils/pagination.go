package utils

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// PaginationParams represents pagination parameters
type PaginationParams struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Offset  int `json:"offset"`
}

// DefaultPagination returns default pagination parameters
func DefaultPagination() PaginationParams {
	return PaginationParams{
		Page:    1,
		PerPage: 20,
		Offset:  0,
	}
}

// GetPaginationParams extracts pagination parameters from request
func GetPaginationParams(c *gin.Context) PaginationParams {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	offset := (page - 1) * perPage

	return PaginationParams{
		Page:    page,
		PerPage: perPage,
		Offset:  offset,
	}
}

// GetSortParams extracts sort parameters from request
func GetSortParams(c *gin.Context, defaultSort, defaultOrder string, allowedFields []string) (string, string) {
	sort := c.DefaultQuery("sort", defaultSort)
	order := c.DefaultQuery("order", defaultOrder)

	// Validate sort field
	validSort := false
	for _, field := range allowedFields {
		if sort == field {
			validSort = true
			break
		}
	}
	if !validSort {
		sort = defaultSort
	}

	// Validate order
	if order != "asc" && order != "desc" {
		order = defaultOrder
	}

	return sort, order
}

// GetFilterParams extracts filter parameters from request
func GetFilterParams(c *gin.Context) map[string]string {
	filters := make(map[string]string)

	// Common filter parameters
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}
	if status := c.Query("status"); status != "" {
		filters["status"] = status
	}
	if tenantID := c.Query("tenant_id"); tenantID != "" {
		filters["tenant_id"] = tenantID
	}

	return filters
}

// CalculateTotalPages calculates total pages
func CalculateTotalPages(total int64, perPage int) int {
	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}
	return totalPages
}

// PaginatedResponse represents a paginated response
type PaginatedResponse struct {
	Items      interface{} `json:"items"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PerPage    int         `json:"per_page"`
	TotalPages int         `json:"total_pages"`
}

// NewPaginatedResponse creates a new paginated response
func NewPaginatedResponse(items interface{}, total int64, params PaginationParams) PaginatedResponse {
	return PaginatedResponse{
		Items:      items,
		Total:      total,
		Page:       params.Page,
		PerPage:    params.PerPage,
		TotalPages: CalculateTotalPages(total, params.PerPage),
	}
}
