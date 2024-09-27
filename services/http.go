package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"CODStatusBot/errorhandler"
	"CODStatusBot/logger"
	"CODStatusBot/models"
)

var (
	url1 = "https://support.activision.com/api/bans/v2/appeal?locale=en"
	url2 = "https://support.activision.com/api/profile?accts=false"
)

func VerifySSOCookie(ssoCookie string) bool {
	logger.Log.Infof("Verifying SSO cookie: %s", ssoCookie)
	req, err := http.NewRequest("GET", url2, nil)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating verification request")
		return false
	}
	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Log.WithError(err).Error("Error sending verification request")
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logger.Log.Errorf("Invalid SSOCookie, status code: %d", resp.StatusCode)
		return false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Log.WithError(err).Error("Error reading verification response body")
		return false
	}
	if len(body) == 0 {
		logger.Log.Error("Invalid SSOCookie, response body is empty")
		return false
	}
	logger.Log.Info("SSO cookie verified successfully")
	return true
}

func CheckAccount(ssoCookie string, userID string) (models.Status, error) {
	logger.Log.Info("Starting CheckAccount function")

	captchaAPIKey, _, err := GetUserCaptchaKey(userID)
	if err != nil {
		return models.StatusUnknown, errorhandler.NewDatabaseError(err, "fetching user's captcha API key")
	}

	gRecaptchaResponse, err := SolveReCaptchaV2WithKey(captchaAPIKey)
	if err != nil {
		return models.StatusUnknown, errorhandler.NewAPIError(err, "EZ-Captcha")
	}

	logger.Log.Info("Successfully solved reCAPTCHA")

	banAppealUrl := url1 + "&g-cc=" + gRecaptchaResponse
	logger.Log.WithField("url", banAppealUrl).Info("Constructed ban appeal URL")

	req, err := http.NewRequest("GET", banAppealUrl, nil)
	if err != nil {
		return models.StatusUnknown, errorhandler.NewNetworkError(err, "creating HTTP request")
	}

	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	logger.Log.WithField("headers", headers).Info("Set request headers")

	client := &http.Client{
		Timeout: 45 * time.Second,
	}

	var resp *http.Response
	var body []byte
	maxRetries := 1
	for i := 0; i < maxRetries; i++ {
		logger.Log.Infof("Sending HTTP request to check account (attempt %d/%d)", i+1, maxRetries)
		resp, err = client.Do(req)
		if err != nil {
			if i == maxRetries-1 {
				return models.StatusUnknown, errorhandler.NewNetworkError(err, "sending HTTP request")
			}
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		defer resp.Body.Close()

		logger.Log.WithField("status", resp.Status).Info("Received response")

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			if i == maxRetries-1 {
				return models.StatusUnknown, errorhandler.NewNetworkError(err, "reading response body")
			}
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		break
	}

	logger.Log.WithField("body", string(body)).Info("Read response body")

	var errorResponse struct {
		Timestamp string `json:"timestamp"`
		Path      string `json:"path"`
		Status    int    `json:"status"`
		Error     string `json:"error"`
		RequestId string `json:"requestId"`
		Exception string `json:"exception"`
	}

	if err := json.Unmarshal(body, &errorResponse); err == nil {
		logger.Log.WithField("errorResponse", errorResponse).Info("Parsed error response")
		if errorResponse.Status == 400 && errorResponse.Path == "/api/bans/v2/appeal" {
			return models.StatusUnknown, errorhandler.NewAPIError(fmt.Errorf("invalid request to new endpoint"), "Activision API")
		}
	}

	var data struct {
		Error     string `json:"error"`
		Success   string `json:"success"`
		CanAppeal bool   `json:"canAppeal"`
		Bans      []struct {
			Enforcement string `json:"enforcement"`
			Title       string `json:"title"`
			CanAppeal   bool   `json:"canAppeal"`
		} `json:"bans"`
	}

	if string(body) == "" {
		logger.Log.Info("Empty response body, treating as invalid cookie")
		return models.StatusInvalidCookie, nil
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		return models.StatusUnknown, errorhandler.NewAPIError(err, "decoding JSON response")
	}
	logger.Log.WithField("data", data).Info("Parsed ban data")

	if len(data.Bans) == 0 {
		logger.Log.Info("No bans found, account status is good")
		return models.StatusGood, nil
	}

	for _, ban := range data.Bans {
		logger.Log.WithField("ban", ban).Info("Processing ban")
		switch ban.Enforcement {
		case "PERMANENT":
			logger.Log.Info("Permanent ban detected")
			return models.StatusPermaban, nil
		case "UNDER_REVIEW":
			logger.Log.Info("Shadowban detected")
			return models.StatusShadowban, nil
		case "TEMPORARY":
			logger.Log.Info("Temporary ban detected")
			return models.StatusTempban, nil
		}
	}

	logger.Log.Info("Unknown account status")
	return models.StatusUnknown, nil
}

func CheckAccountAge(ssoCookie string) (int, int, int, int64, error) {
	logger.Log.Info("Starting CheckAccountAge function")
	req, err := http.NewRequest("GET", url2, nil)
	if err != nil {
		return 0, 0, 0, 0, errorhandler.NewNetworkError(err, "creating HTTP request")
	}
	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, 0, 0, errorhandler.NewNetworkError(err, "sending HTTP request")
	}
	defer resp.Body.Close()

	var data struct {
		Created string `json:"created"`
	}
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return 0, 0, 0, 0, errorhandler.NewAPIError(err, "decoding JSON response")
	}

	logger.Log.Infof("Account created date: %s", data.Created)

	created, err := time.Parse(time.RFC3339, data.Created)
	if err != nil {
		return 0, 0, 0, 0, errorhandler.NewValidationError(err, "parsing created date")
	}

	createdUTC := created.UTC()
	createdEpoch := createdUTC.Unix()

	now := time.Now().UTC()
	age := now.Sub(createdUTC)
	years := int(age.Hours() / 24 / 365.25)
	months := int(age.Hours()/24/30.44) % 12
	days := int(age.Hours()/24) % 30

	logger.Log.Infof("Account age calculated: %d years, %d months, %d days", years, months, days)
	return years, months, days, createdEpoch, nil
}
