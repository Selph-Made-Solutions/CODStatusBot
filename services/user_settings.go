package services

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"os"
	"strconv"
)

var defaultSettings models.UserSettings

func init() {
	checkInterval, _ := strconv.Atoi(os.Getenv("CHECK_INTERVAL"))
	notificationInterval, _ := strconv.ParseFloat(os.Getenv("NOTIFICATION_INTERVAL"), 64)
	cooldownDuration, _ := strconv.ParseFloat(os.Getenv("COOLDOWN_DURATION"), 64)
	statusChangeCooldown, _ := strconv.ParseFloat(os.Getenv("STATUS_CHANGE_COOLDOWN"), 64)

	defaultSettings = models.UserSettings{
		CheckInterval:        checkInterval,
		NotificationInterval: notificationInterval,
		CooldownDuration:     cooldownDuration,
		StatusChangeCooldown: statusChangeCooldown,
		NotificationType:     "channel",
	}
}

func GetUserSettings(userID string) (models.UserSettings, error) {
	logger.Log.Infof("Getting user settings for user: %s", userID)
	var settings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&settings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings")
		return settings, result.Error
	}

	// If the user doesn't have a captcha key, use default settings
	if settings.CaptchaAPIKey == "" {
		settings.CheckInterval = defaultSettings.CheckInterval
		settings.NotificationInterval = defaultSettings.NotificationInterval
		settings.CooldownDuration = defaultSettings.CooldownDuration
		settings.StatusChangeCooldown = defaultSettings.StatusChangeCooldown
		settings.NotificationType = defaultSettings.NotificationType
	}

	logger.Log.Infof("Got user settings for user: %s", userID)
	return settings, nil
}

func GetDefaultSettings() (models.UserSettings, error) {
	return defaultSettings, nil
}

func RemoveCaptchaKey(userID string) error {
	var settings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).First(&settings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings")
		return result.Error
	}

	settings.CaptchaAPIKey = ""
	settings.CheckInterval = defaultSettings.CheckInterval
	settings.NotificationInterval = defaultSettings.NotificationInterval
	settings.CooldownDuration = defaultSettings.CooldownDuration
	settings.StatusChangeCooldown = defaultSettings.StatusChangeCooldown
	settings.NotificationType = defaultSettings.NotificationType

	if err := database.DB.Save(&settings).Error; err != nil {
		logger.Log.WithError(err).Error("Error saving user settings")
		return err
	}

	return nil
}
