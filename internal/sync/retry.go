package sync

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"
)

// ErrTokenExpired is a sentinel error returned when the KRL API responds with 401.
var ErrTokenExpired = errors.New("KAI_AUTH_TOKEN has expired — update the token and re-run sync")

// fetchWithRetry retries the given HTTP request up to maxRetries times on non-2xx
// responses or network errors. It uses exponential backoff (1s, 2s, 4s).
// It does not retry on 401 (token expired) and returns ErrTokenExpired instead.
func fetchWithRetry(client *http.Client, req *http.Request, maxRetries int) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			slog.Warn("retrying request", "attempt", attempt, "max_retries", maxRetries, "backoff", backoff, "error", lastErr)
			time.Sleep(backoff)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			slog.Error("KRL API returned 401", "error", ErrTokenExpired)
			return nil, ErrTokenExpired
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		resp.Body.Close()
		lastErr = fmt.Errorf("non-2xx status: %d", resp.StatusCode)
	}

	return nil, fmt.Errorf("all %d retries exhausted: %w", maxRetries, lastErr)
}
