// Package tokenrefresh scrapes the KCI schedule page to extract the hardcoded
// Bearer token, checks JWT expiry, and notifies the admin via Telegram.
package tokenrefresh

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// tokenRegex matches the first JWT Bearer token embedded in the KCI page JS.
var tokenRegex = regexp.MustCompile(`Bearer\s+(eyJ[A-Za-z0-9_\-\.]+)`)

const kciPageURL = "https://kci.id/perjalanan-krl/jadwal-kereta"

// FetchFromKCI fetches the KCI schedule page and extracts the hardcoded Bearer token.
// The token is embedded directly in the page's inline JavaScript.
func FetchFromKCI(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kciPageURL, nil)
	if err != nil {
		return "", fmt.Errorf("tokenrefresh: build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("tokenrefresh: fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tokenrefresh: KCI page returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("tokenrefresh: read body: %w", err)
	}

	return extractTokenFromPage(body)
}

func extractTokenFromPage(body []byte) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := scanner.Bytes()
		if bytes.Contains(line, []byte("//")) {
			continue
		}

		m := tokenRegex.FindSubmatch(line)
		if len(m) >= 2 {
			return string(m[1]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("tokenrefresh: scan page: %w", err)
	}

	return "", fmt.Errorf("tokenrefresh: Bearer token not found in KCI page — page structure may have changed")
}

// TryRefresh fetches the latest token from KCI and, if it differs from the
// current in-memory token, rotates it and notifies the Telegram admin.
// Returns true if the token was rotated.
func TryRefresh(ctx context.Context, current string, rotate func(string), botToken string, adminID int64) (bool, error) {
	fetched, err := FetchFromKCI(ctx)
	if err != nil {
		return false, err
	}

	if fetched == current {
		slog.Debug("tokenrefresh: token unchanged")
		return false, nil
	}

	slog.Info("tokenrefresh: new token detected, rotating")
	rotate(fetched)

	msg := "🔑 KAI_AUTH_TOKEN rotated automatically.\nThe new token is active. No restart needed.\n\nUpdate KAI_AUTH_TOKEN in your .env / secrets manager to persist across restarts."
	if err := NotifyAdmin(ctx, botToken, adminID, msg); err != nil {
		slog.Warn("tokenrefresh: admin notification failed", "error", err)
	}

	return true, nil
}

// ParseExpiry decodes the JWT payload (no library needed) and returns the exp time.
// Returns a zero time if the token is malformed or has no exp claim.
func ParseExpiry(token string) (time.Time, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("tokenrefresh: not a valid JWT (expected 3 parts)")
	}

	// JWT uses base64url (no padding). Add padding if needed.
	payload := parts[1]
	if pad := len(payload) % 4; pad != 0 {
		payload += strings.Repeat("=", 4-pad)
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return time.Time{}, fmt.Errorf("tokenrefresh: decode JWT payload: %w", err)
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return time.Time{}, fmt.Errorf("tokenrefresh: parse JWT claims: %w", err)
	}
	if claims.Exp == 0 {
		return time.Time{}, fmt.Errorf("tokenrefresh: JWT has no exp claim")
	}

	return time.Unix(claims.Exp, 0), nil
}

// CheckExpiry warns the admin if the token expires within warnWithin.
// Returns true if a warning was sent.
func CheckExpiry(ctx context.Context, token, botToken string, adminID int64, warnWithin time.Duration) bool {
	exp, err := ParseExpiry(token)
	if err != nil {
		slog.Warn("tokenrefresh: could not parse token expiry", "error", err)
		return false
	}

	remaining := time.Until(exp)
	slog.Debug("tokenrefresh: token expiry", "expires_at", exp.Format(time.RFC3339), "remaining", remaining.Round(time.Hour))

	if remaining > warnWithin {
		return false
	}

	days := int(remaining.Hours() / 24)
	msg := fmt.Sprintf(
		"⚠️ KAI_AUTH_TOKEN expires in %d day(s) (at %s).\nRun auto-refresh or update KAI_AUTH_TOKEN manually.",
		days, exp.UTC().Format("2006-01-02 15:04 UTC"),
	)
	if err := NotifyAdmin(ctx, botToken, adminID, msg); err != nil {
		slog.Warn("tokenrefresh: expiry notification failed", "error", err)
	}
	return true
}

// NotifyAdmin sends a plain-text Telegram message to the given chat ID.
// Silently no-ops if botToken is empty or adminID is zero.
func NotifyAdmin(ctx context.Context, botToken string, adminID int64, message string) error {
	if botToken == "" || adminID == 0 {
		return nil
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	payload := fmt.Sprintf(`{"chat_id":%d,"text":%q}`, adminID, message)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(payload))
	if err != nil {
		return fmt.Errorf("tokenrefresh: build notify request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("tokenrefresh: telegram notify: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tokenrefresh: telegram returned HTTP %d", resp.StatusCode)
	}

	slog.Info("tokenrefresh: admin notified via Telegram", "admin_id", adminID)
	return nil
}
