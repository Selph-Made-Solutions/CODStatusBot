package services

import (
	"codstatusbot2.0/logger"
	"codstatusbot2.0/models"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

var url1 = "https://support.activision.com/api/bans/appeal?locale=en" // Endpoint for checking account bans
var url2 = "https://support.activision.com/api/profile?accts=false"   // Endpoint for retrieving profile information
// var url3 = "https://profile.callofduty.com/promotions/redeemCode/" // Endpoint for claiming rewards (currently unused)

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
	req, err := http.NewRequest("GET", url2, nil) // Create a GET request to the profile endpoint
	if err != nil {
		logger.Log.WithError(err).Error("Error creating verification request")
		return false
	}
	headers := GenerateHeaders(ssoCookie) // Generate headers with the SSO cookie
	for k, v := range headers {
		req.Header.Set(k, v) // Set headers for the request
	}
	client := &http.Client{}
	resp, err := client.Do(req) // Send the verification request
	if err != nil {
		logger.Log.WithError(err).Error("Error sending verification request")
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { // Check if the response status is OK
		logger.Log.Errorf("Invalid SSOCookie, status code: %d ", resp.StatusCode)
		return false
	}
	body, err := io.ReadAll(resp.Body) // Read the response body
	if err != nil {
		logger.Log.WithError(err).Error("Error reading verification response body")
		return false
	}
	if len(body) == 0 { // Check if the response body is empty
		logger.Log.Error("Invalid SSOCookie, response body is empty")
		return false
	}
	return true // SSO cookie is valid
}

// CheckAccount checks the account status associated with the provided SSO cookie.
func CheckAccount(ssoCookie string) (models.Status, error) {
	logger.Log.Info("Starting CheckAccount function")
	req, err := http.NewRequest("GET", url1, nil) // Create a GET request to the ban appeal endpoint
	if err != nil {
		return models.StatusUnknown, errors.New("failed to create HTTP request to check account")
	}
	headers := GenerateHeaders(ssoCookie) // Generate headers with the SSO cookie
	for k, v := range headers {
		req.Header.Set(k, v) // Set headers for the request
	}
	client := &http.Client{}
	resp, err := client.Do(req) // Send the request to check account status
	if err != nil {
		return models.StatusUnknown, errors.New("failed to send HTTP request to check account")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body) // Read the response body
	if err != nil {
		return models.StatusUnknown, errors.New("failed to read response body from check account request")
	}
	// logger.Log.Info("Response Body: ", string(body))
	var data struct {
		Ban []struct {
			Enforcement string `json:"enforcement"` // Type of ban (e.g., "PERMANENT", "UNDER_REVIEW")
			Title       string `json:"title"`       // Title of the ban reason
			CanAppeal   bool   `json:"canAppeal"`   // Whether the ban can be appealed
		} `json:"bans"` // Array of bans associated with the account
	}
	if string(body) == "" { // Check if the response body is empty (invalid cookie)
		return models.StatusInvalidCookie, nil
	}
	err = json.NewDecoder(resp.Body).Decode(&data) // Decode the JSON response into the data struct
	if err == nil {                                // Check if decoding failed (possible no response)
		return models.StatusUnknown, errors.New("failed to decode JSON response possible no response was received")
	}
	if len(data.Ban) == 0 { // No bans found, account is in good standing
		return models.StatusGood, nil
	} else { // Iterate through the bans and determine the account status
		for _, ban := range data.Ban {
			if ban.Enforcement == "PERMANENT" {
				return models.StatusPermaban, nil // Account is permanently banned
			} else if ban.Enforcement == "UNDER_REVIEW" {
				return models.StatusShadowban, nil // Account is shadowbanned
			} else {
				return models.StatusGood, nil // Account is not banned or shadowbanned
			}
		}
	}
	return models.StatusUnknown, nil // Unknown account status
}

// CheckAccountAge retrieves the age of the account associated with the provided SSO cookie.
func CheckAccountAge(ssoCookie string) (int, int, int, error) {
	logger.Log.Info("Starting CheckAccountAge function")
	req, err := http.NewRequest("GET", url2, nil) // Create a GET request to the profile endpoint
	if err != nil {
		return 0, 0, 0, errors.New("failed to create HTTP request to check account age")
	}
	headers := GenerateHeaders(ssoCookie) // Generate headers with the SSO cookie
	for k, v := range headers {
		req.Header.Set(k, v) // Set headers for the request
	}
	client := &http.Client{}
	resp, err := client.Do(req) // Send the request to retrieve profile information
	if err != nil {
		return 0, 0, 0, errors.New("failed to send HTTP request to check account age")
	}
	defer resp.Body.Close()
	var data struct {
		Created string `json:"created"` // Creation date of the account in RFC3339 format
	}
	err = json.NewDecoder(resp.Body).Decode(&data) // Decode the JSON response into the data struct
	if err != nil {
		return 0, 0, 0, errors.New("failed to decode JSON response from check account age request")
	}

	created, err := time.Parse(time.RFC3339, data.Created) // Parse the creation date
	if err != nil {
		return 0, 0, 0, errors.New("failed to parse created date in check account age request")
	}

	duration := time.Since(created)             // Calculate the duration since account creation
	years := int(duration.Hours() / 24 / 365)   // Calculate years
	months := int(duration.Hours()/24/30) % 12  // Calculate months (remainder after years)
	days := int(duration.Hours()/24) % 365 % 30 // Calculate days (remainder after years and months)

	return years, months, days, nil // Return the account age in years, months, and days
}
