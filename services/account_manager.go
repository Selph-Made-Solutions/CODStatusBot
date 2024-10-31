package services

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
)

type RateLimiter struct {
	sync.RWMutex
	limits map[string]time.Time
	rates  map[string]time.Duration
}

func (r *RateLimiter) Allow(key string, rate time.Duration) bool {
	r.Lock()
	defer r.Unlock()

	now := time.Now()
	if lastTime, exists := r.limits[key]; exists {
		if now.Sub(lastTime) < rate {
			return false
		}
	}

	r.limits[key] = now
	r.rates[key] = rate
	return true
}

func isChannelError(err error) bool {
	return strings.Contains(err.Error(), "Missing Access") ||
		strings.Contains(err.Error(), "Unknown Channel") ||
		strings.Contains(err.Error(), "Missing Permissions")
}

/*
	func updateNotificationTimestamp(userID string, notificationType string) {
		var settings models.UserSettings
		if err := database.DB.Where("user_id = ?", userID).First(&settings).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to get user settings for timestamp update")
			return
		}

		now := time.Now()
		switch notificationType {
		case "status_change":
			settings.LastStatusChangeNotification = now
		case "daily_update":
			settings.LastDailyUpdateNotification = now
		case "cookie_expiring":
			settings.LastCookieExpirationWarning = now
		case "error":
			settings.LastErrorNotification = now
		default:
			settings.LastNotification = now
		}

		if err := database.DB.Save(&settings).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to update notification timestamp")
		}
	}
*/

/*
	func checkNotificationCooldown(userID string, notificationType string, cooldownDuration time.Duration) bool {
		var settings models.UserSettings
		if err := database.DB.Where("user_id = ?", userID).First(&settings).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to get user settings for cooldown check")
			return false
		}

		var lastNotification time.Time
		switch notificationType {
		case "status_change":
			lastNotification = settings.LastStatusChangeNotification
		case "daily_update":
			lastNotification = settings.LastDailyUpdateNotification
		case "cookie_expiring":
			lastNotification = settings.LastCookieExpirationWarning
		case "error":
			lastNotification = settings.LastErrorNotification
		default:
			lastNotification = settings.LastNotification
		}

		return time.Since(lastNotification) >= cooldownDuration
	}
*/

func getNotificationChannel(s *discordgo.Session, account models.Account, userSettings models.UserSettings) (string, error) {
	if userSettings.NotificationType == "dm" {
		channel, err := s.UserChannelCreate(account.UserID)
		if err != nil {
			return "", fmt.Errorf("failed to create DM channel: %w", err)
		}
		return channel.ID, nil
	}
	return account.ChannelID, nil
}

func processUserAccounts(s *discordgo.Session, userID string, accounts []models.Account) {
	userSettings, err := GetUserSettings(userID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", userID)
		return
	}

	if err := validateUserCaptchaService(userID, userSettings); err != nil {
		logger.Log.WithError(err).Errorf("Captcha service validation failed for user %s", userID)
		notifyUserOfServiceIssue(s, userID, err)
		return
	}

	for _, account := range accounts {
		if !shouldCheckAccount(account, userSettings, time.Now()) {
			continue
		}

		if err := processAccountCheck(s, account, userSettings); err != nil {
			logger.Log.WithError(err).Errorf("Error checking account %s: %v", account.Title, err)
		}

		time.Sleep(time.Second * 2)
	}

	var dailyUpdateAccounts []models.Account
	var expiringAccounts []models.Account
	now := time.Now()

	for _, account := range accounts {
		if shouldIncludeInDailyUpdate(account, userSettings, now) {
			dailyUpdateAccounts = append(dailyUpdateAccounts, account)
		}

		if shouldCheckExpiration(account, now) {
			expiringAccounts = append(expiringAccounts, account)
		}
	}

	if len(dailyUpdateAccounts) > 0 {
		SendConsolidatedDailyUpdate(s, userID, userSettings, dailyUpdateAccounts)
	}

	if len(expiringAccounts) > 0 {
		NotifyCookieExpiringSoon(s, expiringAccounts)
	}
}

func notifyUserOfServiceIssue(s *discordgo.Session, userID string, err error) {
	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create DM channel for service issue")
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: "Service Issue Detected",
		Description: fmt.Sprintf("There is an issue with your account monitoring service: %v\n"+
			"Please check your settings and ensure your captcha service is properly configured.",
			err),
		Color: 0xFF0000,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Action Required",
				Value:  "Please use /setcaptchaservice to review and update your settings.",
				Inline: false,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	_, err = s.ChannelMessageSendEmbed(channel.ID, embed)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send service issue notification")
	}
}

func processAccountCheck(s *discordgo.Session, account models.Account, userSettings models.UserSettings) error {
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		status, err := CheckAccount(account.SSOCookie, account.UserID, "")
		if err != nil {
			if attempt == maxRetryAttempts {
				DBMutex.Lock()
				account.ConsecutiveErrors++
				account.LastErrorTime = time.Now()

				switch {
				case strings.Contains(err.Error(), "Missing Access") || strings.Contains(err.Error(), "Unknown Channel"):
					disableAccount(s, account, "Bot removed from server/channel")
				case strings.Contains(err.Error(), "insufficient balance"):
					disableAccount(s, account, "Insufficient captcha balance")
				case strings.Contains(err.Error(), "invalid captcha API key"):
					disableAccount(s, account, "Invalid captcha API key")
				default:
					if account.ConsecutiveErrors >= maxConsecutiveErrors {
						disableAccount(s, account, fmt.Sprintf("Too many consecutive errors: %v", err))
					} else {
						logger.Log.WithError(err).Errorf("Failed to check account %s: possible expired SSO Cookie", account.Title)
						if err := database.DB.Save(&account).Error; err != nil {
							logger.Log.WithError(err).Errorf("Failed to update account %s after error", account.Title)
						}
						notifyUserOfError(s, account, err)
					}
				}
				DBMutex.Unlock()
				NotifyAdminWithCooldown(s, fmt.Sprintf("Error checking account %s: %v", account.Title, err), 5*time.Minute)
				return err
			}
			logger.Log.Infof("Retrying account check after error (attempt %d/%d): %v", attempt, maxRetryAttempts, err)
			time.Sleep(retryDelay)
			continue
		}

		DBMutex.Lock()
		account.LastStatus = status
		account.LastCheck = time.Now().Unix()
		account.ConsecutiveErrors = 0
		if err := database.DB.Save(&account).Error; err != nil {
			DBMutex.Unlock()
			return fmt.Errorf("failed to update account status: %w", err)
		}
		DBMutex.Unlock()

		if account.LastStatus != status {
			HandleStatusChange(s, account, status, userSettings)
		}

		return nil
	}
	return fmt.Errorf("max retries exceeded for account %s", account.Title)
}

func handleCheckFailure(s *discordgo.Session, account models.Account, err error) {
	DBMutex.Lock()
	defer DBMutex.Unlock()

	account.ConsecutiveErrors++
	account.LastErrorTime = time.Now()

	if account.ConsecutiveErrors >= maxConsecutiveErrors || isChannelError(err) {
		disableReason := "Too many consecutive errors"
		if isChannelError(err) {
			disableReason = "Bot removed from server/channel"
		}
		disableAccount(s, account, disableReason)
		return
	}

	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to update account error state")
	}

	notifyUserOfError(s, account, err)
}

func notifyUserOfError(s *discordgo.Session, account models.Account, err error) {
	canSend, checkErr := CheckNotificationCooldown(account.UserID, "error", time.Hour)
	if checkErr != nil {
		logger.Log.WithError(checkErr).Errorf("Failed to check error notification cooldown for user %s", account.UserID)
		return
	}
	if !canSend {
		logger.Log.Infof("Skipping error notification for user %s due to cooldown", account.UserID)
		return
	}

	NotifyAdminWithCooldown(s, fmt.Sprintf("Error checking account %s (ID: %d): %v", account.Title, account.ID, err), 5*time.Minute)

	if isCriticalError(err) {
		channel, err := s.UserChannelCreate(account.UserID)
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to create DM channel for user %s", account.UserID)
			return
		}

		embed := &discordgo.MessageEmbed{
			Title: "Critical Account Check Error",
			Description: fmt.Sprintf("There was a critical error checking your account '%s'. "+
				"The bot developer has been notified and will investigate the issue.", account.Title),
			Color:     0xFF0000,
			Timestamp: time.Now().Format(time.RFC3339),
		}

		if err := SendNotification(s, account, embed, "", "error"); err != nil {
			logger.Log.WithError(err).Error("Failed to send error notification")
		}

		_, err = s.ChannelMessageSendEmbed(channel.ID, embed)
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send critical error notification to user %s", account.UserID)
			return
		}

		if updateErr := UpdateNotificationTimestamp(account.UserID, "error"); updateErr != nil {
			logger.Log.WithError(updateErr).Errorf("Failed to update error notification timestamp for user %s", account.UserID)
		}
	}
}

func shouldCheckAccount(account models.Account, userSettings models.UserSettings, now time.Time) bool {
	if account.IsCheckDisabled {
		logger.Log.Debugf("Account %s is disabled, skipping check", account.Title)
		return false
	}

	if account.IsPermabanned {
		if account.LastNotification == 0 {
			return true
		}
		return false
	}

	checkInterval := time.Duration(userSettings.CheckInterval) * time.Minute
	return time.Unix(account.LastCheck, 0).Add(checkInterval).Before(now)
}

func shouldIncludeInDailyUpdate(account models.Account, userSettings models.UserSettings, now time.Time) bool {
	return time.Unix(account.LastNotification, 0).Add(time.Duration(userSettings.NotificationInterval) * time.Hour).Before(now)
}

func shouldCheckExpiration(account models.Account, now time.Time) bool {
	if account.IsExpiredCookie {
		return false
	}

	timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
	if err != nil {
		return false
	}

	return timeUntilExpiration > 0 && timeUntilExpiration <= time.Duration(cookieExpirationWarning)*time.Hour
}

func shouldDisableAccount(account models.Account, err error) bool {
	if account.ConsecutiveErrors >= maxConsecutiveErrors {
		return true
	}

	return strings.Contains(err.Error(), "Missing Access") ||
		strings.Contains(err.Error(), "Unknown Channel") ||
		strings.Contains(err.Error(), "insufficient balance") ||
		strings.Contains(err.Error(), "invalid captcha API key")
}

func getDisableReason(err error) string {
	switch {
	case strings.Contains(err.Error(), "Missing Access"):
		return "Bot removed from server/channel"
	case strings.Contains(err.Error(), "insufficient balance"):
		return "Insufficient captcha balance"
	case strings.Contains(err.Error(), "invalid captcha API key"):
		return "Invalid captcha API key"
	default:
		return fmt.Sprintf("Too many consecutive errors: %v", err)
	}
}

func validateUserCaptchaService(userID string, userSettings models.UserSettings) error {
	if !IsServiceEnabled(userSettings.PreferredCaptchaProvider) {
		return fmt.Errorf("captcha service %s is disabled", userSettings.PreferredCaptchaProvider)
	}

	if userSettings.EZCaptchaAPIKey != "" || userSettings.TwoCaptchaAPIKey != "" {
		_, balance, err := GetUserCaptchaKey(userID)
		if err != nil {
			return fmt.Errorf("failed to validate captcha key: %w", err)
		}
		if balance <= 0 {
			return fmt.Errorf("insufficient captcha balance: %.2f", balance)
		}
	}

	return nil

}
