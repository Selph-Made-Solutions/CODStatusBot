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

// var url1 = "https://support.activision.com/api/bans/v2/appeal?locale=en" // Replacement Endpoint for checking account bans
var url1 = "https://support.activision.com/api/bans/appeal?locale=en" // Endpoint for checking account bans
var url2 = "https://support.activision.com/api/profile?accts=false"   // Endpoint for retrieving profile information
// var url3 = "https://profile.callofduty.com/promotions/redeemCode/" // Endpoint for claiming rewards (currently unused)
// var url4 = "https://profile.callofduty.com/api/papi-client/crm/cod/v2/accounts" // Endpoint for retrieving linked platforms and their associated IDs (currently unused)
// var url5 = "https://www.callofduty.com/api/papi-client/crm/cod/v2/identities/" // Endpoint for retrieving (currently unused)
// var url6 = "https://s.activision.com/activision/userInfo/{SSO_COOKIE}" // Endpoint for retrieving extremely detailed account information (currently unused)

/*
func ClaimSingleReward(ssoCookie, code string) (string, error) {
	logger.Log.Info("Starting ClaimSingleReward function")
	req, err := http.NewRequest("POST", url3, strings.NewReader(fmt.Sprintf("code=%s", code)))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request to claim reward: %w", err)
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
		return "", fmt.Errorf("failed to send HTTP request to claim reward: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	if strings.Contains(string(body), "redemption-success") {
		start := strings.Index(string(body), "Just Unlocked:<br><br><div class=\"accent-highlight mw2\">")
		end := strings.Index(string(body), "</div></h4>")
		if start != -1 && end != -1 {
			unlockedItem := strings.TrimSpace(string(body)[start+len("Just Unlocked:<br><br><div class=\"accent-highlight mw2\">") : end])
			return fmt.Sprintf("Successfully claimed reward: %s", unlockedItem), nil
		}
		return "Successfully claimed reward, but couldn't extract details", nil
	}
	logger.Log.Infof("Unexpected response body: %s", string(body))
	return "", fmt.Errorf("failed to claim reward: unexpected response")
}
*/

// VerifySSOCookie checks if the provided SSO cookie is valid.
func VerifySSOCookie(ssoCookie string) bool {
	logger.Log.Infof("Verifying SSO cookie: %s ", ssoCookie)
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
		logger.Log.Errorf("Invalid SSOCookie, status code: %d ", resp.StatusCode)
		return false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Log.WithError(err).Error("Error reading verification response body")
		return false
	}
	// possible bug use either len or string not sure
	if len(body) == 0 { // Check if the response body is empty
		// if string(body) == "" { // Check if the response body is empty
		logger.Log.Error("Invalid SSOCookie, response body is empty")
		return false
	}
	return true
}

// CheckAccount checks the account status associated with the provided SSO cookie.
func CheckAccount(ssoCookie string) (models.Status, error) {
	logger.Log.Info("Starting CheckAccount function")
	req, err := http.NewRequest("GET", url1, nil)
	if err != nil {
		return models.StatusUnknown, errors.New("failed to create HTTP request to check account")
	}
	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return models.StatusUnknown, errors.New("failed to send HTTP request to check account")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.StatusUnknown, errors.New("failed to read response body from check account request")
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
		return models.StatusInvalidCookie, nil
	}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return models.StatusUnknown, fmt.Errorf("failed to decode JSON response: %v ", err)
	}
	if len(data.Bans) == 0 {
		return models.StatusGood, nil
	}
	for _, ban := range data.Bans {
		switch ban.Enforcement {
		case "PERMANENT":
			return models.StatusPermaban, nil
		case "UNDER_REVIEW":
			return models.StatusShadowban, nil
		}
	}
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
