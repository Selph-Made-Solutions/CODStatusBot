package services

import (
	"fmt"
)

// GenerateHeaders generates a map of headers for HTTP requests.
// It includes the SSO cookie for authentication.
func GenerateHeaders(ssoCookie string) map[string]string {
	return map[string]string{
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
