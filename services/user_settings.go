package services

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"os"
	"strconv"
)

func GetUserSettings(userID string) (models.UserSettings, error) {
	logger.Log.Infof("Getting user settings for user: %s", userID)
	var settings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&settings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings")
		return settings, result.Error
	}

	// Set default values if not set
	if settings.CheckInterval == 0 {
		settings.CheckInterval, _ = strconv.Atoi(os.Getenv("CHECK_INTERVAL"))
	}
	if settings.NotificationInterval == 0 {
		settings.NotificationInterval, _ = strconv.ParseFloat(os.Getenv("NOTIFICATION_INTERVAL"), 64)
	}
	if settings.CooldownDuration == 0 {
		settings.CooldownDuration, _ = strconv.ParseFloat(os.Getenv("COOLDOWN_DURATION"), 64)
	}
	if settings.StatusChangeCooldown == 0 {
		settings.StatusChangeCooldown, _ = strconv.ParseFloat(os.Getenv("STATUS_CHANGE_COOLDOWN"), 64)
	}

	logger.Log.Infof("Got user settings for user: %s", userID)
	return settings, nil
}
