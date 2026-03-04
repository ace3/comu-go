package handlers

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

// parsePagination extracts page and limit from query parameters.
// Defaults: page=1, limit=100. Max limit is 500.
func parsePagination(c *gin.Context) (page, limit int) {
	page = 1
	limit = 100

	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}
	if limit > 500 {
		limit = 500
	}

	return page, limit
}

// paginationCacheKey builds a cache key including pagination params.
func paginationCacheKey(prefix string, page, limit int) string {
	return fmt.Sprintf("%s:page%d:limit%d", prefix, page, limit)
}
