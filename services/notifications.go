package services

import (
	"CODStatusBot/admin"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/patrickmn/go-cache"
	"os"
	"strings"
	"time"
)

var adminNotificationCache = cache.New(5*time.Minute, 10*time.Minute)

func NotifyAdminWithCooldown(s *discordgo.Session, message string, cooldownDuration time.Duration) {
	admin.NotificationMutex.Lock()
	defer admin.NotificationMutex.Unlock()

	notificationType := "admin_" + strings.Split(message, " ")[0] // Use first word of message as type

	_, found := adminNotificationCache.Get(notificationType)
	if !found {
		NotifyAdmin(s, message)
		adminNotificationCache.Set(notificationType, time.Now(), cooldownDuration)
	} else {
		logger.Log.Infof("Skipping admin notification '%s' due to cooldown", notificationType)
	}
}

func NotifyAdmin(s *discordgo.Session, message string) {
	adminID := os.Getenv("DEVELOPER_ID")
	if adminID == "" {
		logger.Log.Error("DEVELOPER_ID not set in environment variables")
		return
	}

	channel, err := s.UserChannelCreate(adminID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create DM channel with admin")
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Admin Notification",
		Description: message,
		Color:       0xFF0000, // Red color for admin notifications
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	_, err = s.ChannelMessageSendEmbed(channel.ID, embed)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send admin notification")
	}
}

func checkNotificationCooldown(userID string, notificationType string, cooldownDuration time.Duration) (bool, error) {
	var settings models.UserSettings
	if err := database.DB.Where("user_id = ?", userID).First(&settings).Error; err != nil {
		return false, err
	}

	var lastNotification time.Time
	switch notificationType {
	case "balance":
		lastNotification = settings.LastBalanceNotification
	case "error":
		lastNotification = settings.LastErrorNotification
	case "disabled":
		lastNotification = settings.LastDisabledNotification
	default:
		return false, fmt.Errorf("unknown notification type: %s", notificationType)
	}

	if time.Since(lastNotification) >= cooldownDuration {
		return true, nil
	}
	return false, nil
}

func updateNotificationTimestamp(userID string, notificationType string) error {
	var settings models.UserSettings
	if err := database.DB.Where("user_id = ?", userID).First(&settings).Error; err != nil {
		return err
	}

	now := time.Now()
	switch notificationType {
	case "balance":
		settings.LastBalanceNotification = now
	case "error":
		settings.LastErrorNotification = now
	case "disabled":
		settings.LastDisabledNotification = now
	default:
		return fmt.Errorf("unknown notification type: %s", notificationType)
	}

	return database.DB.Save(&settings).Error
}
