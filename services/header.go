package services

import (
	"fmt"
)

// GenerateHeaders generates a map of headers for HTTP requests.
// It includes the SSO cookie for authentication.
func GenerateHeaders(ssoCookie string) map[string]string {
	return map[string]string{
		"user-agent":     "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0",
		"accept":         "*/*",
		"sec-fetch-mode": "cors",
		"cookie":         fmt.Sprintf("ACT_SSO_COOKIE=%s", ssoCookie),
	}
}

// GeneratePostHeaders generates a map of headers for HTTP POST requests.
// It includes the SSO cookie, Content-Type and x-requested-with headers.
func GeneratePostHeaders(ssoCookie string) map[string]string {
	headers := GenerateHeaders(ssoCookie)
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	headers["x-requested-with"] = "XMLHttpRequest"
	return headers
}
