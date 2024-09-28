package services

import (
	"CODStatusBot/admin"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxConsecutiveErrors          = 5
	maxUserErrorNotifications     = 3
	userErrorNotificationCooldown = 24 * time.Hour
	balanceNotificationThreshold  = 1000           // Notify when balance is below this value
	balanceNotificationInterval   = 24 * time.Hour // How often to notify about low balance
)

type NotificationConfig struct {
	Type              string
	Cooldown          time.Duration
	AllowConsolidated bool
}

var (
	userErrorNotifications      = make(map[string][]time.Time)
	userErrorNotificationMutex  sync.Mutex
	checkInterval               float64 // Check interval for accounts (in minutes).
	notificationInterval        float64 // Notification interval for daily updates (in hours).
	cooldownDuration            float64 // Cooldown duration for invalid cookie notifications (in hours).
	sleepDuration               int     // Sleep duration for the account checking loop (in minutes).
	cookieCheckIntervalPermaban float64 // Check interval for permabanned accounts (in hours).
	statusChangeCooldown        float64 // Cooldown duration for status change notifications (in hours).
	globalNotificationCooldown  float64 // Global cooldown for notifications per user (in hours).
	cookieExpirationWarning     float64 // Time before cookie expiration to send a warning (in hours).
	tempBanUpdateInterval       float64 // Interval for temporary ban update notifications (in hours).
	userNotificationMutex       sync.Mutex
	userNotificationTimestamps  = make(map[string]map[string]time.Time)
	DBMutex                     sync.Mutex
	defaultRateLimit            time.Duration // Default rate limit for checks (in minutes).
	checkNowRateLimit           time.Duration // Rate limit for the check now command (in seconds).

)

func init() {
	err := godotenv.Load()
	if err != nil {
		logger.Log.WithError(err).Error("Failed to load .env file")
	}

	checkInterval = getEnvFloat("CHECK_INTERVAL", 15)
	notificationInterval = getEnvFloat("NOTIFICATION_INTERVAL", 24)
	cooldownDuration = getEnvFloat("COOLDOWN_DURATION", 6)
	sleepDuration = getEnvInt("SLEEP_DURATION", 1)
	cookieCheckIntervalPermaban = getEnvFloat("COOKIE_CHECK_INTERVAL_PERMABAN", 24)
	statusChangeCooldown = getEnvFloat("STATUS_CHANGE_COOLDOWN", 1)
	globalNotificationCooldown = getEnvFloat("GLOBAL_NOTIFICATION_COOLDOWN", 2)
	cookieExpirationWarning = getEnvFloat("COOKIE_EXPIRATION_WARNING", 24)
	tempBanUpdateInterval = getEnvFloat("TEMP_BAN_UPDATE_INTERVAL", 24)
	defaultRateLimit = time.Duration(getEnvInt("DEFAULT_RATE_LIMIT", 5)) * time.Minute
	checkNowRateLimit = time.Duration(getEnvInt("CHECK_NOW_RATE_LIMIT", 3600)) * time.Second

	defaultSettings.NotificationInterval = notificationInterval

	logger.Log.Infof("Loaded config: CHECK_INTERVAL=%.2f, NOTIFICATION_INTERVAL=%.2f, COOLDOWN_DURATION=%.2f, SLEEP_DURATION=%d, COOKIE_CHECK_INTERVAL_PERMABAN=%.2f, STATUS_CHANGE_COOLDOWN=%.2f, GLOBAL_NOTIFICATION_COOLDOWN=%.2f, COOKIE_EXPIRATION_WARNING=%.2f, TEMP_BAN_UPDATE_INTERVAL=%.2f, CHECK_NOW_RATE_LIMIT=%v, DEFAULT_RATE_LIMIT=%v",
		checkInterval, notificationInterval, cooldownDuration, sleepDuration, cookieCheckIntervalPermaban, statusChangeCooldown, globalNotificationCooldown, cookieExpirationWarning, tempBanUpdateInterval, checkNowRateLimit, defaultRateLimit)
}

var notificationConfigs = map[string]NotificationConfig{
	"status_change":        {Cooldown: time.Hour, AllowConsolidated: false},
	"permaban":             {Cooldown: 24 * time.Hour, AllowConsolidated: false},
	"daily_update":         {Cooldown: 0, AllowConsolidated: true},
	"invalid_cookie":       {Cooldown: 6 * time.Hour, AllowConsolidated: true},
	"cookie_expiring_soon": {Cooldown: 24 * time.Hour, AllowConsolidated: true},
	"temp_ban_update":      {Cooldown: time.Hour, AllowConsolidated: false},
}

func getEnvFloat(key string, fallback float64) float64 {
	value := getEnvFloatRaw(key, fallback)
	// Convert hours to minutes for certain settings
	if key == "CHECK_INTERVAL" || key == "SLEEP_DURATION" || key == "DEFAULT_RATE_LIMIT" {
		return value
	}
	// All other values are in hours, so we don't need to convert them
	return value
}
func getEnvFloatRaw(key string, fallback float64) float64 {
	if value, ok := os.LookupEnv(key); ok {
		floatValue, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return floatValue
		}
		logger.Log.WithError(err).Errorf("Failed to parse %s, using fallback value", key)
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := getEnvIntRaw(key, fallback)
	// All int values are currently in minutes, so we don't need to convert them
	return value
}
func getEnvIntRaw(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		intValue, err := strconv.Atoi(value)
		if err == nil {
			return intValue
		}
		logger.Log.WithError(err).Errorf("Failed to parse %s, using fallback value", key)
	}
	return fallback
}

func sendNotification(discord *discordgo.Session, account models.Account, embed *discordgo.MessageEmbed, content string, notificationType string) error {
	if account.IsCheckDisabled {
		logger.Log.Infof("Skipping notification for disabled account %s", account.Title)
		return nil
	}

	var userSettings models.UserSettings
	if err := database.DB.Where("user_id = ?", account.UserID).First(&userSettings).Error; err != nil {
		return fmt.Errorf("failed to get user settings: %w", err)
	}

	config, ok := notificationConfigs[notificationType]
	if !ok {
		return fmt.Errorf("unknown notification type: %s", notificationType)
	}

	if config.Cooldown == 0 {
		config.Cooldown = time.Duration(userSettings.NotificationInterval) * time.Hour
	}

	userNotificationMutex.Lock()
	if _, exists := userNotificationTimestamps[account.UserID]; !exists {
		userNotificationTimestamps[account.UserID] = make(map[string]time.Time)
	}
	lastNotification, exists := userNotificationTimestamps[account.UserID][notificationType]
	shouldSend := !exists || time.Since(lastNotification) >= config.Cooldown
	if shouldSend {
		userNotificationTimestamps[account.UserID][notificationType] = time.Now()
	}
	userNotificationMutex.Unlock()
	shouldSend = true
	var cooldownDuration time.Duration

	switch notificationType {
	case "status_change":
		// Always send status change notifications
		shouldSend = true
	case "permaban":
		if exists {
			cooldownDuration = 24 * time.Hour // Set 24-hour cooldown for permaban reminders.
			shouldSend = time.Since(lastNotification) >= cooldownDuration
		}
	case "daily_update", "invalid_cookie", "cookie_expiring_soon":
		cooldownDuration = time.Duration(userSettings.NotificationInterval) * time.Hour
		shouldSend = !exists || time.Since(lastNotification) >= cooldownDuration
	case "temp_ban_update":
		cooldownDuration = 1 * time.Hour // Set a reasonable cooldown for temp ban updates
		shouldSend = !exists || time.Since(lastNotification) >= cooldownDuration
	default:
		cooldownDuration = time.Duration(globalNotificationCooldown) * time.Hour
		shouldSend = !exists || time.Since(lastNotification) >= cooldownDuration
	}

	if !shouldSend {
		logger.Log.Infof("Skipping %s notification for user %s (cooldown)", notificationType, account.UserID)
		return nil
	}

	var channelID string
	if userSettings.NotificationType == "dm" {
		channel, err := discord.UserChannelCreate(account.UserID)
		if err != nil {
			return fmt.Errorf("failed to create DM channel: %w", err)
		}
		channelID = channel.ID
	} else {
		channelID = account.ChannelID
	}

	_, err := discord.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed:   embed,
		Content: content,
	})
	if err != nil {
		if strings.Contains(err.Error(), "Missing Access") || strings.Contains(err.Error(), "Unknown Channel") {
			logger.Log.Warnf("Bot might have been removed from the channel or server for user %s", account.UserID)
			// Consider updating the account status or notifying the user through alternative means
			return fmt.Errorf("bot might have been removed: %w", err)
		}
		return fmt.Errorf("failed to send notification: %w", err)
	}

	logger.Log.Infof("%s notification sent to user %s", notificationType, account.UserID)
	userNotificationMutex.Lock()
	userNotificationTimestamps[account.UserID][notificationType] = time.Now()
	userNotificationMutex.Unlock()
	return nil
}

func sendIndividualDailyUpdate(account models.Account, discord *discordgo.Session) {
	now := time.Now()
	if now.Sub(account.Last24HourNotification) < 24*time.Hour {
		logger.Log.Infof("Skipping daily update for account %s as last notification was less than 24 hours ago", account.Title)
		return
	}

	logger.Log.Infof("Sending individual daily update for account %s", account.Title)

	var description string
	if account.IsExpiredCookie {
		description = fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title)
	} else {
		timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
		if err != nil {
			logger.Log.WithError(err).Errorf("Error checking SSO cookie expiration for account %s", account.Title)
			description = fmt.Sprintf("An error occurred while checking the SSO cookie expiration for account %s. Please check the account status manually.", account.Title)
		} else if timeUntilExpiration > 0 {
			description = fmt.Sprintf("The last status of account %s was %s. SSO cookie will expire in %s.", account.Title, account.LastStatus, FormatExpirationTime(account.SSOCookieExpiration))
		} else {
			description = fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title)
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%.2f Hour Update - %s", notificationInterval, account.Title),
		Description: description,
		Color:       GetColorForStatus(account.LastStatus, account.IsExpiredCookie, account.IsCheckDisabled),
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	err := sendNotification(discord, account, embed, "", "daily_update")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send individual daily update message for account %s", account.Title)
	} else {
		account.LastCheck = time.Now().Unix()
		account.LastNotification = time.Now().Unix()
		if err := database.DB.Save(&account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
		}
	}

	account.Last24HourNotification = now
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to update Last24HourNotification for account %s", account.Title)
	}
}

func CheckAccounts(s *discordgo.Session) {
	cleanupTicker := time.NewTicker(24 * time.Hour)
	defer cleanupTicker.Stop()

	for {
		logger.Log.Info("Starting periodic account check")
		var accounts []models.Account
		if err := database.DB.Find(&accounts).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to fetch accounts from the database")
			time.Sleep(time.Duration(sleepDuration) * time.Minute)
			continue
		}

		accountsByUser := make(map[string][]models.Account)
		for _, account := range accounts {
			accountsByUser[account.UserID] = append(accountsByUser[account.UserID], account)
		}

		for userID, userAccounts := range accountsByUser {
			go processUserAccounts(s, userID, userAccounts)
		}

		select {
		case <-cleanupTicker.C:
			go cleanupInactiveAccounts(s)
		default:

		}

		time.Sleep(time.Duration(sleepDuration) * time.Minute)
	}
}

func processUserAccounts(s *discordgo.Session, userID string, accounts []models.Account) {
	captchaAPIKey, balance, err := GetUserCaptchaKey(userID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to get user captcha key for user %s", userID)
		return
	}

	logger.Log.Infof("User %s captcha balance: %.2f", userID, balance)

	if captchaAPIKey != "" {
		checkAndNotifyBalance(s, userID, balance)
	}

	var accountsToUpdate []models.Account
	var dailyUpdateAccounts []models.Account

	for _, account := range accounts {
		if account.IsCheckDisabled {
			logger.Log.Infof("Skipping check for disabled account: %s", account.Title)
			continue
		}

		if account.IsPermabanned {
			// Use the appropriate notification interval
			notificationInterval := defaultSettings.NotificationInterval
			if captchaAPIKey != "" {
				userSettings, err := GetUserSettings(userID)
				if err != nil {
					logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", userID)
					continue
				}
				notificationInterval = userSettings.NotificationInterval
			}

			lastNotification := time.Unix(account.LastNotification, 0)
			if time.Since(lastNotification).Hours() >= notificationInterval {
				handlePermabannedAccount(account, s)
			}
			continue
		}

		checkInterval := defaultSettings.CheckInterval
		if captchaAPIKey != "" {
			userSettings, err := GetUserSettings(userID)
			if err != nil {
				logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", userID)
				continue
			}
			checkInterval = userSettings.CheckInterval
		}

		lastCheck := time.Unix(account.LastCheck, 0)
		if time.Since(lastCheck).Minutes() < float64(checkInterval) {
			logger.Log.Infof("Skipping check for account %s (rate limit)", account.Title)
			continue
		}

		accountsToUpdate = append(accountsToUpdate, account)

		if time.Since(time.Unix(account.LastNotification, 0)).Hours() > notificationInterval {
			dailyUpdateAccounts = append(dailyUpdateAccounts, account)
		}
	}

	// Check and update accounts
	for _, account := range accountsToUpdate {
		go CheckSingleAccount(account, s)
	}

	// Send consolidated daily update if needed
	if len(dailyUpdateAccounts) > 0 {
		go sendConsolidatedDailyUpdate(dailyUpdateAccounts, s)
	}
}

func checkAndNotifyBalance(s *discordgo.Session, userID string, balance float64) {
	userSettings, err := GetUserSettings(userID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", userID)
		return
	}

	now := time.Now()
	if balance < balanceNotificationThreshold &&
		(userSettings.LastBalanceNotification.IsZero() ||
			now.Sub(userSettings.LastBalanceNotification) >= balanceNotificationInterval) {

		embed := &discordgo.MessageEmbed{
			Title:       "Low EZ-Captcha Balance Alert",
			Description: fmt.Sprintf("Your EZ-Captcha balance is currently %.2f points, which is below the recommended threshold of %d points.", balance, balanceNotificationThreshold),
			Color:       0xFFA500, // Orange color for warning
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Action Required",
					Value:  "Please recharge your EZ-Captcha balance to ensure uninterrupted service for your account checks.",
					Inline: false,
				},
			},
			Timestamp: now.Format(time.RFC3339),
		}

		// Use the first account's notification settings for this user
		var account models.Account
		if err := database.DB.Where("user_id = ?", userID).First(&account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to get an account for user %s", userID)
			return
		}

		err = sendNotification(s, account, embed, "", "balance_warning")
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send balance notification to user %s", userID)
			return
		}

		// Update last balance notification timestamp
		userSettings.LastBalanceNotification = now
		if err := database.DB.Save(&userSettings).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to update LastBalanceNotification for user %s", userID)
		}
	}
}

// Updated handlePermabannedAccount function
func handlePermabannedAccount(account models.Account, s *discordgo.Session) {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Permanent Ban Status", account.Title),
		Description: fmt.Sprintf("The account %s is still permanently banned. Please remove this account from monitoring using the /removeaccount command.", account.Title),
		Color:       GetColorForStatus(models.StatusPermaban, false, account.IsCheckDisabled),
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	err := sendNotification(s, account, embed, fmt.Sprintf("<@%s>", account.UserID), "permaban")
	if err != nil {
		if strings.Contains(err.Error(), "bot might have been removed") {
			logger.Log.Warnf("Bot removed for account %s. Considering account inactive.", account.Title)
			account.IsCheckDisabled = true
			if err := database.DB.Save(&account).Error; err != nil {
				logger.Log.WithError(err).Errorf("Failed to update account status for %s", account.Title)
			}
		} else {
			logger.Log.WithError(err).Errorf("Failed to send permaban update for account %s", account.Title)
		}
	} else {
		account.LastNotification = time.Now().Unix()
		if err := database.DB.Save(&account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to update LastNotification for account %s", account.Title)
		}
	}
}

// Handle accounts with expired cookies
func handleExpiredCookieAccount(account models.Account, s *discordgo.Session) {
	logger.Log.WithField("account", account.Title).Info("Processing account with expired cookie")

	userSettings, err := GetUserSettings(account.UserID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get user settings, using default notification interval")
		userSettings = defaultSettings
	}

	if time.Since(time.Unix(account.LastNotification, 0)).Hours() > userSettings.NotificationInterval {
		go sendIndividualDailyUpdate(account, s)
	} else {
		logger.Log.WithField("account", account.Title).Infof("Owner of %s recently notified within %.2f hours already, skipping", account.Title, userSettings.NotificationInterval)
	}
}

// CheckSingleAccount function: checks the status of a single account
func CheckSingleAccount(account models.Account, discord *discordgo.Session) {
	logger.Log.Infof("Checking account: %s", account.Title)

	if account.IsCheckDisabled {
		logger.Log.Infof("Account %s is disabled. Reason: %s", account.Title, account.DisabledReason)
		return
	}

	timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check SSO cookie expiration for account %s", account.Title)
		handleCheckAccountError(account, discord, err)
		return
	} else if timeUntilExpiration > 0 && timeUntilExpiration <= 24*time.Hour {
		notifyCookieExpiringSoon(account, discord, timeUntilExpiration)
	}

	// Remove the unused userSettings variable
	// userSettings, err := GetUserSettings(account.UserID)
	// if err != nil {
	// 	logger.Log.WithError(err).Errorf("Failed to get user settings for account %s", account.Title)
	// 	return
	// }

	result, err := CheckAccount(account.SSOCookie, account.UserID)
	if err != nil {
		handleCheckAccountError(account, discord, err)
		return
	}

	// Reset consecutive errors if check is successful
	account.ConsecutiveErrors = 0
	account.LastSuccessfulCheck = time.Now()

	updateAccountStatus(account, result, discord)
}

func handleCheckAccountError(account models.Account, discord *discordgo.Session, err error) {
	account.ConsecutiveErrors++
	account.LastErrorTime = time.Now()

	switch {
	case strings.Contains(err.Error(), "Missing Access") || strings.Contains(err.Error(), "Unknown Channel"):
		disableAccount(account, discord, "Bot removed from server/channel")
	case strings.Contains(err.Error(), "insufficient balance"):
		disableAccount(account, discord, "Insufficient captcha balance")
	case strings.Contains(err.Error(), "invalid captcha API key"):
		disableAccount(account, discord, "Invalid captcha API key")
	default:
		if account.ConsecutiveErrors >= maxConsecutiveErrors {
			disableAccount(account, discord, fmt.Sprintf("Too many consecutive errors: %v", err))
		} else {
			logger.Log.WithError(err).Errorf("Failed to check account %s: possible expired SSO Cookie", account.Title)
			notifyUserOfCheckError(account, discord, err)
		}
	}

	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to update account %s after error", account.Title)
	}
}

func disableAccount(account models.Account, discord *discordgo.Session, reason string) {
	account.IsCheckDisabled = true
	account.DisabledReason = reason

	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to disable account %s", account.Title)
		return
	}

	logger.Log.Infof("Account %s has been disabled. Reason: %s", account.Title, reason)

	// Attempt to notify user via DM
	notifyUserAboutDisabledAccount(discord, account, reason)
}

func notifyCookieExpiringSoon(account models.Account, discord *discordgo.Session, timeUntilExpiration time.Duration) {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - SSO Cookie Expiring Soon", account.Title),
		Description: fmt.Sprintf("The SSO cookie for account %s will expire in %s. Please update the cookie soon using the /updateaccount command.", account.Title, FormatExpirationTime(account.SSOCookieExpiration)),
		Color:       0xFFA500, // Orange color for warning
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	err := sendNotification(discord, account, embed, "", "cookie_expiring_soon")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send SSO cookie expiration notification for account %s", account.Title)
	}
}

func handleInvalidCookie(account models.Account, discord *discordgo.Session) {
	userSettings, _ := GetUserSettings(account.UserID)
	lastNotification := time.Unix(account.LastCookieNotification, 0)
	if time.Since(lastNotification).Hours() >= userSettings.CooldownDuration || account.LastCookieNotification == 0 {
		logger.Log.Infof("Account %s has an invalid SSO cookie", account.Title)
		embed := &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Invalid SSO Cookie", account.Title),
			Description: fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title),
			Color:       0xff9900,
			Timestamp:   time.Now().Format(time.RFC3339),
		}

		err := sendNotification(discord, account, embed, "", "invalid_cookie")
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send invalid cookie notification for account %s", account.Title)
		}

		// Update account information regarding the expired cookie
		DBMutex.Lock()
		account.LastCookieNotification = time.Now().Unix()
		account.IsExpiredCookie = true
		if err := database.DB.Save(&account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
		}
		DBMutex.Unlock()
	} else {
		logger.Log.Infof("Skipping expired cookie notification for account %s (cooldown)", account.Title)
	}
}

// Update account information
func updateAccountStatus(account models.Account, result models.Status, discord *discordgo.Session) {
	DBMutex.Lock()
	lastStatus := account.LastStatus
	account.LastCheck = time.Now().Unix()
	account.IsExpiredCookie = false
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
		DBMutex.Unlock()
		return
	}
	DBMutex.Unlock()

	// Handle status changes and send notifications
	if result != lastStatus {
		handleStatusChange(account, result, discord)
	}
}

// Update the handleStatusChange function
func handleStatusChange(account models.Account, newStatus models.Status, discord *discordgo.Session) {
	DBMutex.Lock()
	account.LastStatus = newStatus
	account.LastStatusChange = time.Now().Unix()
	account.IsPermabanned = newStatus == models.StatusPermaban
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
		DBMutex.Unlock()
		return
	}
	logger.Log.Infof("Account %s status changed to %s", account.Title, newStatus)

	// Create a record for the account.
	ban := models.Ban{
		Account:   account,
		Status:    newStatus,
		AccountID: account.ID,
	}

	if newStatus == models.StatusTempban {
		ban.TempBanDuration = calculateBanDuration(time.Now())
	}

	if err := database.DB.Create(&ban).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to create new ban record for account %s", account.Title)
	}
	DBMutex.Unlock()

	// Create an embed message for the status change notification
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - %s", account.Title, EmbedTitleFromStatus(newStatus)),
		Description: getStatusDescription(newStatus, account.Title, ban),
		Color:       GetColorForStatus(newStatus, account.IsExpiredCookie, account.IsCheckDisabled),
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	notificationType := "status_change"
	if newStatus == models.StatusPermaban {
		notificationType = "permaban"
	}

	err := sendNotification(discord, account, embed, fmt.Sprintf("<@%s>", account.UserID), notificationType)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send status update message for account %s", account.Title)
	}
	if newStatus == models.StatusTempban {
		go scheduleTempBanNotification(account, ban.TempBanDuration, discord)
	}
}

func scheduleTempBanNotification(account models.Account, duration string, discord *discordgo.Session) {
	// Parse the duration string (assuming it is in the format "X days, Y hours").
	parts := strings.Split(duration, ",")
	if len(parts) != 2 {
		logger.Log.Errorf("Invalid duration format for account %s: %s", account.Title, duration)
		return
	}
	days, _ := strconv.Atoi(strings.TrimSpace(strings.Split(parts[0], " ")[0]))
	hours, _ := strconv.Atoi(strings.TrimSpace(strings.Split(parts[1], " ")[0]))

	sleepDuration := time.Duration(days)*24*time.Hour + time.Duration(hours)*time.Hour

	// Send intermediate notifications
	for remainingTime := sleepDuration; remainingTime > 0; remainingTime -= 24 * time.Hour {
		if remainingTime > 24*time.Hour {
			time.Sleep(24 * time.Hour)
		} else {
			time.Sleep(remainingTime)
		}

		embed := &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Update", account.Title),
			Description: fmt.Sprintf("Your account is still temporarily banned. Remaining time: %v", remainingTime),
			Color:       GetColorForStatus(models.StatusTempban, false, account.IsCheckDisabled),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
		err := sendNotification(discord, account, embed, "", "temp_ban_update")
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send temporary ban update for account %s", account.Title)
		}
	}

	// Check the account status after the temporary ban duration
	result, err := CheckAccount(account.SSOCookie, account.UserID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check account %s after temporary ban duration", account.Title)
		return
	}

	var embed *discordgo.MessageEmbed
	if result == models.StatusGood {
		// If the status has returned to good, send a notification stating this.
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Lifted", account.Title),
			Description: fmt.Sprintf("The temporary ban for account %s has been lifted. The account is now in good standing.", account.Title),
			Color:       GetColorForStatus(result, false, account.IsCheckDisabled),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	} else if result == models.StatusPermaban {
		// If the status is now permaban, send a notification stating this.
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Escalated", account.Title),
			Description: fmt.Sprintf("The temporary ban for account %s has been escalated to a permanent ban.", account.Title),
			Color:       GetColorForStatus(result, false, account.IsCheckDisabled),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	} else {
		// If the status is still temporary ban or any other status, send a notification stating this.
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Update", account.Title),
			Description: fmt.Sprintf("The temporary ban for account %s is still in effect. Current status: %s", account.Title, result),
			Color:       GetColorForStatus(result, false, account.IsCheckDisabled),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	}

	err = sendNotification(discord, account, embed, fmt.Sprintf("<@%s>", account.UserID), "temp_ban_update")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send temporary ban update message for account %s", account.Title)
	}
}

// GetColorForStatus function: returns the appropriate color for an embed message based on the account status.
func GetColorForStatus(status models.Status, isExpiredCookie bool, isCheckDisabled bool) int {
	if isCheckDisabled {
		return 0xA9A9A9 // Dark Gray for disabled checks
	}
	if isExpiredCookie {
		return 0xFF6347 // Tomato for expired cookie
	}
	switch status {
	case models.StatusPermaban:
		return 0x8B0000 // Dark Red for permanent ban
	case models.StatusShadowban:
		return 0xFFD700 // Gold for shadowban
	case models.StatusTempban:
		return 0xFF8C00 // Dark Orange for temporary ban
	case models.StatusGood:
		return 0x32CD32 // Lime Green for good status
	default:
		return 0x708090 // Slate Gray for unknown status
	}
}

func EmbedTitleFromStatus(status models.Status) string {
	switch status {
	case models.StatusTempban:
		return "TEMPORARY BAN DETECTED"
	case models.StatusPermaban:
		return "PERMANENT BAN DETECTED"
	case models.StatusShadowban:
		return "SHADOWBAN DETECTED"
	default:
		return "ACCOUNT NOT BANNED"
	}
}

func getStatusDescription(status models.Status, accountTitle string, ban models.Ban) string {
	affectedGames := strings.Split(ban.AffectedGames, ",")
	gamesList := strings.Join(affectedGames, ", ")

	switch status {
	case models.StatusPermaban:
		return fmt.Sprintf("The account %s has been permanently banned.\nAffected games: %s", accountTitle, gamesList)
	case models.StatusShadowban:
		return fmt.Sprintf("The account %s is currently shadowbanned.\nAffected games: %s", accountTitle, gamesList)
	case models.StatusTempban:
		return fmt.Sprintf("The account %s is temporarily banned for %s.\nAffected games: %s", accountTitle, ban.TempBanDuration, gamesList)
	default:
		return fmt.Sprintf("The account %s is currently not banned.", accountTitle)
	}
}

// SendGlobalAnnouncement function: sends a global announcement to users who haven't seen it yet.
func SendGlobalAnnouncement(s *discordgo.Session, userID string) error {
	var userSettings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&userSettings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings for global announcement")
		return result.Error
	}

	if !userSettings.HasSeenAnnouncement {
		channelID, err := getChannelForAnnouncement(s, userID, userSettings)
		if err != nil {
			logger.Log.WithError(err).Error("Error finding recent channel for user")
			return err
		}

		announcementEmbed := createAnnouncementEmbed()

		_, err = s.ChannelMessageSendEmbed(channelID, announcementEmbed)
		if err != nil {
			logger.Log.WithError(err).Error("Error sending global announcement")
			return err
		}

		userSettings.HasSeenAnnouncement = true
		if err := database.DB.Save(&userSettings).Error; err != nil {
			logger.Log.WithError(err).Error("Error updating user settings after sending global announcement")
			return err
		}
	}

	return nil
}

func getChannelForAnnouncement(s *discordgo.Session, userID string, userSettings models.UserSettings) (string, error) {
	if userSettings.NotificationType == "dm" {
		channel, err := s.UserChannelCreate(userID)
		if err != nil {
			logger.Log.WithError(err).Error("Error creating DM channel for global announcement")
			return "", err
		}
		return channel.ID, nil
	}

	var account models.Account
	if err := database.DB.Where("user_id = ?", userID).Order("updated_at DESC").First(&account).Error; err != nil {
		logger.Log.WithError(err).Error("Error finding recent channel for user")
		return "", err
	}
	return account.ChannelID, nil
}

func SendAnnouncementToAllUsers(s *discordgo.Session) error {
	var users []models.UserSettings
	if err := database.DB.Find(&users).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching all users")
		return err
	}

	for _, user := range users {
		if err := SendGlobalAnnouncement(s, user.UserID); err != nil {
			logger.Log.WithError(err).Errorf("Failed to send announcement to user %s", user.UserID)
		}
	}

	return nil
}

func createAnnouncementEmbed() *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: "Important Announcement: Changes to COD Status Bot",
		Description: "Due to the high demand and usage of our bot, we've reached the limit of our free EZCaptcha tokens. " +
			"To continue using the check ban feature, users now need to provide their own EZCaptcha API key.\n\n" +
			"Here's what you need to know:",
		Color: 0xFFD700, // Gold color
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "How to Get Your Own API Key",
				Value: "1. Visit our [referral link](https://dashboard.ez-captcha.com/#/register?inviteCode=uyNrRgWlEKy) to sign up for EZCaptcha\n" +
					"2. Request a free trial of 10,000 tokens\n" +
					"3. Use the `/setcaptchaservice` command to set your API key in the bot",
			},
			{
				Name: "Benefits of Using Your Own API Key",
				Value: "• Continue using the check ban feature\n" +
					"• Customize your check intervals\n" +
					"• Support the bot indirectly through our referral program",
			},
			{
				Name:  "Our Commitment",
				Value: "We're working on ways to maintain a free tier for all users. Your support by using our referral link helps us achieve this goal.",
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Thank you for your understanding and continued support!",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

func calculateBanDuration(banStartTime time.Time) string {
	duration := time.Since(banStartTime)
	days := int(duration.Hours() / 24)
	hours := int(duration.Hours()) % 24
	return fmt.Sprintf("%d days, %d hours", days, hours)
}

// Update the sendConsolidatedDailyUpdate function
func sendConsolidatedDailyUpdate(accounts []models.Account, discord *discordgo.Session) {
	if len(accounts) == 0 {
		return
	}

	logger.Log.Infof("Sending consolidated daily update for %d accounts", len(accounts))

	var embedFields []*discordgo.MessageEmbedField

	for _, account := range accounts {
		var description string
		if account.IsExpiredCookie {
			description = "SSO cookie has expired. Please update using /updateaccount command."
		} else {
			timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
			if err != nil {
				description = "Error checking SSO cookie expiration. Please check manually."
			} else if timeUntilExpiration > 0 {
				description = fmt.Sprintf("Status: %s. Cookie expires in %s.", account.LastStatus, FormatExpirationTime(account.SSOCookieExpiration))
			} else {
				description = "SSO cookie has expired. Please update using /updateaccount command."
			}
		}

		embedFields = append(embedFields, &discordgo.MessageEmbedField{
			Name:   account.Title,
			Value:  description,
			Inline: false,
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%.2f Hour Update - Multiple Accounts", notificationInterval),
		Description: "Here's an update on your monitored accounts:",
		Color:       0x00ff00, // Green color
		Fields:      embedFields,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	// Send the consolidated notification
	err := sendNotification(discord, accounts[0], embed, "", "daily_update")
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send consolidated daily update")
	} else {
		for _, account := range accounts {
			account.LastCheck = time.Now().Unix()
			account.LastNotification = time.Now().Unix()
			if err := database.DB.Save(&account).Error; err != nil {
				logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
			}
		}
	}
}

func cleanupInactiveAccounts(s *discordgo.Session) {
	logger.Log.Info("Starting cleanup of inactive accounts")

	var accounts []models.Account
	if err := database.DB.Find(&accounts).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to fetch accounts for cleanup")
		return
	}

	for _, account := range accounts {
		if err := checkAccountAccess(s, &account); err != nil {
			logger.Log.WithError(err).Warnf("Lost access to account %s for user %s", account.Title, account.UserID)

			account.IsCheckDisabled = true
			account.DisabledReason = "Bot removed from server/channel"

			if err := database.DB.Save(&account).Error; err != nil {
				logger.Log.WithError(err).Errorf("Failed to update account %s status during cleanup", account.Title)
				continue
			}

			// Notify developer about the disabled account
			admin.NotifyAdmin(fmt.Sprintf("Account disabled during cleanup: %s (ID: %d, User: %s)", account.Title, account.ID, account.UserID))

			// Attempt to notify the user via DM about the account being disabled
			notifyUserAboutDisabledAccount(s, account, account.DisabledReason)
		}
	}

	logger.Log.Info("Completed cleanup of inactive accounts")
}

func checkAccountAccess(s *discordgo.Session, account *models.Account) error {
	var channelID string

	if account.NotificationType == "dm" {
		channel, err := s.UserChannelCreate(account.UserID)
		if err != nil {
			return fmt.Errorf("failed to create DM channel: %v", err)
		}
		channelID = channel.ID
	} else {
		channelID = account.ChannelID
	}

	// Attempt to send a test message
	_, err := s.ChannelMessageSend(channelID, "This is a test message to verify bot access. You can ignore this message.")
	if err != nil {
		return fmt.Errorf("failed to send test message: %v", err)
	}

	return nil
}

func notifyUserAboutDisabledAccount(s *discordgo.Session, account models.Account, reason string) {
	channel, err := s.UserChannelCreate(account.UserID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to create DM channel for user %s", account.UserID)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: "Account Disabled",
		Description: fmt.Sprintf("Your account '%s' has been disabled. Reason: %s\n\n"+
			"To re-enable monitoring, please address the issue and use the /togglecheck command to re-enable your account.", account.Title, reason),
		Color:     0xFF0000, // Red color for alert
		Timestamp: time.Now().Format(time.RFC3339),
	}

	_, err = s.ChannelMessageSendEmbed(channel.ID, embed)
	_, err = s.ChannelMessageSendEmbed(channel.ID, embed)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send account disabled notification to user %s", account.UserID)
	}
}

func notifyUserOfCheckError(account models.Account, discord *discordgo.Session, err error) {
	// Send notification to developer
	admin.NotifyAdmin(fmt.Sprintf("Error checking account %s (ID: %d): %v", account.Title, account.ID, err))

	// Only notify user if it's a critical error
	if isCriticalError(err) {
		channel, err := discord.UserChannelCreate(account.UserID)
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to create DM channel for user %s", account.UserID)
			return
		}

		embed := &discordgo.MessageEmbed{
			Title: "Critical Account Check Error",
			Description: fmt.Sprintf("There was a critical error checking your account '%s'. "+
				"The bot developer has been notified and will investigate the issue.", account.Title),
			Color:     0xFF0000, // Red color for critical error
			Timestamp: time.Now().Format(time.RFC3339),
		}

		_, err = discord.ChannelMessageSendEmbed(channel.ID, embed)
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send critical error notification to user %s", account.UserID)
		}
	}
}

func isCriticalError(err error) bool {
	criticalErrors := []string{
		"invalid captcha API key",
		"insufficient balance",
		"bot removed from server/channel",
	}

	for _, criticalErr := range criticalErrors {
		if strings.Contains(err.Error(), criticalErr) {
			return true
		}
	}
	return false
}

func notifyUserOfError(s *discordgo.Session, userID string, message string) {
	userErrorNotificationMutex.Lock()
	defer userErrorNotificationMutex.Unlock()

	now := time.Now()
	notifications := userErrorNotifications[userID]

	// Remove old notifications
	for len(notifications) > 0 && now.Sub(notifications[0]) > userErrorNotificationCooldown {
		notifications = notifications[1:]
	}

	if len(notifications) < maxUserErrorNotifications {
		// Send notification to user
		channel, err := s.UserChannelCreate(userID)
		if err == nil {
			s.ChannelMessageSend(channel.ID, message)
		}
		notifications = append(notifications, now)
	}

	userErrorNotifications[userID] = notifications
}
