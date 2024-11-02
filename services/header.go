package services

import (
	"fmt"
)

func GenerateHeaders(ssoCookie string) map[string]string {
	return map[string]string{
		"user-agent":     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
		"accept":         "*/*",
		"sec-fetch-mode": "cors",
		"cookie":         fmt.Sprintf("ACT_SSO_COOKIE=%s", ssoCookie),
	}
}

func GeneratePostHeaders(ssoCookie string) map[string]string {
	headers := GenerateHeaders(ssoCookie)
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	headers["x-requested-with"] = "XMLHttpRequest"
	return headers
}
