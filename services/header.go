package services

import (
	"fmt"
)

func GenerateHeaders(ssoCookie string) map[string]string {
	headers := map[string]string{
		"user-agent":       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		"accept":           "*/*",
		"cache-control":    "no-cache",
		"origin":           "https://support.activision.com",
		"referer":          "https://support.activision.com/ban-appeal",
		"sec-fetch-dest":   "empty",
		"sec-fetch-mode":   "cors",
		"sec-fetch-site":   "same-site",
		"Content-Type":     "application/json",
		"x-requested-with": "XMLHttpRequest",
		"Accept-Language":  "en-US,en;q=0.9",
		"Cookie":           fmt.Sprintf("ACT_SSO_COOKIE=%s", ssoCookie),

		/*	"Cookie": [
			ACT_SSO_COOKIE=MjM5NDQzOToxNzM1MTA2MzQ4MDIyOmY0ZjJlMDA5MmJlNjUwYjdmNzhjNWI4NTk4ZDViNGRm
			ACT_SSO_COOKIE_EXPIRY=1735106348022
			ACT_SSO_EVENT="LOGOUT:1733826099604"
			ACT_SSO_LOCALE=en_US
			CookieConsentPolicy=0:1
			XSRF-TOKEN=QPRrXBUm5MUkZjNmd_WZ2qK0TOvfYEQX5iTbVE9l2hqZ7jDFFqzr-f2H_ZR8aFhg
			comid=activision
			gpv_pn=support%3Aban-appeal
			new_SiteId=activision
			pgacct=steam
			priv_reg_name=ccpa
			s_cc=true
			s_ppv=support%253Aban-appeal%2C82%2C82%2C1999
			tfa_enrollment_seen=true
		*/
	}
	return headers
}

func GeneratePostHeaders(ssoCookie string) map[string]string {
	headers := GenerateHeaders(ssoCookie)
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	headers["x-requested-with"] = "XMLHttpRequest"
	return headers
}
