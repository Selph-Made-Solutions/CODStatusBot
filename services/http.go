package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
)

var (
	checkURL         = os.Getenv("CHECK_ENDPOINT")     // account status check endpoint.
	profileURL       = os.Getenv("PROFILE_ENDPOINT")   // Endpoint for retrieving profile information
	checkVIP         = os.Getenv("CHECK_VIP_ENDPOINT") // Endpoint for checking VIP status
	recaptchaSiteKey = os.Getenv("RECAPTCHA_SITE_KEY") // Site key for reCAPTCHA
	recaptchaURL     = os.Getenv("RECAPTCHA_URL")      // URL for reCAPTCHA
)

type AccountValidationResult struct {
	IsValid     bool
	Created     int64
	IsVIP       bool
	ExpiresAt   int64
	ProfileData map[string]interface{}
}

func VerifySSOCookie(ssoCookie string) bool {
	logger.Log.Infof("Verifying SSO cookie: %s", ssoCookie)
	req, err := http.NewRequest("GET", profileURL, nil)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating verification request")
		return false
	}
	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{
		Timeout: 60 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		logger.Log.WithError(err).Error("Error sending verification request")
		return false
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Log.WithError(err).Error("Error closing response body")
		}
	}(resp.Body)

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

func CheckAccount(ssoCookie string, userID string, captchaAPIKey string) (models.Status, error) {
	// Lock database operations for operation
	DBMutex.Lock()
	defer DBMutex.Unlock()
	logger.Log.Info("Starting CheckAccount function")

	// Validate user exists and has permission to check accounts
	userSettings, err := GetUserSettings(userID)
	if err != nil {
		logger.Log.WithError(err).WithField("userID", userID).Error("Failed to get user settings")
		if isCriticalError(err) {
			logger.Log.WithError(err).Error("Critical error getting user settings")
			return models.StatusUnknown, fmt.Errorf("critical error: %w", err)
		}
		return models.StatusUnknown, fmt.Errorf("failed to get user settings: %w", err)
	}

	// Validate captcha service availability
	if !IsServiceEnabled("ezcaptcha") && !IsServiceEnabled("2captcha") {
		return models.StatusUnknown, fmt.Errorf("no captcha services are currently enabled")
	}

	// Handle captcha provider fallback
	if !IsServiceEnabled(userSettings.PreferredCaptchaProvider) {
		if IsServiceEnabled("ezcaptcha") {
			// userSettings.PreferredCaptchaProvider = ezcap
			userSettings.PreferredCaptchaProvider = "ezcaptcha"
			database.DB.Save(&userSettings)
		} else if IsServiceEnabled("2captcha") {
			// userSettings.PreferredCaptchaProvider = twocap
			userSettings.PreferredCaptchaProvider = "2captcha"
			database.DB.Save(&userSettings)
		} else {
			return models.StatusUnknown, fmt.Errorf("no captcha services are currently enabled")
		}
	}

	// Verify user hasn't exceeded rate limit
	if !checkActionRateLimit(userID, fmt.Sprintf("check_account_%s", userID), time.Hour) { // Experimental
		return models.StatusUnknown, fmt.Errorf("rate limit exceeded for user %s", userID) // Experimental
	} // Experimental

	// Get captcha solver with retry logic
	solver, err := GetCaptchaSolver(userID)
	if err != nil {
		if isCriticalError(err) {
			if strings.Contains(err.Error(), "insufficient balance") {
				if err := DisableUserCaptcha(nil, userID, "Insufficient balance"); err != nil {
					logger.Log.WithError(err).Error("Failed to disable user captcha service")
				}
			}
			return models.StatusUnknown, fmt.Errorf("critical error: %w", err)
		}
		return models.StatusUnknown, fmt.Errorf("failed to create captcha solver: %w", err)
	}

	// Solve reCAPTCHA with retry logic
	var gRecaptchaResponse string
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		gRecaptchaResponse, err = solver.SolveReCaptchaV2(recaptchaSiteKey, recaptchaURL)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "insufficient balance") {
			if err := DisableUserCaptcha(nil, userID, "Insufficient balance"); err != nil {
				logger.Log.WithError(err).Error("Failed to disable user captcha service")
			}
			return models.StatusUnknown, fmt.Errorf("insufficient captcha balance")
		}
		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * time.Second)
		}
	}
	if err != nil {
		return models.StatusUnknown, fmt.Errorf("failed to solve reCAPTCHA after %d attempts: %w", maxRetries, err)
	}

	logger.Log.Info("Successfully received reCAPTCHA response")

	// Construct request URL
	checkRequest := fmt.Sprintf("%s?locale=en&g-cc=%s", checkURL, gRecaptchaResponse)
	logger.Log.WithField("url", checkRequest).Info("Constructed account check request")

	// Create request headers
	req, err := http.NewRequest("GET", checkRequest, nil)
	if err != nil {
		return models.StatusUnknown, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add all required headers
	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	logger.Log.WithField("headers", headers).Info("Set request headers")

	// Configure client with timeouts and settings
	client := &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{ // Experimental
			TLSHandshakeTimeout:   10 * time.Second, // Experimental
			ResponseHeaderTimeout: 30 * time.Second, // Experimental
			ExpectContinueTimeout: 1 * time.Second,  // Experimental
			MaxIdleConnsPerHost:   10,               // Experimental
			DisableCompression:    false,            // Experimental
		},
	}

	// Request with retry logic
	var resp *http.Response
	var body []byte
	maxAPIRetries := 3
	var lastError error

	for i := 0; i < maxAPIRetries; i++ {
		logger.Log.Infof("Sending HTTP request to check account (attempt %d/%d)", i+1, maxAPIRetries)
		resp, err = client.Do(req)
		if err != nil {
			lastError = err
			if i < maxAPIRetries-1 {
				time.Sleep(time.Duration(i+1) * time.Second)
				continue
			}
			return models.StatusUnknown, fmt.Errorf("failed to send HTTP request after %d attempts: %w", maxAPIRetries, lastError)
		}
		defer func(Body io.ReadCloser) {
			if err := Body.Close(); err != nil {
				logger.Log.WithError(err).Error("Failed to close response body")
			}
		}(resp.Body)

		logger.Log.WithField("status", resp.Status).Info("Received response")

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			lastError = err
			if i < maxAPIRetries-1 {
				time.Sleep(time.Duration(i+1) * time.Second)
				continue
			}
			return models.StatusUnknown, fmt.Errorf("failed to read response body after %d attempts: %w", maxAPIRetries, lastError)
		}
		break
	}

	logger.Log.WithField("body", string(body)).Info("Read response body")

	// Handle error responses
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
			return models.StatusUnknown, fmt.Errorf("invalid request to endpoint: %s", errorResponse.Error)
		}
	}

	// Handle empty response
	if string(body) == "" {
		return models.StatusInvalidCookie, nil
	}

	// Parse response data
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

	if err := json.Unmarshal(body, &data); err != nil {
		return models.StatusUnknown, fmt.Errorf("failed to decode JSON response: %w", err)
	}
	logger.Log.WithField("data", data).Info("Parsed ban data")

	// Update captcha usage tracking
	if err := UpdateCaptchaUsage(userID); err != nil {
		logger.Log.WithError(err).Error("Failed to update captcha usage")
	}

	// Determine account status
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

func UpdateCaptchaUsage(userID string) error {
	settings, err := GetUserSettings(userID)
	if err != nil {
		return err
	}

	if settings.EZCaptchaAPIKey == "" && settings.TwoCaptchaAPIKey == "" {
		return nil
	}

	apiKey := settings.EZCaptchaAPIKey
	provider := "ezcaptcha"
	if settings.PreferredCaptchaProvider == "2captcha" {
		apiKey = settings.TwoCaptchaAPIKey
		provider = "2captcha"
	}

	isValid, balance, err := ValidateCaptchaKey(apiKey, provider)
	if err != nil {
		return err
	}

	if !isValid {
		return errors.New("invalid captcha key")
	}

	settings.CaptchaBalance = balance
	settings.LastBalanceCheck = time.Now()

	return database.DB.Save(&settings).Error
}

func CheckAccountAge(ssoCookie string) (int, int, int, int64, error) {
	logger.Log.Info("Starting CheckAccountAge function")
	req, err := http.NewRequest("GET", profileURL, nil)
	if err != nil {
		return 0, 0, 0, 0, errors.New("failed to create HTTP request to check account age")
	}
	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{
		Timeout: 60 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, 0, 0, errors.New("failed to send HTTP request to check account age")
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Log.WithError(err).Error("Failed to close response body")
		}
	}(resp.Body)

	var data struct {
		Created string `json:"created"`
	}
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return 0, 0, 0, 0, errors.New("failed to decode JSON response from check account age request")
	}

	logger.Log.Infof("Account created date: %s", data.Created)

	created, err := time.Parse(time.RFC3339, data.Created)
	if err != nil {
		return 0, 0, 0, 0, errors.New("failed to parse created date in check account age request")
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

func CheckVIPStatus(ssoCookie string) (bool, error) {
	logger.Log.Info("Checking VIP status")
	req, err := http.NewRequest("GET", checkVIP+ssoCookie, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create HTTP request to check VIP status: %w", err)
	}
	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to send HTTP request to check VIP status: %w", err)
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Log.WithError(err).Error("Failed to close response body")
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("invalid response status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response body: %w", err)
	}

	var data struct {
		VIP bool `json:"vip"`
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		return false, fmt.Errorf("failed to decode JSON response: %w", err)
	}

	logger.Log.Infof("VIP status check complete. Result: %v", data.VIP)
	return data.VIP, nil
}

/*
func RedeemCode(ssoCookie, code string) (string, error) {
	logger.Log.Infof("Attempting to redeem code: %s", code)

	req, err := http.NewRequest("POST", redeemCodeURL, strings.NewReader(fmt.Sprintf("code=%s", code)))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request to redeem code: %w", err)
	}

	headers := GeneratePostHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send HTTP request to redeem code: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Log.Errorf("Failed to close response body: %v", err)
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var jsonResponse struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &jsonResponse); err == nil && jsonResponse.Status == "success" {
		logger.Log.Infof("Code redemption successful: %s", jsonResponse.Message)
		return jsonResponse.Message, nil
	}

	bodyStr := string(body)
	if strings.Contains(bodyStr, "redemption-success") {
		start := strings.Index(bodyStr, "Just Unlocked:<br><br><div class=\"accent-highlight mw2\">")
		end := strings.Index(bodyStr, "</div></h4>")
		if start != -1 && end != -1 {
			unlockedItem := strings.TrimSpace(bodyStr[start+len("Just Unlocked:<br><br><div class=\"accent-highlight mw2\">") : end])
			logger.Log.Infof("Successfully claimed reward: %s", unlockedItem)
			return fmt.Sprintf("Successfully claimed reward: %s", unlockedItem), nil
		}
		logger.Log.Info("Successfully claimed reward, but couldn't extract details")
		return "Successfully claimed reward, but couldn't extract details", nil
	}

	logger.Log.Warnf("Unexpected response body: %s", bodyStr)
	return "", fmt.Errorf("failed to redeem code: unexpected response")
}
*/

func ValidateAndGetAccountInfo(ssoCookie string) (*AccountValidationResult, error) {
	// Create request with timeouts
	req, err := http.NewRequest("GET", profileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create profile request: %w", err)
	}

	// Add all headers
	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// client with timeouts
	client := &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{ // Experimental
			TLSHandshakeTimeout:   10 * time.Second, // Experimental
			ResponseHeaderTimeout: 30 * time.Second, // Experimental
			ExpectContinueTimeout: 1 * time.Second,  // Experimental
			MaxIdleConnsPerHost:   10,               // Experimental
			DisableCompression:    false,            // Experimental
			IdleConnTimeout:       90 * time.Second, // Experimental
			DisableKeepAlives:     false,            // Experimental
		}, // Experimental
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send profile request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			logger.Log.WithError(err).Error("Failed to close response body")
		}
	}(resp.Body)

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return &AccountValidationResult{IsValid: false}, nil
	}

	// Parse data with validation
	var profileData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&profileData); err != nil {
		return nil, fmt.Errorf("failed to decode profile response: %w", err)
	}

	// Parse creation date
	createdStr, ok := profileData["created"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid creation date format")
	}

	created, err := time.Parse(time.RFC3339, createdStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse creation date: %w", err)
	}

	// Check VIP status
	isVIP, err := CheckVIPStatus(ssoCookie)
	if err != nil {
		logger.Log.WithError(err).Warn("Failed to check VIP status, defaulting to false")
		isVIP = false
	}

	// Decode cookie for expiration
	expirationTimestamp, err := DecodeSSOCookie(ssoCookie)
	if err != nil {
		return nil, fmt.Errorf("failed to decode SSO cookie: %w", err)
	}

	// Check for expired cookie
	if time.Now().Unix() > expirationTimestamp {
		return &AccountValidationResult{IsValid: false}, nil
	}

	return &AccountValidationResult{
		IsValid:     true,
		Created:     created.Unix(),
		IsVIP:       isVIP,
		ExpiresAt:   expirationTimestamp,
		ProfileData: profileData,
	}, nil
}
