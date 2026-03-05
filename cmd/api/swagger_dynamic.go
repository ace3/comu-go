package main

import (
	"net/http"
	"strings"

	"github.com/comu/api/docs"
)

func requestScheme(r *http.Request) string {
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		for _, part := range parts {
			scheme := strings.ToLower(strings.TrimSpace(part))
			if scheme == "http" || scheme == "https" {
				return scheme
			}
		}
	}

	if r.TLS != nil {
		return "https"
	}

	return "http"
}

func buildDynamicSwaggerDoc(host, scheme string) ([]byte, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		host = docs.SwaggerInfo.Host
	}

	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme != "http" && scheme != "https" {
		scheme = "http"
	}

	spec := *docs.SwaggerInfo
	spec.Host = host
	spec.Schemes = []string{scheme}
	return []byte(spec.ReadDoc()), nil
}
