package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/config"
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

	userSettings.EnsureMapsInitialized()

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

	userSettings.EnsureMapsInitialized()

	now := time.Now()
	if userSettings.LastActionTimes == nil {
		userSettings.LastActionTimes = make(map[string]time.Time)
	}
	if userSettings.ActionCounts == nil {
		userSettings.ActionCounts = make(map[string]int)
	}

	lastAction := userSettings.LastActionTimes[action]
	count := userSettings.ActionCounts[action]

	if now.Sub(lastAction) > duration {
		count = 0
	}

	if count >= getActionLimit(action) {
		return false
	}

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
		return 5
	case "notification":
		return 10
	default:
		return 3
	}
}

func processUserAccounts(s *discordgo.Session, userID string, accounts []models.Account) {
	cfg := config.Get()

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
	var accountsForDailyUpdate []models.Account

	now := time.Now()
	shouldSendDaily := time.Since(userSettings.LastDailyUpdateNotification) >=
		time.Duration(cfg.NotificationInterval)*time.Hour

	for _, account := range accounts {
		if !shouldCheckAccount(account, userSettings) {
			if !account.IsCheckDisabled && !account.IsExpiredCookie {
				accountsForDailyUpdate = append(accountsForDailyUpdate, account)
			}
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

		account.LastCheck = now.Unix()
		account.LastSuccessfulCheck = now
		account.ConsecutiveErrors = 0

		if hasStatusChanged(account, result) {
			account.LastStatus = result
			account.LastStatusChange = now.Unix()
			accountsToNotify = append(accountsToNotify, account)
		}

		accountsToUpdate = append(accountsToUpdate, account)
		accountsForDailyUpdate = append(accountsForDailyUpdate, account)
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

	if shouldSendDaily && len(accountsForDailyUpdate) > 0 {
		SendConsolidatedDailyUpdate(s, userID, userSettings, accountsForDailyUpdate)
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

func shouldCheckAccount(account models.Account, settings models.UserSettings) bool {
	cfg := config.Get()
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
		nextCheckTime = time.Unix(account.LastCheck, 0).Add(time.Duration(cfg.CookieCheckIntervalPermaban) * time.Hour)
	} else {
		checkInterval := settings.CheckInterval
		if checkInterval < 1 {
			checkInterval = cfg.CheckInterval
		}
		nextCheckTime = time.Unix(account.LastCheck, 0).Add(time.Duration(checkInterval) * time.Minute)
	}

	if account.ConsecutiveErrors > 0 && !account.LastErrorTime.IsZero() {
		errorCooldown := time.Hour * 1
		if time.Since(account.LastErrorTime) < errorCooldown {
			return false
		}
	}

	if settings.EZCaptchaAPIKey != "" || settings.TwoCaptchaAPIKey != "" {
		return now.After(nextCheckTime)
	}

	return now.After(nextCheckTime) && time.Since(time.Unix(account.LastCheck, 0)) >= cfg.DefaultRateLimit
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
