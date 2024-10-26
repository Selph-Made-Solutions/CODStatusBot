package services

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
)

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
		Color:       0xFF0000,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	_, err = s.ChannelMessageSendEmbed(channel.ID, embed)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send admin notification")
	}
}

func GetCooldownDuration(userSettings models.UserSettings, notificationType string, defaultCooldown time.Duration) time.Duration {
	switch notificationType {
	case "status_change":
		return time.Duration(userSettings.StatusChangeCooldown) * time.Hour
	case "daily_update", "invalid_cookie", "cookie_expiring_soon":
		return time.Duration(userSettings.NotificationInterval) * time.Hour
	default:
		return defaultCooldown
	}
}

func GetNotificationChannel(s *discordgo.Session, account models.Account, userSettings models.UserSettings) (string, error) {
	if userSettings.NotificationType == "dm" {
		channel, err := s.UserChannelCreate(account.UserID)
		if err != nil {
			return "", fmt.Errorf("failed to create DM channel: %w", err)
		}
		return channel.ID, nil
	}
	return account.ChannelID, nil
}

func FormatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func CheckAndNotifyBalance(s *discordgo.Session, userID string, balance float64) {
	userSettings, err := GetUserSettings(userID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to get user settings for balance check: %s", userID)
		return
	}

	if time.Since(userSettings.LastBalanceNotification) < 24*time.Hour {
		return
	}

	if !IsServiceEnabled(userSettings.PreferredCaptchaProvider) {
		logger.Log.Infof("Skipping balance check for disabled service: %s", userSettings.PreferredCaptchaProvider)
		return
	}

	var thresholds = map[string]float64{
		"ezcaptcha": 250,
		"2captcha":  0.25,
	}

	threshold := thresholds[userSettings.PreferredCaptchaProvider]
	if balance < threshold {
		embed := &discordgo.MessageEmbed{
			Title: fmt.Sprintf("Low %s Balance Alert", userSettings.PreferredCaptchaProvider),
			Description: fmt.Sprintf("Your %s balance is currently %.2f points, which is below the recommended threshold of %.2f points.",
				userSettings.PreferredCaptchaProvider, balance, threshold),
			Color: 0xFFA500,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name: "Action Required",
					Value: fmt.Sprintf("Please recharge your %s balance to ensure uninterrupted service for your account checks.",
						userSettings.PreferredCaptchaProvider),
					Inline: false,
				},
				{
					Name:   "Current Provider",
					Value:  userSettings.PreferredCaptchaProvider,
					Inline: true,
				},
				{
					Name:   "Current Balance",
					Value:  fmt.Sprintf("%.2f", balance),
					Inline: true,
				},
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}

		var account models.Account
		if err := database.DB.Where("user_id = ?", userID).First(&account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to get account for balance notification: %s", userID)
			return
		}

		err := SendNotification(s, account, embed, "", "balance_warning")
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send balance notification to user %s", userID)
			return
		}

		userSettings.LastBalanceNotification = time.Now()
		if err := database.DB.Save(&userSettings).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to update LastBalanceNotification for user %s", userID)
		}
	}
}

func ScheduleBalanceChecks(s *discordgo.Session) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		var users []models.UserSettings
		if err := database.DB.Find(&users).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to fetch users for balance check")
			continue
		}

		for _, user := range users {
			if !IsServiceEnabled(user.PreferredCaptchaProvider) {
				continue
			}

			if user.EZCaptchaAPIKey == "" && user.TwoCaptchaAPIKey == "" {
				continue
			}

			var apiKey string
			var provider string
			if user.PreferredCaptchaProvider == "2captcha" && user.TwoCaptchaAPIKey != "" {
				apiKey = user.TwoCaptchaAPIKey
				provider = "2captcha"
			} else if user.PreferredCaptchaProvider == "ezcaptcha" && user.EZCaptchaAPIKey != "" {
				apiKey = user.EZCaptchaAPIKey
				provider = "ezcaptcha"
			} else {
				continue
			}

			isValid, balance, err := ValidateCaptchaKey(apiKey, provider)
			if err != nil {
				logger.Log.WithError(err).Errorf("Failed to validate %s key for user %s", provider, user.UserID)
				continue
			}

			if !isValid {
				if err := DisableUserCaptcha(s, user.UserID, fmt.Sprintf("Invalid %s API key", provider)); err != nil {
					logger.Log.WithError(err).Errorf("Failed to disable captcha for user %s", user.UserID)
				}
				continue
			}

			user.CaptchaBalance = balance
			user.LastBalanceCheck = time.Now()
			if err := database.DB.Save(&user).Error; err != nil {
				logger.Log.WithError(err).Errorf("Failed to update balance for user %s", user.UserID)
				continue
			}

			CheckAndNotifyBalance(s, user.UserID, balance)
		}
	}
}

func DisableUserCaptcha(s *discordgo.Session, userID string, reason string) error {
	var settings models.UserSettings
	if err := database.DB.Where("user_id = ?", userID).First(&settings).Error; err != nil {
		return err
	}

	settings.TwoCaptchaAPIKey = ""
	if IsServiceEnabled("ezcaptcha") {
		settings.PreferredCaptchaProvider = "ezcaptcha"
	} else if IsServiceEnabled("2captcha") {
		settings.PreferredCaptchaProvider = "2captcha"
	} else {
		settings.PreferredCaptchaProvider = "ezcaptcha"
	}

	settings.EZCaptchaAPIKey = ""
	settings.CustomSettings = false
	settings.CheckInterval = defaultSettings.CheckInterval
	settings.NotificationInterval = defaultSettings.NotificationInterval

	if err := database.DB.Save(&settings).Error; err != nil {
		return err
	}

	embed := &discordgo.MessageEmbed{
		Title: "Captcha Service Configuration Update",
		Description: fmt.Sprintf("Your captcha service configuration has been updated. Reason: %s\n\n"+
			"Current available services: %s\n"+
			"The bot will use default settings for the available service.",
			reason,
			getEnabledServicesString()),
		Color:     0xFF0000,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	var account models.Account
	if err := database.DB.Where("user_id = ?", userID).First(&account).Error; err != nil {
		return err
	}

	return SendNotification(s, account, embed, "", "captcha_disabled")
}

func getEnabledServicesString() string {
	var enabledServices []string
	if IsServiceEnabled("ezcaptcha") {
		enabledServices = append(enabledServices, "EZCaptcha")
	}
	if IsServiceEnabled("2captcha") {
		enabledServices = append(enabledServices, "2Captcha")
	}
	if len(enabledServices) == 0 {
		return "No services currently enabled"
	}
	return strings.Join(enabledServices, ", ")
}

func SendTempBanUpdateNotification(s *discordgo.Session, account models.Account, remainingTime time.Duration) {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Temporary Ban Update", account.Title),
		Description: fmt.Sprintf("Your account is still temporarily banned. Remaining time: %v", remainingTime),
		Color:       GetColorForStatus(models.StatusTempban, false, account.IsCheckDisabled),
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	err := SendNotification(s, account, embed, "", "temp_ban_update")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send temporary ban update for account %s", account.Title)
	}
}
