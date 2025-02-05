package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/configuration"
	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/sirupsen/logrus"
)

const (
	capsol = "capsolver"
	ezcap  = "ezcaptcha"
	twocap = "2captcha"
)

type AccountValidationResult struct {
	IsValid     bool
	Created     int64
	IsVIP       bool
	ExpiresAt   int64
	ProfileData map[string]interface{}
}

func init() {
	cfg := configuration.Get()
	logger.Log.Infof("Initialized endpoints: Profile URL: %s", cfg.API.ProfileEndpoint)
}

func VerifySSOCookie(ssoCookie string) bool {
	cfg := configuration.Get()
	logger.Log.Infof("Starting SSO cookie verification for cookie: %s", ssoCookie)

	profileURL := cfg.API.ProfileEndpoint
	if profileURL == "" {
		logger.Log.Error("PROFILE_ENDPOINT not configured")
		return false
	}

	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	maxRetries := 3
	var lastError error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("GET", profileURL, nil)
		if err != nil {
			lastError = fmt.Errorf("error creating verification request (attempt %d/%d): %w", attempt, maxRetries, err)
			logger.Log.WithError(err).Error("Error creating verification request")
			continue
		}

		headers := GenerateHeaders(ssoCookie)
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		logger.Log.Infof("Sending verification request to: %s (attempt %d/%d)", profileURL, attempt, maxRetries)
		resp, err := client.Do(req)
		if err != nil {
			lastError = fmt.Errorf("error sending verification request (attempt %d/%d): %w", attempt, maxRetries, err)
			logger.Log.WithError(err).Error("Error sending verification request")
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		if resp != nil && resp.Body != nil {
			defer func(Body io.ReadCloser) {
				err := Body.Close()
				if err != nil {
					logger.Log.WithError(err).Error("Failed to close response body")
				}
			}(resp.Body)
		}

		if resp.StatusCode != http.StatusOK {
			lastError = fmt.Errorf("invalid status code (attempt %d/%d): %d", attempt, maxRetries, resp.StatusCode)
			logger.Log.WithFields(logrus.Fields{
				"statusCode": resp.StatusCode,
				"attempt":    attempt,
			}).Error("Invalid status code in SSO verification")
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastError = fmt.Errorf("error reading response body (attempt %d/%d): %w", attempt, maxRetries, err)
			logger.Log.WithError(err).Error("Error reading response body")
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		if len(body) == 0 {
			lastError = fmt.Errorf("empty response body (attempt %d/%d)", attempt, maxRetries)
			logger.Log.Error("Empty response body in SSO verification")
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		logger.Log.Info("SSO cookie verified successfully")
		return true
	}

	logger.Log.WithError(lastError).Error("SSO cookie verification failed after all retries")
	return false
}

func CheckAccount(ssoCookie string, userID string, captchaAPIKey string) (models.Status, error) {
	cfg := configuration.Get()
	logger.Log.Info("Starting CheckAccount function")

	userSettings, err := GetUserSettings(userID)
	if err != nil {
		if isCriticalError(err) {
			logger.Log.WithError(err).Error("Critical error getting user settings")
			return models.StatusUnknown, fmt.Errorf("critical error: %w", err)
		}
		return models.StatusUnknown, fmt.Errorf("failed to get user settings: %w", err)
	}

	if !VerifySSOCookie(ssoCookie) {
		return models.StatusInvalidCookie, nil
	}

	if !IsServiceEnabled("ezcaptcha") &&
		!IsServiceEnabled("capsolver") &&
		!IsServiceEnabled("2captcha") {
		return models.StatusUnknown, fmt.Errorf("no captcha services are currently enabled")
	}

	if !IsServiceEnabled(userSettings.PreferredCaptchaProvider) {
		if IsServiceEnabled("capsolver") {
			userSettings.PreferredCaptchaProvider = capsol
			database.DB.Save(&userSettings)
		} else if IsServiceEnabled("ezcaptcha") {
			userSettings.PreferredCaptchaProvider = ezcap
			database.DB.Save(&userSettings)
		} else if IsServiceEnabled("2captcha") {
			userSettings.PreferredCaptchaProvider = twocap
			database.DB.Save(&userSettings)
		} else {
			return models.StatusUnknown, fmt.Errorf("no captcha services are currently enabled")
		}
	}

	solver, err := GetCaptchaSolver(userID)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient balance") {
			if err := DisableUserCaptcha(nil, userID, "Insufficient balance"); err != nil {
				logger.Log.WithError(err).Error("Failed to disable user captcha service")
			}
			return models.StatusUnknown, fmt.Errorf("critical error: %w", err)
		}
		return models.StatusUnknown, fmt.Errorf("failed to create captcha solver: %w", err)
	}

	gRecaptchaResponse, err := solver.SolveReCaptchaV2(cfg.CaptchaService.RecaptchaSiteKey, cfg.CaptchaService.RecaptchaURL)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient balance") {
			if err := DisableUserCaptcha(nil, userID, "Insufficient balance"); err != nil {
				logger.Log.WithError(err).Error("Failed to disable user captcha service")
			}
			return models.StatusUnknown, fmt.Errorf("insufficient captcha balance")
		}
		return models.StatusUnknown, fmt.Errorf("failed to solve reCAPTCHA: %w", err)
	}

	if strings.Contains(gRecaptchaResponse, "Invalid") || len(gRecaptchaResponse) < 50 {
		return models.StatusUnknown, fmt.Errorf("invalid captcha response received")
	}

	logger.Log.Info("Successfully received reCAPTCHA response")

	checkRequest := fmt.Sprintf("%s?locale=en&g-cc=%s", cfg.API.CheckEndpoint, gRecaptchaResponse)
	logger.Log.WithField("url", checkRequest).Info("Constructed account check request")

	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	req, err := http.NewRequest("GET", checkRequest, nil)
	if err != nil {
		return models.StatusUnknown, fmt.Errorf("failed to create request: %w", err)
	}

	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	logger.Log.WithFields(logrus.Fields{
		"url":          checkRequest,
		"headers":      headers,
		"cookieLength": len(ssoCookie),
	}).Debug("Set request headers")
	var resp *http.Response
	var body []byte
	maxRetries := 3

	backoffDuration := time.Second
	for i := 0; i < maxRetries; i++ {
		logger.Log.Infof("Sending request to check account (attempt %d/%d)", i+1, maxRetries)
		resp, err = client.Do(req)
		if err != nil {
			if i == maxRetries-1 {
				return models.StatusUnknown, fmt.Errorf("failed to send request after %d attempts: %w", maxRetries, err)
			}
			backoffDuration *= 2
			time.Sleep(backoffDuration)
			continue
		}

		body, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			if i == maxRetries-1 {
				return models.StatusUnknown, fmt.Errorf("failed to read response body after %d attempts: %w", maxRetries, err)
			}
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		logger.Log.WithFields(logrus.Fields{
			"statusCode":  resp.StatusCode,
			"bodyLength":  len(body),
			"bodyContent": string(body),
		}).Info("Received API response")

		break
	}

	logger.Log.WithField("body", string(body)).Info("Read response body")

	if resp.ContentLength == 0 {
		logger.Log.Warn("Received empty response (Content-Length: 0)")
	}

	if len(body) == 0 {
		logger.Log.Warn("Empty response body after reading")
	}

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

	var data struct {
		Error     string `json:"error"`
		Success   string `json:"success"`
		CanAppeal bool   `json:"canAppeal"`
		Bans      []struct {
			Enforcement string   `json:"enforcement"`
			Title       string   `json:"title"`
			CanAppeal   bool     `json:"canAppeal"`
			Bar         struct{} `json:"bar,omitempty"`
		} `json:"bans"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return models.StatusUnknown, fmt.Errorf("failed to parse response: %w", err)
	}
	logger.Log.WithField("data", data).Info("Parsed ban data")

	if data.Success == "true" && len(data.Bans) == 0 {
		logger.Log.Info("No bans found, account status is good")
		return models.StatusGood, nil
	}

	if err := UpdateCaptchaUsage(userID); err != nil {
		logger.Log.WithError(err).Error("Failed to update captcha usage")
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

// TODO: update with new capsolver details

func UpdateCaptchaUsage(userID string) error {
	settings, err := GetUserSettings(userID)
	if err != nil {
		return err
	}

	if settings.EZCaptchaAPIKey == "" &&
		settings.TwoCaptchaAPIKey == "" &&
		settings.CapSolverAPIKey == "" {
		return nil
	}

	var apiKey string
	var provider string

	switch settings.PreferredCaptchaProvider {
	case "ezcaptcha":
		apiKey = settings.EZCaptchaAPIKey
		provider = "ezcaptcha"
	case "2captcha":
		apiKey = settings.TwoCaptchaAPIKey
		provider = "2captcha"
	default:
		apiKey = settings.CapSolverAPIKey
		provider = "capsolver"
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
	cfg := configuration.Get()
	req, err := http.NewRequest("GET", cfg.API.ProfileEndpoint, nil)
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
	cfg := configuration.Get()
	logger.Log.Info("Checking VIP status")
	req, err := http.NewRequest("GET", cfg.API.CheckVIPEndpoint+ssoCookie, nil)
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

func ValidateAndGetAccountInfo(ssoCookie string) (*AccountValidationResult, error) {
	cfg := configuration.Get()
	req, err := http.NewRequest("GET", cfg.API.ProfileEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create profile request: %w", err)
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
		return nil, fmt.Errorf("failed to send profile request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Log.WithError(err).Error("Failed to close response body")
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return &AccountValidationResult{IsValid: false}, nil
	}

	var profileData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&profileData); err != nil {
		return nil, fmt.Errorf("failed to decode profile response: %w", err)
	}

	createdStr, ok := profileData["created"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid creation date format")
	}

	created, err := time.Parse(time.RFC3339, createdStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse creation date: %w", err)
	}

	isVIP, err := CheckVIPStatus(ssoCookie)
	if err != nil {
		logger.Log.WithError(err).Warn("Failed to check VIP status, defaulting to false")
		isVIP = false
	}

	expirationTimestamp, err := DecodeSSOCookie(ssoCookie)
	if err != nil {
		return nil, fmt.Errorf("failed to decode SSO cookie: %w", err)
	}

	return &AccountValidationResult{
		IsValid:     true,
		Created:     created.Unix(),
		IsVIP:       isVIP,
		ExpiresAt:   expirationTimestamp,
		ProfileData: profileData,
	}, nil
}
