package services

import (
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var (
	url1 = "https://support.activision.com/api/bans/v2/appeal?locale=en" // Replacement Endpoint for checking account bans
	url2 = "https://support.activision.com/api/profile?accts=false"      // Endpoint for retrieving profile information
)

var (
	httpClient = &http.Client{Timeout: 30 * time.Second}
	maxRetries = 3
)

// VerifySSOCookie checks if the provided SSO cookie is valid.
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
	resp, err := httpClient.Do(req)
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
	if len(body) == 0 { // Check if the response body is empty
		logger.Log.Error("Invalid SSOCookie, response body is empty")
		return false
	}
	logger.Log.Info("SSO cookie verified successfully")
	return true
}

// CheckAccount checks the account status associated with the provided SSO cookie.
func CheckAccount(ssoCookie string, userID string) (models.Status, error) {
	logger.Log.Info("Starting CheckAccount function")

	captchaAPIKey, err := GetUserCaptchaKey(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get user's captcha API key")
		return models.StatusUnknown, fmt.Errorf("failed to get user's captcha API key: %v", err)
	}

	gRecaptchaResponse, err := SolveReCaptchaV2WithKey(captchaAPIKey)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to solve reCAPTCHA")
		return models.StatusUnknown, fmt.Errorf("failed to solve reCAPTCHA: %v", err)
	}

	// Further processing of gRecaptchaResponse can be done here
	logger.Log.Info("Successfully solved reCAPTCHA")

	// Construct the ban appeal URL with the reCAPTCHA response
	banAppealUrl := fmt.Sprintf("%s&g-cc=%s", url1, gRecaptchaResponse)
	logger.Log.WithField("url", banAppealUrl).Info("Constructed ban appeal URL")

	var body []byte

	for attempt := 0; attempt < maxRetries; attempt++ {
		_, body, err = makeRequest("GET", banAppealUrl, ssoCookie)
		if err == nil {
			break
		}
		logger.Log.WithError(err).Errorf("Attempt %d failed to check account status", attempt+1)
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}

	if err != nil {
		return models.StatusUnknown, fmt.Errorf("failed to check account status after %d attempts: %v", maxRetries, err)
	}

	return parseAccountStatus(body)
}

func makeRequest(method, url, ssoCookie string) (*http.Response, []byte, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send HTTP request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response body: %v", err)
	}
	logger.Log.WithField("body", string(body)).Info("Read response body")

	return resp, body, nil
}

func parseAccountStatus(body []byte) (models.Status, error) {
	if len(body) == 0 {
		return models.StatusInvalidCookie, nil
	}

	var data struct {
		Error     string `json:"error"`
		Success   string `json:"success"`
		CanAppeal bool   `json:"canAppeal"`
		Bans      []struct {
			Enforcement string `json:"enforcement"`
			Title       string `json:"title"`
			CanAppeal   bool   `json:"canAppeal"`
			Bar         struct {
				CaseNumber string `json:"CaseNumber"`
				Status     string `json:"Status"`
			} `json:"bar"`
		} `json:"bans"`
	}

	err := json.Unmarshal(body, &data)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to decode JSON response")
		return models.StatusUnknown, fmt.Errorf("failed to decode JSON response: %v", err)
	}

	if data.Error != "" {
		return models.StatusUnknown, fmt.Errorf("API error: %s", data.Error)
	}

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

		if ban.Bar.Status == "Open" {
			logger.Log.Info("Account under investigation")
			return models.StatusUnderInvestigation, nil
		}

		if ban.Bar.Status == "Closed" {
			logger.Log.Info("Ban final")
			return models.StatusBanFinal, nil
		}
	}

	logger.Log.Info("Unknown account status")
	return models.StatusUnknown, nil
}

// CheckAccountAge retrieves the age of the account associated with the provided SSO cookie.
func CheckAccountAge(ssoCookie string) (int, int, int, error) {
	logger.Log.Info("Starting CheckAccountAge function")
	req, err := http.NewRequest("GET", url2, nil)
	if err != nil {
		return 0, 0, 0, errors.New("failed to create HTTP request to check account age")
	}
	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, 0, 0, errors.New("failed to send HTTP request to check account age")
	}
	defer resp.Body.Close()

	var data struct {
		Created string `json:"created"`
	}
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return 0, 0, 0, errors.New("failed to decode JSON response from check account age request")
	}

	logger.Log.Infof("Account created date: %s", data.Created)

	created, err := time.Parse(time.RFC3339, data.Created)
	if err != nil {
		return 0, 0, 0, errors.New("failed to parse created date in check account age request")
	}

	now := time.Now()
	age := now.Sub(created)

	years := int(age.Hours() / 24 / 365.25)
	months := int(age.Hours()/24/30.44) % 12
	days := int(age.Hours()/24) % 30

	logger.Log.Infof("Account age calculated: %d years, %d months, %d days", years, months, days)
	return years, months, days, nil
}
