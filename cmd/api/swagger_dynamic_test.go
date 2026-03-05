package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestRequestScheme(t *testing.T) {
	t.Run("uses x-forwarded-proto when present", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/docs", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		if got := requestScheme(req); got != "https" {
			t.Fatalf("scheme = %q, expected https", got)
		}
	})

	t.Run("falls back to request tls", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/docs", nil)
		if got := requestScheme(req); got != "https" {
			t.Fatalf("scheme = %q, expected https", got)
		}
	})

	t.Run("defaults to http", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/docs", nil)
		if got := requestScheme(req); got != "http" {
			t.Fatalf("scheme = %q, expected http", got)
		}
	})
}

func TestBuildDynamicSwaggerDoc(t *testing.T) {
	doc, err := buildDynamicSwaggerDoc("ignas-comu-7lhlvl-6446bd-138-2-84-251.traefik.me", "https")
	if err != nil {
		t.Fatalf("buildDynamicSwaggerDoc() error = %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(doc, &parsed); err != nil {
		t.Fatalf("json unmarshal error = %v", err)
	}

	if parsed["host"] != "ignas-comu-7lhlvl-6446bd-138-2-84-251.traefik.me" {
		t.Fatalf("host = %v", parsed["host"])
	}

	schemes, ok := parsed["schemes"].([]any)
	if !ok || len(schemes) != 1 || schemes[0] != "https" {
		t.Fatalf("schemes = %#v", parsed["schemes"])
	}
}
