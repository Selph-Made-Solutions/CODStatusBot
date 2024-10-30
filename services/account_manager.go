package services

import (
	"fmt"
	"os"
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

/*
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		limits: make(map[string]time.Time),
		rates:  make(map[string]time.Duration),
	}
}
*/

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
func sendNotification(s *discordgo.Session, account models.Account, embed *discordgo.MessageEmbed, content, notificationType string) error {
	if account.IsCheckDisabled {
		return nil
	}

	userSettings, err := GetUserSettings(account.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user settings: %w", err)
	}

	config, ok := notificationConfigs[notificationType]
	if !ok {
		return fmt.Errorf("unknown notification type: %s", notificationType)
	}

	if !checkNotificationCooldown(account.UserID, notificationType, config.Cooldown) {
		return nil
	}

	channelID, err := getNotificationChannel(s, account, userSettings)
	if err != nil {
		return fmt.Errorf("failed to get notification channel: %w", err)
	}

	_, err = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed:   embed,
		Content: content,
	})

	if err != nil {
		if isChannelError(err) {
			handleChannelError(s, account, err)
			return err
		}
		return fmt.Errorf("failed to send notification: %w", err)
	}

	updateNotificationTimestamp(account.UserID, notificationType)
	return nil
}
*/

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

/*
func handleChannelError(s *discordgo.Session, account models.Account, err error) {
	logger.Log.WithError(err).Errorf("Channel error for account %s", account.Title)

	channel, err := s.UserChannelCreate(account.UserID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create DM channel")
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: "Channel Access Error",
		Description: fmt.Sprintf("The bot has lost access to the channel for account '%s'. "+
			"Please ensure the bot has proper permissions or set a new notification channel.",
			account.Title),
		Color:     0xFF0000,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	_, err = s.ChannelMessageSendEmbed(channel.ID, embed)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send DM about channel error")
	}
}
*/

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

/*
func processUserAccountBatch(s *discordgo.Session, userID string, accounts []models.Account) {
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

	var (
		accountsToUpdate    []models.Account
		dailyUpdateAccounts []models.Account
		expiringAccounts    []models.Account
		now                 = time.Now()
	)

	for _, account := range accounts {
		if shouldCheckAccount(account, userSettings, now) {
			accountsToUpdate = append(accountsToUpdate, account)
		}

		if shouldIncludeInDailyUpdate(account, userSettings, now) {
			dailyUpdateAccounts = append(dailyUpdateAccounts, account)
		}

		if shouldCheckExpiration(account, now) {
			expiringAccounts = append(expiringAccounts, account)
		}
	}

	var wg sync.WaitGroup
	errorChan := make(chan error, len(accountsToUpdate))

	for _, account := range accountsToUpdate {
		wg.Add(1)
		go func(acc models.Account) {
			defer wg.Done()

			checkSemaphore <- struct{}{}
			defer func() { <-checkSemaphore }()

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			logger.Log.WithFields(logrus.Fields{
				"account": acc.Title,
				"userId":  acc.UserID,
			}).Info("Starting account check")

			if err := processAccountCheck(ctx, s, acc, userSettings); err != nil {
				errorChan <- fmt.Errorf("error checking account %s: %w", acc.Title, err)
			}
		}(account)
	}

	wg.Wait()
	close(errorChan)

	for err := range errorChan {
		logger.Log.Error(err)
	}

	if len(dailyUpdateAccounts) > 0 {
		SendConsolidatedDailyUpdate(s, userID, userSettings, dailyUpdateAccounts)
	}

	if len(expiringAccounts) > 0 {
		NotifyCookieExpiringSoon(s, expiringAccounts)
	}
}
*/

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
				handleCheckFailure(s, account, err)
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

/*
func checkAccountWithContext(ctx context.Context, s *discordgo.Session, account models.Account, userSettings models.UserSettings) error {
	captchaAPIKey, err := getCaptchaKeyForUser(userSettings)
	if err != nil {
		return fmt.Errorf("failed to get captcha key: %w", err)
	}

	status, err := CheckAccountWithContext(ctx, account.SSOCookie, account.UserID, captchaAPIKey)
	if err != nil {
		return fmt.Errorf("failed to check account status: %w", err)
	}

	DBMutex.Lock()
	defer DBMutex.Unlock()

	previousStatus := account.LastStatus
	account.LastStatus = status
	account.LastCheck = time.Now().Unix()
	account.LastSuccessfulCheck = time.Now()
	account.ConsecutiveErrors = 0

	if err := database.DB.Save(&account).Error; err != nil {
		return fmt.Errorf("failed to update account state: %w", err)
	}

	if previousStatus != status {
		HandleStatusChange(s, account, status, userSettings)
	}

	return nil
}
*/

func handleCheckFailure(s *discordgo.Session, account models.Account, err error) {
	DBMutex.Lock()
	defer DBMutex.Unlock()

	account.ConsecutiveErrors++
	account.LastErrorTime = time.Now()

	if shouldDisableAccount(account, err) {
		disableAccount(s, account, getDisableReason(err))
		return
	}

	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to update account error state")
	}

	notifyUserOfError(s, account, err)
}

func notifyUserOfError(s *discordgo.Session, account models.Account, err error) {
	if !checkNotificationCooldown(account.UserID, "error", errorCooldownPeriod) {
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Check Error", account.Title),
		Description: fmt.Sprintf("An error occurred while checking your account: %v", err),
		Color:       0xFF0000,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Consecutive Errors",
				Value:  fmt.Sprintf("%d", account.ConsecutiveErrors),
				Inline: true,
			},
			{
				Name:   "Last Successful Check",
				Value:  account.LastSuccessfulCheck.Format(time.RFC1123),
				Inline: true,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	userSettings, err := GetUserSettings(account.UserID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get user settings for error notification")
		return
	}

	channelID, err := getNotificationChannel(s, account, userSettings)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get notification channel")
		return
	}

	_, err = s.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send error notification")
	}

	updateNotificationTimestamp(account.UserID, "error")
}

func shouldCheckAccount(account models.Account, userSettings models.UserSettings, now time.Time) bool {
	if account.IsCheckDisabled {
		logger.Log.Debugf("Account %s is disabled, skipping check", account.Title)
		return false
	}

	if account.IsPermabanned {
		nextCheck := time.Unix(account.LastCookieCheck, 0).Add(time.Duration(cookieCheckIntervalPermaban) * time.Hour)
		if nextCheck.After(now) {
			logger.Log.Debugf("Account %s is permabanned, skipping check until %s", account.Title, nextCheck)
			return false
		}
		// Only check permabanned accounts for cookie validity
		return true
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

func getCaptchaKeyForUser(userSettings models.UserSettings) (string, error) {
	switch userSettings.PreferredCaptchaProvider {
	case "ezcaptcha":
		if userSettings.EZCaptchaAPIKey != "" {
			return userSettings.EZCaptchaAPIKey, nil
		}
		return os.Getenv("EZCAPTCHA_CLIENT_KEY"), nil
	case "2captcha":
		if userSettings.TwoCaptchaAPIKey != "" {
			return userSettings.TwoCaptchaAPIKey, nil
		}
		return "", fmt.Errorf("no 2captcha API key available")
	default:
		return "", fmt.Errorf("unsupported captcha provider: %s", userSettings.PreferredCaptchaProvider)
	}
}
