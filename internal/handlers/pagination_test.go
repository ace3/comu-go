package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestParsePagination_Defaults(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		page, limit := parsePagination(c)
		if page != 1 {
			t.Errorf("expected default page 1, got %d", page)
		}
		if limit != 100 {
			t.Errorf("expected default limit 100, got %d", limit)
		}
		c.JSON(http.StatusOK, nil)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

func TestParsePagination_CustomValues(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		page, limit := parsePagination(c)
		if page != 3 {
			t.Errorf("expected page 3, got %d", page)
		}
		if limit != 50 {
			t.Errorf("expected limit 50, got %d", limit)
		}
		c.JSON(http.StatusOK, nil)
	})

	req := httptest.NewRequest("GET", "/test?page=3&limit=50", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

func TestParsePagination_MaxLimit(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		_, limit := parsePagination(c)
		if limit != 500 {
			t.Errorf("expected max limit 500, got %d", limit)
		}
		c.JSON(http.StatusOK, nil)
	})

	req := httptest.NewRequest("GET", "/test?limit=1000", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

func TestParsePagination_InvalidValues(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		page, limit := parsePagination(c)
		if page != 1 {
			t.Errorf("expected default page 1 for invalid value, got %d", page)
		}
		if limit != 100 {
			t.Errorf("expected default limit 100 for invalid value, got %d", limit)
		}
		c.JSON(http.StatusOK, nil)
	})

	req := httptest.NewRequest("GET", "/test?page=abc&limit=xyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

func TestPaginationCacheKey(t *testing.T) {
	key := paginationCacheKey("station:all", 2, 50)
	expected := "station:all:page2:limit50"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}
