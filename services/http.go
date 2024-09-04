package services

import (
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

const (
	banAppealURL = "https://support.activision.com/api/bans/v2/appeal?locale=en" // Replacement Endpoint for checking
	profileURL   = "https://support.activision.com/api/profile?accts=false"      // Endpoint for retrieving profile information
)

// VerifySSOCookie checks if the provided SSO cookie is valid.
func VerifySSOCookie(ssoCookie string) bool {
	logger.Log.Infof("Verifying SSO cookie: %s", ssoCookie)
	req, err := createRequest("GET", profileURL, ssoCookie)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating verification request")
		return false
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
func CheckAccount(ssoCookie string, userID string, installType models.InstallationType) (models.AccountStatus, error) {
	logger.Log.Info("Starting CheckAccount function")

	gRecaptchaResponse, err := SolveReCaptchaV2(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to solve reCAPTCHA")
		return models.AccountStatus{}, fmt.Errorf("failed to solve reCAPTCHA: %v", err)
	}

	if gRecaptchaResponse == "" {
		logger.Log.Error("Received empty reCAPTCHA response")
		return models.AccountStatus{}, fmt.Errorf("received empty reCAPTCHA response")
	}

	logger.Log.Info("Successfully solved reCAPTCHA")

	banAppealURLWithCaptcha := fmt.Sprintf("%s&g-cc=%s", banAppealURL, gRecaptchaResponse)
	logger.Log.WithField("url", banAppealURLWithCaptcha).Info("Constructed ban appeal URL")

	req, err := createRequest("GET", banAppealURLWithCaptcha, ssoCookie)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create HTTP request")
		return models.AccountStatus{}, fmt.Errorf("failed to create HTTP request to check account: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send HTTP request")
		return models.AccountStatus{}, fmt.Errorf("failed to send HTTP request to check account: %v", err)
	}
	defer resp.Body.Close()

	logger.Log.WithField("status", resp.Status).Info("Received response")

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to read response body")
		return models.AccountStatus{}, fmt.Errorf("failed to read response body from check account request: %v", err)
	}

	logger.Log.WithField("body", string(body)).Info("Read response body")

	return parseAccountStatus(body)
}

func createRequest(method, url, ssoCookie string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

func parseAccountStatus(body []byte) (models.AccountStatus, error) {
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

	err := json.Unmarshal(body, &data)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to decode JSON response")
		return models.AccountStatus{}, fmt.Errorf("failed to decode JSON response: %v", err)
	}

	logger.Log.WithField("data", data).Info("Parsed ban data")

	return processAccountStatus(data)
}

func processAccountStatus(data struct {
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
}) (models.AccountStatus, error) {
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

		gameStatus.Status = determineGameStatus(ban.Enforcement)
		accountStatus.Overall = updateOverallStatus(accountStatus.Overall, gameStatus.Status)

		accountStatus.Games[ban.Title] = gameStatus
	}

	logger.Log.WithField("accountStatus", accountStatus).Info("Processed account status")
	return accountStatus, nil
}

func determineGameStatus(enforcement string) models.Status {
	switch enforcement {
	case "PERMANENT":
		return models.StatusPermaban
	case "TEMPORARY":
		return models.StatusTempban
	case "UNDER_REVIEW":
		return models.StatusShadowban
	case "":
		return models.StatusGood
	default:
		return models.StatusUnknown
	}
}

func updateOverallStatus(currentOverall, newStatus models.Status) models.Status {
	if newStatus == models.StatusPermaban {
		return models.StatusPermaban
	}
	if newStatus == models.StatusTempban && currentOverall != models.StatusPermaban {
		return models.StatusTempban
	}
	if newStatus == models.StatusShadowban && (currentOverall != models.StatusPermaban && currentOverall != models.StatusTempban) {
		return models.StatusShadowban
	}
	return currentOverall
}

// CheckAccountAge retrieves the age of the account associated with the provided SSO cookie.
func CheckAccountAge(ssoCookie string) (int, int, int, error) {
	logger.Log.Info("Starting CheckAccountAge function")
	req, err := createRequest("GET", profileURL, ssoCookie)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to create HTTP request to check account age: %v", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to send HTTP request to check account age: %v", err)
	}
	defer resp.Body.Close()

	var data struct {
		Created string `json:"created"`
	}
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to decode JSON response from check account age request: %v", err)
	}

	logger.Log.Infof("Account created date: %s", data.Created)

	created, err := time.Parse(time.RFC3339, data.Created)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to parse created date in check account age request: %v", err)
	}

	years, months, days := calculateAge(created)

	logger.Log.Infof("Account age calculated: %d years, %d months, %d days", years, months, days)
	return years, months, days, nil
}

func calculateAge(created time.Time) (int, int, int) {
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

	return years, months, days
}
