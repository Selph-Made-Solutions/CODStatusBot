package services

import (
	"fmt"
	"time"
)

func GenerateHeaders(ssoCookie string) map[string]string {
	headers := map[string]string{
		"user-agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		"accept":             "*/*",
		"accept-language":    "en-US,en;q=0.9",
		"cache-control":      "no-cache",
		"pragma":             "no-cache",
		"sec-ch-ua":          "\"Chromium\";v=\"130\", \"Google Chrome\";v=\"130\", \"Not?A_Brand\";v=\"99\"",
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": "\"Windows\"",
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "same-origin",
		"x-requested-with":   "XMLHttpRequest",
		"Cookie": fmt.Sprintf("ACT_SSO_COOKIE=%s; ACT_SSO_REMEMBER_ME=%s; ACT_SSO_EVENT=\"LOGIN_SUCCESS:%d\"; POAct-ACTXSRF=active",
			ssoCookie,
			ssoCookie,
			time.Now().UnixMilli()),
	}
	return headers
}
