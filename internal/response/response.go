package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Metadata holds success/error status for the response envelope.
type Metadata struct {
	Success         bool   `json:"success"`
	Message         string `json:"message,omitempty"`
	PlannerMode     string `json:"planner_mode,omitempty"`
	Projected       bool   `json:"projected,omitempty"`
	SnapshotDateWIB string `json:"snapshot_date_wib,omitempty"`
	SnapshotAgeDays int    `json:"snapshot_age_days,omitempty"`
	SyncTriggered   bool   `json:"sync_triggered,omitempty"`
}

// PaginatedMetadata extends Metadata with pagination info.
type PaginatedMetadata struct {
	Success         bool   `json:"success"`
	Message         string `json:"message,omitempty"`
	Page            int    `json:"page"`
	Limit           int    `json:"limit"`
	Total           int    `json:"total"`
	Projected       bool   `json:"projected,omitempty"`
	SnapshotDateWIB string `json:"snapshot_date_wib,omitempty"`
	SnapshotAgeDays int    `json:"snapshot_age_days,omitempty"`
	SyncTriggered   bool   `json:"sync_triggered,omitempty"`
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
	BuildSuccessWithMetadata(c, Metadata{Success: true}, data)
}

// BuildSuccessWithMetadata sends a 200 JSON response with custom metadata.
func BuildSuccessWithMetadata(c *gin.Context, metadata Metadata, data any) {
	c.JSON(http.StatusOK, Response{
		Metadata: metadata,
		Data:     data,
	})
}

// BuildPaginatedSuccess builds a paginated response with metadata.
func BuildPaginatedSuccess(data any, page, limit, total int) PaginatedResponse {
	return BuildPaginatedSuccessWithMetadata(
		data,
		PaginatedMetadata{
			Success: true,
			Page:    page,
			Limit:   limit,
			Total:   total,
		},
	)
}

// BuildPaginatedSuccessWithMetadata builds a paginated response with custom metadata.
func BuildPaginatedSuccessWithMetadata(data any, metadata PaginatedMetadata) PaginatedResponse {
	return PaginatedResponse{
		Metadata: metadata,
		Data:     data,
	}
}

// BuildError sends a JSON error response with the standard envelope.
func BuildError(c *gin.Context, status int, message string) {
	c.JSON(status, Response{
		Metadata: Metadata{Success: false, Message: message},
		Data:     nil,
	})
}
