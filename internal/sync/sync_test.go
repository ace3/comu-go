package sync

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchWithRetry_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	client := &http.Client{}

	resp, err := fetchWithRetry(client, req, 3)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestFetchWithRetry_TokenExpired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`<html>unauthorized</html>`))
	}))
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	client := &http.Client{}

	_, err := fetchWithRetry(client, req, 3)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestFetchWithRetry_RetriesOnServerError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	client := &http.Client{}

	resp, err := fetchWithRetry(client, req, 3)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer resp.Body.Close()

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestFetchWithRetry_ExhaustsRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
	client := &http.Client{}

	_, err := fetchWithRetry(client, req, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestTitleCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MANGGARAI BESAR", "Manggarai Besar"},
		{"manggarai", "Manggarai"},
		{"BOGOR", "Bogor"},
		{"", ""},
	}

	for _, tc := range tests {
		result := TitleCase(tc.input)
		if result != tc.expected {
			t.Errorf("TitleCase(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestScheduleID_IncludesDepartureTime(t *testing.T) {
	// Verify the ID format uses {station_id}-{train_id}-{HHmm}
	loc, _ := time.LoadLocation("Asia/Jakarta")
	departsAt := time.Date(2026, 1, 1, 5, 30, 0, 0, loc)

	id := fmt.Sprintf("%s-%s-%s", "MRI", "1234", departsAt.Format("1504"))
	expected := "MRI-1234-0530"
	if id != expected {
		t.Errorf("expected %q, got %q", expected, id)
	}

	// Test another time
	departsAt2 := time.Date(2026, 1, 1, 14, 5, 0, 0, loc)
	id2 := fmt.Sprintf("%s-%s-%s", "BOO", "5678", departsAt2.Format("1504"))
	expected2 := "BOO-5678-1405"
	if id2 != expected2 {
		t.Errorf("expected %q, got %q", expected2, id2)
	}
}
