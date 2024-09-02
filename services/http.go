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

var url1 = "https://support.activision.com/api/bans/v2/appeal?locale=en" // Replacement Endpoint for checking
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
func CheckAccount(ssoCookie string, userID string, installType models.InstallationType) (models.AccountStatus, error) {
	logger.Log.Info("Starting CheckAccount function")

	captchaAPIKey, err := GetUserCaptchaKey(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get user's captcha API key")
		return models.AccountStatus{}, fmt.Errorf("failed to get user's captcha API key: %v", err)
	}

	gRecaptchaResponse, err := SolveReCaptchaV2WithKey(captchaAPIKey)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to solve reCAPTCHA")
		return models.AccountStatus{}, fmt.Errorf("failed to solve reCAPTCHA: %v", err)
	}

	// Further processing of gRecaptchaResponse can be done here
	logger.Log.Info("Successfully solved reCAPTCHA")

	// Construct the ban appeal URL with the reCAPTCHA response
	banAppealUrl := url1 + "&g-cc=" + gRecaptchaResponse
	logger.Log.WithField("url", banAppealUrl).Info("Constructed ban appeal URL")

	req, err := http.NewRequest("GET", banAppealUrl, nil)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create HTTP request")
		return models.AccountStatus{}, errors.New("failed to create HTTP request to check account")
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
		return models.AccountStatus{}, errors.New("failed to send HTTP request to check account")
	}
	defer resp.Body.Close()

	logger.Log.WithField("status", resp.Status).Info("Received response")

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to read response body")
		return models.AccountStatus{}, errors.New("failed to read response body from check account request")
	}
	logger.Log.WithField("body", string(body)).Info("Read response body")

	var data struct {
		Error     string `json:"error"`
		Success   string `json:"success"`
		CanAppeal bool   `json:"canAppeal"`
		Bans      []struct {
			Enforcement     string `json:"enforcement"`
			DurationSeconds int    `json:"durationSeconds"`
			CanAppeal       bool   `json:"canAppeal"`
			Bar             struct {
				CaseNumber string `json:"CaseNumber"`
				Status     string `json:"Status"`
			} `json:"bar"`
			Title string `json:"title"`
		} `json:"bans"`
	}

	if string(body) == "" {
		logger.Log.Info("Empty response body, treating as invalid cookie")
		return models.AccountStatus{Overall: models.StatusInvalidCookie}, nil
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to decode JSON response")
		return models.AccountStatus{}, fmt.Errorf("failed to decode JSON response: %v", err)
	}

	logger.Log.WithField("data", data).Info("Parsed ban data")

	accountStatus := models.AccountStatus{
		Overall: models.StatusGood,
		Games:   make(map[string]models.GameStatus),
	}

	for _, ban := range data.Bans {
		gameStatus := models.GameStatus{
			Title:           ban.Title,
			Enforcement:     ban.Enforcement,
			CanAppeal:       ban.CanAppeal,
			CaseNumber:      ban.Bar.CaseNumber,
			CaseStatus:      ban.Bar.Status,
			DurationSeconds: ban.DurationSeconds,
		}

		switch ban.Enforcement {
		case "PERMANENT":
			gameStatus.Status = models.StatusPermaban
			if accountStatus.Overall != models.StatusPermaban {
				accountStatus.Overall = models.StatusPermaban
			}
		case "TEMPORARY":
			gameStatus.Status = models.StatusTempban
			if accountStatus.Overall == models.StatusGood {
				accountStatus.Overall = models.StatusTempban
			}
		case "UNDER_REVIEW":
			gameStatus.Status = models.StatusShadowban
			if accountStatus.Overall == models.StatusGood {
				accountStatus.Overall = models.StatusShadowban
			}
		case "":
			// If enforcement is empty, consider it as good status
			gameStatus.Status = models.StatusGood
		default:
			gameStatus.Status = models.StatusUnknown
			logger.Log.Infof("Unknown enforcement status for game %s: %s", ban.Title, ban.Enforcement)
		}

		accountStatus.Games[ban.Title] = gameStatus
	}

	logger.Log.WithField("accountStatus", accountStatus).Info("Processed account status")
	return accountStatus, nil
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

	logger.Log.Infof("Account created date: %s", data.Created)

	created, err := time.Parse(time.RFC3339, data.Created)
	if err != nil {
		return 0, 0, 0, errors.New("failed to parse created date in check account age request")
	}

	now := time.Now().UTC()

	years := now.Year() - created.Year()
	months := int(now.Month() - created.Month())
	days := now.Day() - created.Day()

	if months < 0 || (months == 0 && days < 0) {
		years--
		months += 12
	}

	if days < 0 {
		// Get the last day of the previous month
		lastMonth := now.AddDate(0, -1, 0)
		daysInLastMonth := time.Date(lastMonth.Year(), lastMonth.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		days += daysInLastMonth
		months--
	}

	logger.Log.Infof("Account age calculated: %d years, %d months, %d days", years, months, days)
	return years, months, days, nil
}
