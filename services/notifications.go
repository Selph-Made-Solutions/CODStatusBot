package services

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"gorm.io/gorm"
	"time"
)

func checkNotificationCooldown(userID, notificationType string, cooldownDuration time.Duration) bool {
	var cooldown models.NotificationCooldown
	result := database.DB.Where("user_id = ? AND notification_type = ?", userID, notificationType).First(&cooldown)

	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		logger.Log.WithError(result.Error).Error("Error checking notification cooldown")
		return false
	}

	now := time.Now()
	if result.Error == gorm.ErrRecordNotFound || now.Sub(cooldown.LastNotification) >= cooldownDuration {
		cooldown.UserID = userID
		cooldown.NotificationType = notificationType
		cooldown.LastNotification = now

		if result := database.DB.Save(&cooldown); result.Error != nil {
			logger.Log.WithError(result.Error).Error("Error saving notification cooldown")
			return false
		}
		return true
	}

	return false
}
