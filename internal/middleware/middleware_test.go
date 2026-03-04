package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRequireSyncSecret_ValidSecret(t *testing.T) {
	r := gin.New()
	r.Use(RequireSyncSecret("my-secret"))
	r.POST("/sync", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("POST", "/sync", nil)
	req.Header.Set("X-Sync-Secret", "my-secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireSyncSecret_MissingHeader(t *testing.T) {
	r := gin.New()
	r.Use(RequireSyncSecret("my-secret"))
	r.POST("/sync", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("POST", "/sync", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireSyncSecret_WrongSecret(t *testing.T) {
	r := gin.New()
	r.Use(RequireSyncSecret("my-secret"))
	r.POST("/sync", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("POST", "/sync", nil)
	req.Header.Set("X-Sync-Secret", "wrong-secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequestID_GeneratesUUID(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		id, _ := c.Get("request_id")
		c.JSON(http.StatusOK, gin.H{"request_id": id})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	respID := w.Header().Get("X-Request-ID")
	if respID == "" {
		t.Error("expected X-Request-ID header to be set")
	}
	if len(respID) < 32 {
		t.Errorf("expected UUID-length request ID, got %q", respID)
	}
}

func TestRequestID_UsesExistingHeader(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		id, _ := c.Get("request_id")
		c.JSON(http.StatusOK, gin.H{"request_id": id})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "existing-id-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	respID := w.Header().Get("X-Request-ID")
	if respID != "existing-id-123" {
		t.Errorf("expected existing-id-123, got %q", respID)
	}
}

func TestMetricsMiddleware_RecordsRequest(t *testing.T) {
	r := gin.New()
	r.Use(MetricsMiddleware())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
