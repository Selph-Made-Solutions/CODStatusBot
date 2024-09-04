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
	"sync"
	"time"
)

var (
	defaultSettings       models.UserSettings
	userSettingsCache     = make(map[string]models.UserSettings)
	userSettingsCacheMu   sync.RWMutex
	userSettingsCacheTTL  = 5 * time.Minute
	userSettingsCacheTime = make(map[string]time.Time)
	userRateLimiters      = make(map[string]*time.Time)
	rateLimitMutex        sync.Mutex
)

func init() {
	loadDefaultSettings()
}

func loadDefaultSettings() {
	defaultSettings = models.UserSettings{
		CheckInterval:        getEnvAsInt("CHECK_INTERVAL", 15),
		NotificationInterval: getEnvAsFloat("NOTIFICATION_INTERVAL", 24),
		CooldownDuration:     getEnvAsFloat("COOLDOWN_DURATION", 6),
		StatusChangeCooldown: getEnvAsFloat("STATUS_CHANGE_COOLDOWN", 1),
		NotificationType:     "channel",
	}

	logger.Log.Infof("Default settings loaded: CheckInterval=%d, NotificationInterval=%.2f, CooldownDuration=%.2f, StatusChangeCooldown=%.2f",
		defaultSettings.CheckInterval, defaultSettings.NotificationInterval, defaultSettings.CooldownDuration, defaultSettings.StatusChangeCooldown)
}

func getEnvAsInt(key string, fallback int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return fallback
}

func getEnvAsFloat(key string, fallback float64) float64 {
	if value, exists := os.LookupEnv(key); exists {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return fallback
}

func GetUserSettings(userID string, installType models.InstallationType) (models.UserSettings, error) {
	logger.Log.Infof("Getting user settings for user: %s, installation type: %d", userID, installType)

	if settings, ok := getUserSettingsFromCache(userID); ok {
		return settings, nil
	}

	settings, err := getUserSettingsFromDB(userID)
	if err != nil {
		return settings, err
	}

	applyDefaultSettingsIfNeeded(&settings)
	setNotificationTypeBasedOnInstallType(&settings, installType)

	updateUserSettingsCache(userID, settings)

	logger.Log.Infof("Got user settings for user: %s", userID)
	return settings, nil
}

func getUserSettingsFromCache(userID string) (models.UserSettings, bool) {
	userSettingsCacheMu.RLock()
	defer userSettingsCacheMu.RUnlock()

	settings, ok := userSettingsCache[userID]
	if ok && time.Since(userSettingsCacheTime[userID]) < userSettingsCacheTTL {
		return settings, true
	}

	return models.UserSettings{}, false
}

func getUserSettingsFromDB(userID string) (models.UserSettings, error) {
	var settings models.UserSettings
	result := database.GetDB().Where(models.UserSettings{UserID: userID}).FirstOrCreate(&settings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings from database")
		return settings, result.Error
	}
	return settings, nil
}

func applyDefaultSettingsIfNeeded(settings *models.UserSettings) {
	if settings.CaptchaAPIKey == "" {
		settings.CheckInterval = defaultSettings.CheckInterval
		settings.NotificationInterval = defaultSettings.NotificationInterval
		settings.CooldownDuration = defaultSettings.CooldownDuration
		settings.StatusChangeCooldown = defaultSettings.StatusChangeCooldown
	}
}

func setNotificationTypeBasedOnInstallType(settings *models.UserSettings, installType models.InstallationType) {
	if settings.NotificationType == "" {
		if installType == models.InstallTypeGuild {
			settings.NotificationType = "channel"
		} else {
			settings.NotificationType = "dm"
		}
	}
}

func updateUserSettingsCache(userID string, settings models.UserSettings) {
	userSettingsCacheMu.Lock()
	defer userSettingsCacheMu.Unlock()

	userSettingsCache[userID] = settings
	userSettingsCacheTime[userID] = time.Now()
}

func SetUserCaptchaKey(userID string, captchaKey string) error {
	if !isValidUserID(userID) {
		logger.Log.Error("Invalid userID provided")
		return fmt.Errorf("invalid userID")
	}

	settings, err := getUserSettingsFromDB(userID)
	if err != nil {
		return err
	}

	if captchaKey != "" {
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
		settings.CheckInterval = 15
		settings.NotificationInterval = 12

		logger.Log.Infof("Valid captcha key set for user: %s. Balance: %.2f points", userID, balance)
	} else {
		resetToDefaultSettings(&settings)
		logger.Log.Infof("Captcha key removed for user: %s. Reset to default settings", userID)
	}

	if err := saveUserSettings(&settings); err != nil {
		return err
	}

	updateUserSettingsCache(userID, settings)

	logger.Log.Infof("Updated captcha key and settings for user: %s", userID)
	return nil
}

func resetToDefaultSettings(settings *models.UserSettings) {
	settings.CaptchaAPIKey = ""
	settings.CheckInterval = defaultSettings.CheckInterval
	settings.NotificationInterval = defaultSettings.NotificationInterval
	settings.CooldownDuration = defaultSettings.CooldownDuration
	settings.StatusChangeCooldown = defaultSettings.StatusChangeCooldown
}

func saveUserSettings(settings *models.UserSettings) error {
	if err := database.GetDB().Save(settings).Error; err != nil {
		logger.Log.WithError(err).Error("Error saving user settings")
		return err
	}
	return nil
}

func isValidUserID(userID string) bool {
	for _, char := range userID {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func GetUserCaptchaKey(userID string) (string, error) {
	settings, err := GetUserSettings(userID, models.InstallTypeUser)
	if err != nil {
		return "", err
	}

	if settings.CaptchaAPIKey != "" {
		// Check if the key is valid
		isValid, _, err := CheckCaptchaKeyValidity(settings.CaptchaAPIKey)
		if err != nil {
			logger.Log.WithError(err).Error("Error checking captcha key validity")
		} else if !isValid {
			logger.Log.Warn("User's captcha key is invalid, falling back to default key")
			// Reset the user's API key
			settings.CaptchaAPIKey = ""
			if err := saveUserSettings(&settings); err != nil {
				logger.Log.WithError(err).Error("Failed to reset user's invalid API key")
			}
		} else {
			return settings.CaptchaAPIKey, nil
		}
	}

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
	settings, err := getUserSettingsFromDB(userID)
	if err != nil {
		return err
	}

	resetToDefaultSettings(&settings)

	if err := saveUserSettings(&settings); err != nil {
		return err
	}

	updateUserSettingsCache(userID, settings)

	logger.Log.Infof("Removed captcha key and reset settings for user: %s", userID)
	return nil
}

func UpdateUserSettings(userID string, newSettings models.UserSettings) error {
	settings, err := getUserSettingsFromDB(userID)
	if err != nil {
		return err
	}

	// Only allow updating settings if the user has a valid API key
	if settings.CaptchaAPIKey != "" {
		updateSettingsIfValid(&settings, newSettings)
	}

	settings.NotificationType = newSettings.NotificationType

	if err := saveUserSettings(&settings); err != nil {
		return err
	}

	updateUserSettingsCache(userID, settings)

	logger.Log.Infof("Updated settings for user: %s", userID)
	return nil
}

func updateSettingsIfValid(settings *models.UserSettings, newSettings models.UserSettings) {
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

func CheckDefaultKeyRateLimit(userID string) bool {
	rateLimitMutex.Lock()
	defer rateLimitMutex.Unlock()

	lastUse, exists := userRateLimiters[userID]
	if !exists || time.Since(*lastUse) >= time.Hour {
		now := time.Now()
		userRateLimiters[userID] = &now
		return true
	}
	return false
}
