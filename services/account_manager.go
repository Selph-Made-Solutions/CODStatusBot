package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
)

func validateRateLimit(userID, action string, duration time.Duration) bool {
	var userSettings models.UserSettings
	if err := database.DB.Where("user_id = ?", userID).First(&userSettings).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching user settings")
		return false
	}

	now := time.Now()
	if userSettings.LastCommandTimes == nil {
		userSettings.LastCommandTimes = make(map[string]time.Time)
	}

	lastAction := userSettings.LastCommandTimes[action]

	config, exists := notificationConfigs[action]
	if !exists {
		config.Cooldown = duration
		config.MaxPerHour = 4
	}

	if !lastAction.IsZero() && now.Sub(lastAction) < config.Cooldown {
		return false
	}

	userSettings.LastCommandTimes[action] = now
	if err := database.DB.Save(&userSettings).Error; err != nil {
		logger.Log.WithError(err).Error("Error saving rate limit")
		return false
	}

	return true
}
func isChannelError(err error) bool {
	return strings.Contains(err.Error(), "Missing Access") ||
		strings.Contains(err.Error(), "Unknown Channel") ||
		strings.Contains(err.Error(), "Missing Permissions")
}

func checkActionRateLimit(userID, action string, duration time.Duration) bool {
	var userSettings models.UserSettings
	if err := database.DB.Where("user_id = ?", userID).First(&userSettings).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching user settings")
		return false
	}

	now := time.Now()
	if userSettings.LastActionTimes == nil {
		userSettings.LastActionTimes = make(map[string]time.Time)
	}
	if userSettings.ActionCounts == nil {
		userSettings.ActionCounts = make(map[string]int)
	}

	lastAction := userSettings.LastActionTimes[action]
	count := userSettings.ActionCounts[action]

	// Reset counts if outside the time window
	if now.Sub(lastAction) > duration {
		count = 0
	}

	// Check rate limits
	if count >= getActionLimit(action) {
		return false
	}

	// Update counts and times
	userSettings.LastActionTimes[action] = now
	userSettings.ActionCounts[action] = count + 1

	if err := database.DB.Save(&userSettings).Error; err != nil {
		logger.Log.WithError(err).Error("Error saving user settings")
		return false
	}

	return true
}

func getActionLimit(action string) int {
	switch action {
	case "check_account":
		return 5 // 5 checks per interval
	case "notification":
		return 10 // 10 notifications per interval
	default:
		return 3 // Default limit
	}
}

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

	var accountsToUpdate []models.Account
	var accountsToNotify []models.Account

	for _, account := range accounts {
		if !shouldCheckAccount(account, userSettings) {
			continue
		}

		if !checkActionRateLimit(userID, fmt.Sprintf("check_account_%d", account.ID), time.Hour) {
			logger.Log.Infof("Rate limit reached for account %s", account.Title)
			continue
		}

		result, err := CheckAccount(account.SSOCookie, userID, "")
		if err != nil {
			handleCheckError(s, &account, err)
			continue
		}

		now := time.Now()
		account.LastCheck = now.Unix()
		account.LastSuccessfulCheck = now
		account.ConsecutiveErrors = 0

		if hasStatusChanged(account, result) {
			account.LastStatus = result
			account.LastStatusChange = now.Unix()
			accountsToNotify = append(accountsToNotify, account)
		}

		accountsToUpdate = append(accountsToUpdate, account)
	}

	if len(accountsToUpdate) > 0 {
		DBMutex.Lock()
		if err := database.DB.Save(&accountsToUpdate).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to batch update accounts")
		}
		DBMutex.Unlock()
	}

	if len(accountsToNotify) > 0 {
		processNotifications(s, accountsToNotify, userSettings)
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

func shouldCheckAccount(account models.Account, settings models.UserSettings) bool {
	now := time.Now()

	if account.IsCheckDisabled {
		logger.Log.Debugf("Account %s is disabled, skipping check", account.Title)
		return false
	}

	if account.IsExpiredCookie {
		return false
	}

	var nextCheckTime time.Time
	if account.IsPermabanned {
		nextCheckTime = time.Unix(account.LastCheck, 0).Add(time.Duration(cookieCheckIntervalPermaban) * time.Hour)
	} else {
		checkInterval := settings.CheckInterval
		if checkInterval < 1 {
			checkInterval = GetEnvInt("CHECK_INTERVAL", 15)
		}
		nextCheckTime = time.Unix(account.LastCheck, 0).Add(time.Duration(checkInterval) * time.Minute)
	}

	if account.ConsecutiveErrors > 0 && !account.LastErrorTime.IsZero() {
		errorCooldown := time.Hour * 1
		if time.Since(account.LastErrorTime) < errorCooldown {
			return false
		}
	}

	return now.After(nextCheckTime)
}

func hasStatusChanged(account models.Account, newStatus models.Status) bool {
	if account.LastStatus == models.StatusUnknown {
		return true
	}
	return account.LastStatus != newStatus
}

func handleCheckError(s *discordgo.Session, account *models.Account, err error) {
	account.ConsecutiveErrors++
	account.LastErrorTime = time.Now()

	if account.ConsecutiveErrors >= maxConsecutiveErrors {
		reason := fmt.Sprintf("Max consecutive errors reached (%d). Last error: %v", maxConsecutiveErrors, err)
		disableAccount(s, *account, reason)
	}

	if err := database.DB.Save(account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to update account error status: %s", account.Title)
	}
}

func processNotifications(s *discordgo.Session, accounts []models.Account, userSettings models.UserSettings) {
	for _, account := range accounts {
		if !validateRateLimit(account.UserID, "notification", time.Hour) {
			logger.Log.Infof("Notification rate limit reached for user %s", account.UserID)
			continue
		}

		switch account.LastStatus {
		case models.StatusPermaban, models.StatusShadowban, models.StatusTempban:
			HandleStatusChange(s, account, account.LastStatus, userSettings)
		case models.StatusGood:
			if isComingFromBannedState(account) {
				HandleStatusChange(s, account, account.LastStatus, userSettings)
			}
		}
	}
}

func isComingFromBannedState(account models.Account) bool {
	bannedStates := []models.Status{
		models.StatusPermaban,
		models.StatusShadowban,
		models.StatusTempban,
	}

	for _, state := range bannedStates {
		if account.LastStatus == state {
			return true
		}
	}
	return false
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
