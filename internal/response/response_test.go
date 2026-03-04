package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestBuildSuccess(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		BuildSuccess(c, map[string]string{"key": "value"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.Metadata.Success {
		t.Error("expected success to be true")
	}
}

func TestBuildError(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		BuildError(c, http.StatusNotFound, "not found")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Metadata.Success {
		t.Error("expected success to be false")
	}
	if resp.Metadata.Message != "not found" {
		t.Errorf("expected message 'not found', got %q", resp.Metadata.Message)
	}
}

func TestBuildPaginatedSuccess(t *testing.T) {
	data := []string{"a", "b", "c"}
	resp := BuildPaginatedSuccess(data, 2, 50, 100)

	if !resp.Metadata.Success {
		t.Error("expected success to be true")
	}
	if resp.Metadata.Page != 2 {
		t.Errorf("expected page 2, got %d", resp.Metadata.Page)
	}
	if resp.Metadata.Limit != 50 {
		t.Errorf("expected limit 50, got %d", resp.Metadata.Limit)
	}
	if resp.Metadata.Total != 100 {
		t.Errorf("expected total 100, got %d", resp.Metadata.Total)
	}
}

func TestBuildPaginatedSuccess_JSONFormat(t *testing.T) {
	data := []string{"x", "y"}
	resp := BuildPaginatedSuccess(data, 1, 10, 2)

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	meta, ok := parsed["metadata"].(map[string]any)
	if !ok {
		t.Fatal("expected metadata object")
	}

	if meta["success"] != true {
		t.Error("expected success true")
	}
	if meta["page"].(float64) != 1 {
		t.Errorf("expected page 1, got %v", meta["page"])
	}
	if meta["limit"].(float64) != 10 {
		t.Errorf("expected limit 10, got %v", meta["limit"])
	}
	if meta["total"].(float64) != 2 {
		t.Errorf("expected total 2, got %v", meta["total"])
	}
}
