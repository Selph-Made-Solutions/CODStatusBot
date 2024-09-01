package services

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
)

var defaultSettings models.UserSettings

func init() {
	checkInterval, err := strconv.Atoi(os.Getenv("CHECK_INTERVAL"))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse CHECK_INTERVAL, using default of 15 minutes")
		checkInterval = 15
	}

	notificationInterval, err := strconv.ParseFloat(os.Getenv("NOTIFICATION_INTERVAL"), 64)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse NOTIFICATION_INTERVAL, using default of 24 hours")
		notificationInterval = 24
	}

	cooldownDuration, err := strconv.ParseFloat(os.Getenv("COOLDOWN_DURATION"), 64)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse COOLDOWN_DURATION, using default of 6 hours")
		cooldownDuration = 6
	}

	statusChangeCooldown, err := strconv.ParseFloat(os.Getenv("STATUS_CHANGE_COOLDOWN"), 64)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse STATUS_CHANGE_COOLDOWN, using default of 1 hour")
		statusChangeCooldown = 1
	}

	defaultSettings = models.UserSettings{
		CheckInterval:        checkInterval,
		NotificationInterval: notificationInterval,
		CooldownDuration:     cooldownDuration,
		StatusChangeCooldown: statusChangeCooldown,
		NotificationType:     "channel",
	}

	logger.Log.Infof("Default settings loaded: CheckInterval=%d, NotificationInterval=%.2f, CooldownDuration=%.2f, StatusChangeCooldown=%.2f",
		defaultSettings.CheckInterval, defaultSettings.NotificationInterval, defaultSettings.CooldownDuration, defaultSettings.StatusChangeCooldown)
}

func GetUserSettings(userID string, installType models.InstallationType) (models.UserSettings, error) {
	logger.Log.Infof("Getting user settings for user: %s", userID)
	var settings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&settings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings")
		return settings, result.Error
	}

	// If the user doesn't have custom settings or doesn't have a custom API key, use default settings
	if settings.CaptchaAPIKey == "" {
		settings.CheckInterval = defaultSettings.CheckInterval
		settings.NotificationInterval = defaultSettings.NotificationInterval
		settings.CooldownDuration = defaultSettings.CooldownDuration
		settings.StatusChangeCooldown = defaultSettings.StatusChangeCooldown
	}

	// Set notification type based on installation type if not already set
	if settings.NotificationType == "" {
		if installType == models.InstallTypeGuild {
			settings.NotificationType = "channel"
		} else {
			settings.NotificationType = "dm"
		}
	}

	logger.Log.Infof("Got user settings for user: %s", userID)
	return settings, nil
}

func SetUserCaptchaKey(userID string, captchaKey string) error {
	// Check if userID is not an API key
	if len(userID) > 20 || !isValidUserID(userID) {
		logger.Log.Error("Invalid userID provided")
		return fmt.Errorf("invalid userID")
	}

	var settings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&settings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error setting user settings")
		return result.Error
	}

	if captchaKey != "" {
		// Validate the captcha key before setting it and get balance
		isValid, balance, err := CheckCaptchaKeyValidity(captchaKey)
		if err != nil {
			logger.Log.WithError(err).Error("Error validating captcha key")
			return err
		}
		if !isValid {
			logger.Log.Error("Invalid captcha key provided")
			return fmt.Errorf("invalid captcha key")
		}

		settings.CaptchaAPIKey = captchaKey
		// Enable custom settings when user sets their own valid API key
		settings.CheckInterval = 15        // Allow more frequent checks, e.g., every 15 minutes
		settings.NotificationInterval = 12 // Allow more frequent notifications, e.g., every 12 hours

		logger.Log.Infof("Valid captcha key set for user: %s. Balance: %.2f points", userID, balance)
	} else {
		// Reset to default settings when API key is removed
		settings.CaptchaAPIKey = ""
		settings.CheckInterval = defaultSettings.CheckInterval
		settings.NotificationInterval = defaultSettings.NotificationInterval
		settings.CooldownDuration = defaultSettings.CooldownDuration
		settings.StatusChangeCooldown = defaultSettings.StatusChangeCooldown
		// Keep the user's notification type preference

		logger.Log.Infof("Captcha key removed for user: %s. Reset to default settings", userID)
	}

	if err := database.DB.Save(&settings).Error; err != nil {
		logger.Log.WithError(err).Error("Error saving user settings")
		return err
	}

	logger.Log.Infof("Updated captcha key and settings for user: %s", userID)
	return nil
}

// Add this helper function to validate userID
func isValidUserID(userID string) bool {
	// Check if userID consists only of digits (Discord user IDs are numeric)
	for _, char := range userID {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
func GetUserCaptchaKey(userID string) (string, error) {
	var settings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).First(&settings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings")
		return "", result.Error
	}

	// If the user has a custom API key, return it
	if settings.CaptchaAPIKey != "" {
		return settings.CaptchaAPIKey, nil
	}

	// If the user doesn't have a custom API key, return the default key
	defaultKey := os.Getenv("EZCAPTCHA_CLIENT_KEY")
	if defaultKey == "" {
		return "", fmt.Errorf("default EZCAPTCHA_CLIENT_KEY not set in environment")
	}
	return defaultKey, nil
}

func GetDefaultSettings() (models.UserSettings, error) {
	return defaultSettings, nil
}

func RemoveCaptchaKey(userID string) error {
	var settings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).First(&settings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error removing apikey in settings")
		return result.Error
	}

	settings.CaptchaAPIKey = ""
	settings.CheckInterval = defaultSettings.CheckInterval
	settings.NotificationInterval = defaultSettings.NotificationInterval
	settings.CooldownDuration = defaultSettings.CooldownDuration
	settings.StatusChangeCooldown = defaultSettings.StatusChangeCooldown
	// Keep the user's notification type preference

	if err := database.DB.Save(&settings).Error; err != nil {
		logger.Log.WithError(err).Error("Error saving user settings")
		return err
	}

	logger.Log.Infof("Removed captcha key and reset settings for user: %s", userID)
	return nil
}

func UpdateUserSettings(userID string, newSettings models.UserSettings) error {
	var settings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&settings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error updating user settings")
		return result.Error
	}

	// Only allow updating settings if the user has a valid API key
	if settings.CaptchaAPIKey != "" {
		if newSettings.CheckInterval != 0 {
			settings.CheckInterval = newSettings.CheckInterval
		}
		if newSettings.NotificationInterval != 0 {
			settings.NotificationInterval = newSettings.NotificationInterval
		}
		if newSettings.CooldownDuration != 0 {
			settings.CooldownDuration = newSettings.CooldownDuration
		}
		if newSettings.StatusChangeCooldown != 0 {
			settings.StatusChangeCooldown = newSettings.StatusChangeCooldown
		}
	}

	// Allow updating notification type regardless of API key
	if newSettings.NotificationType != "" {
		settings.NotificationType = newSettings.NotificationType
	}

	if err := database.DB.Save(&settings).Error; err != nil {
		logger.Log.WithError(err).Error("Error updating user settings")
		return err
	}

	logger.Log.Infof("Updated settings for user: %s", userID)
	return nil
}

func CheckCaptchaKeyValidity(captchaKey string) (bool, float64, error) {
	url := "https://api.ez-captcha.com/getBalance"
	payload := map[string]string{
		"clientKey": captchaKey,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return false, 0, fmt.Errorf("failed to marshal JSON payload: %v", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return false, 0, fmt.Errorf("failed to send getBalance request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, 0, fmt.Errorf("failed to read response body: %v", err)
	}

	var result struct {
		ErrorId int     `json:"errorId"`
		Balance float64 `json:"balance"`
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return false, 0, fmt.Errorf("failed to parse JSON response: %v", err)
	}

	if result.ErrorId != 0 {
		return false, 0, nil
	}

	return true, result.Balance, nil
}
