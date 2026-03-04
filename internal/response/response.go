package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Metadata holds success/error status for the response envelope.
type Metadata struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// PaginatedMetadata extends Metadata with pagination info.
type PaginatedMetadata struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Page    int    `json:"page"`
	Limit   int    `json:"limit"`
	Total   int    `json:"total"`
}

// Response is the standard API response envelope.
type Response struct {
	Metadata Metadata `json:"metadata"`
	Data     any      `json:"data"`
}

// PaginatedResponse is the API response envelope with pagination metadata.
type PaginatedResponse struct {
	Metadata PaginatedMetadata `json:"metadata"`
	Data     any               `json:"data"`
}

// BuildSuccess sends a 200 JSON response with the standard envelope.
func BuildSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Metadata: Metadata{Success: true},
		Data:     data,
	})
}

// BuildPaginatedSuccess builds a paginated response with metadata.
func BuildPaginatedSuccess(data any, page, limit, total int) PaginatedResponse {
	return PaginatedResponse{
		Metadata: PaginatedMetadata{
			Success: true,
			Page:    page,
			Limit:   limit,
			Total:   total,
		},
		Data: data,
	}
}

// BuildError sends a JSON error response with the standard envelope.
func BuildError(c *gin.Context, status int, message string) {
	c.JSON(status, Response{
		Metadata: Metadata{Success: false, Message: message},
		Data:     nil,
	})
}
