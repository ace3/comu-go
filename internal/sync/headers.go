package sync

import (
	"net/http"
	"strings"
)

// setKRLHeaders applies the headers required by the KRL partner API.
// kciHost is the base URL (e.g. "https://kci.id"); defaults to "https://kci.id" if empty.
// If customHeaderKey is non-empty, an additional header is set with customHeaderValue.
func setKRLHeaders(req *http.Request, token, kciHost, customHeaderKey, customHeaderValue string) {
	if kciHost == "" {
		kciHost = "https://kci.id"
	}
	kciHost = strings.TrimRight(kciHost, "/")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Origin", kciHost)
	req.Header.Set("Referer", kciHost+"/")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1")
	req.Header.Set("sec-ch-ua", `"Not:A-Brand";v="99", "Google Chrome";v="145", "Chromium";v="145"`)
	req.Header.Set("sec-ch-ua-mobile", "?1")
	req.Header.Set("sec-ch-ua-platform", `"iOS"`)
	if customHeaderKey != "" {
		req.Header.Set(customHeaderKey, customHeaderValue)
	}
}
