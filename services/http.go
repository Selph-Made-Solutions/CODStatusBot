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

var url1 = "https://support.activision.com/api/bans/v2/appeal?locale=en" // Replacement Endpoint for checking account bans
var url2 = "https://support.activision.com/api/profile?accts=false"      // Endpoint for retrieving profile information

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
	if len(body) == 0 { // Check if the response body is empty
		logger.Log.Error("Invalid SSOCookie, response body is empty")
		return false
	}
	logger.Log.Info("SSO cookie verified successfully")
	return true
}

// CheckAccount checks the account status associated with the provided SSO cookie.
func CheckAccount(ssoCookie string, captchaAPIKey string) (models.Status, error) {
	logger.Log.Info("Starting CheckAccount function")

	var gRecaptchaResponse string
	var err error

	if captchaAPIKey != "" {
		// Use user's API key
		gRecaptchaResponse, err = SolveReCaptchaV2WithKey(captchaAPIKey)
		if err != nil {
			logger.Log.WithError(err).Error("Failed to solve reCAPTCHA with user's API key")
			return models.StatusUnknown, fmt.Errorf("invalid API key or reCAPTCHA solving failed: %v", err)
		}
	} else {
		gRecaptchaResponse, err = SolveReCaptchaV2()
		if err != nil {
			logger.Log.WithError(err).Error("Failed to solve reCAPTCHA with default key")
			return models.StatusUnknown, fmt.Errorf("failed to solve reCAPTCHA: %v", err)
		}
	}

	logger.Log.Info("Successfully solved reCAPTCHA")

	// Construct the ban appeal URL with the reCAPTCHA response
	banAppealUrl := url1 + "&g-cc=" + gRecaptchaResponse
	logger.Log.WithField("url", banAppealUrl).Info("Constructed ban appeal URL")

	req, err := http.NewRequest("GET", banAppealUrl, nil)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create HTTP request")
		return models.StatusUnknown, errors.New("failed to create HTTP request to check account")
	}

	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	logger.Log.WithField("headers", headers).Info("Set request headers")

	client := &http.Client{}
	logger.Log.Info("Sending HTTP request to check account")
	resp, err := client.Do(req)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send HTTP request")
		return models.StatusUnknown, errors.New("failed to send HTTP request to check account")
	}
	defer resp.Body.Close()

	logger.Log.WithField("status", resp.Status).Info("Received response")

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to read response body")
		return models.StatusUnknown, errors.New("failed to read response body from check account request")
	}
	logger.Log.WithField("body", string(body)).Info("Read response body")

	// Check for specific error responses
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
		switch {
		case errorResponse.Status == 404 && errorResponse.Path == "/api/bans/appeal":
			logger.Log.Error("Old endpoint no longer available")
			return models.StatusUnknown, errors.New("old endpoint no longer available")
		case errorResponse.Status == 400 && errorResponse.Path == "/api/bans/v2/appeal":
			logger.Log.Error("Invalid request to new endpoint, possibly missing or invalid reCAPTCHA")
			return models.StatusUnknown, errors.New("invalid request to new endpoint, possibly missing or invalid reCAPTCHA")
		}
	}

	// If not an error response, proceed with parsing the actual ban data
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
		logger.Log.WithError(err).Error("Failed to decode JSON response")
		return models.StatusUnknown, fmt.Errorf("failed to decode JSON response: %v", err)
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
	client := &http.Client{}
	resp, err := client.Do(req)
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

	created, err := time.Parse(time.RFC3339, data.Created)
	if err != nil {
		return 0, 0, 0, errors.New("failed to parse created date in check account age request")
	}

	duration := time.Since(created)
	years := int(duration.Hours() / 24 / 365)
	months := int(duration.Hours()/24/30) % 12
	days := int(duration.Hours()/24) % 365 % 30
	return years, months, days, nil
}
