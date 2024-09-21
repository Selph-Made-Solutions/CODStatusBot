package services

import (
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

var (
	checkInterval               float64 // Check interval for accounts (in minutes).
	notificationInterval        float64 // Notification interval for daily updates (in hours).
	cooldownDuration            float64 // Cooldown duration for invalid cookie notifications (in hours).
	sleepDuration               int     // Sleep duration for the account checking loop (in minutes).
	cookieCheckIntervalPermaban float64 // Check interval for permabanned accounts (in hours).
	statusChangeCooldown        float64 // Cooldown duration for status change notifications (in hours).
	globalNotificationCooldown  float64 // Global cooldown for notifications per user (in hours).
	cookieExpirationWarning     float64 // Time before cookie expiration to send a warning (in hours).
	tempBanUpdateInterval       float64 // Interval for temporary ban update notifications (in hours).
	userNotificationTimestamps  = make(map[string]time.Time)
	userNotificationMutex       sync.Mutex
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

	logger.Log.Infof("Loaded config: CHECK_INTERVAL=%.2f, NOTIFICATION_INTERVAL=%.2f, COOLDOWN_DURATION=%.2f, SLEEP_DURATION=%d, COOKIE_CHECK_INTERVAL_PERMABAN=%.2f, STATUS_CHANGE_COOLDOWN=%.2f, GLOBAL_NOTIFICATION_COOLDOWN=%.2f, COOKIE_EXPIRATION_WARNING=%.2f, TEMP_BAN_UPDATE_INTERVAL=%.2f, CHECK_NOW_RATE_LIMIT=%v, DEFAULT_RATE_LIMIT=%v",
		checkInterval, notificationInterval, cooldownDuration, sleepDuration, cookieCheckIntervalPermaban, statusChangeCooldown, globalNotificationCooldown, cookieExpirationWarning, tempBanUpdateInterval, checkNowRateLimit, defaultRateLimit)
}

const (
	maxRetryAttempts  = 3
	retryDelay        = 5 * time.Second
	maxFailedAttempts = 5
)

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

func PeriodicCleanup() {
	for {
		time.Sleep(24 * time.Hour)
		cleanupDisabledAccounts()
	}
}

// sendNotification function: sends notifications based on user preference
func sendNotification(discord *discordgo.Session, account models.Account, embed *discordgo.MessageEmbed, content string, notificationType string) error {
	userSettings, err := GetUserSettings(account.UserID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get user settings")
		return err
	}

	userNotificationMutex.Lock()
	lastNotification, exists := userNotificationTimestamps[account.UserID]
	userNotificationMutex.Unlock()

	shouldSend := true
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
	if account.NotificationType == "dm" {
		channel, err := discord.UserChannelCreate(account.UserID)
		if err != nil {
			return fmt.Errorf("failed to create DM channel: %v", err)
		}
		channelID = channel.ID
	} else {
		channelID = account.ChannelID
	}

	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		_, err = discord.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Embed:   embed,
			Content: content,
		})
		if err != nil {
			if isDiscordError(err, 10003, 50001, 50007) {
				logger.Log.WithError(err).Errorf("Discord API error for account %s (attempt %d/%d)", account.Title, attempt+1, maxRetryAttempts)
				if attempt == maxRetryAttempts-1 {
					// Max attempts reached, update failed attempts count
					if err := incrementFailedAttempts(account); err != nil {
						logger.Log.WithError(err).Error("Failed to increment failed attempts")
					}
					return err
				}
				time.Sleep(retryDelay)
				continue
			}
			logger.Log.WithError(err).Error("Failed to send notification")
			return err
		}
		// Message sent successfully
		logger.Log.Infof("%s notification sent to user %s", notificationType, account.UserID)
		userNotificationMutex.Lock()
		userNotificationTimestamps[account.UserID] = time.Now()
		userNotificationMutex.Unlock()
		// Reset failed attempts on successful send
		if err := resetFailedAttempts(account); err != nil {
			logger.Log.WithError(err).Error("Failed to reset failed attempts")
		}
		return nil
	}
	return fmt.Errorf("failed to send notification after %d attempts", maxRetryAttempts)
}

func isDiscordError(err error, codes ...int) bool {
	for _, code := range codes {
		if strings.Contains(err.Error(), fmt.Sprintf("\"code\": %d", code)) {
			return true
		}
	}
	return false
}

func incrementFailedAttempts(account models.Account) error {
	account.FailedAttempts++
	if account.FailedAttempts >= maxFailedAttempts {
		account.IsErrorDisabled = true
		logger.Log.Warnf("Account %s automatically disabled due to persistent notification failures", account.Title)
		notifyAdminAboutErrorDisabledAccount(account)
	}
	return database.DB.Save(&account).Error
}

func notifyAdminAboutErrorDisabledAccount(account models.Account) {
	adminID := os.Getenv("DEVELOPER_ID")
	if adminID == "" {
		logger.Log.Error("DEVELOPER_ID not set in environment variables")
		return
	}

	discord, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create Discord session")
		return
	}
	defer discord.Close()

	channel, err := discord.UserChannelCreate(adminID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create DM channel with admin")
		return
	}

	message := fmt.Sprintf("Account '%s' (ID: %d) has been automatically disabled due to persistent notification failures. Please review and take appropriate action.", account.Title, account.ID)
	_, err = discord.ChannelMessageSend(channel.ID, message)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send notification to admin")
	}
}

func resetFailedAttempts(account models.Account) error {
	account.FailedAttempts = 0
	return database.DB.Save(&account).Error
}

// sendDailyUpdate function: sends a daily update message for a given account.
func sendDailyUpdate(account models.Account, discord *discordgo.Session) {
	logger.Log.Infof("Attempting to send daily update for account %s", account.Title)

	// Prepare the description based on the account's cookie status
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

	// Send the notification
	err := sendNotification(discord, account, embed, "", "daily_update")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send scheduled update message for account %s", account.Title)
	} else {
		account.LastCheck = time.Now().Unix()
		account.LastNotification = time.Now().Unix()
		if err := database.DB.Save(&account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
		}
	}
}

// CheckAccounts function: periodically checks all accounts for status changes
func CheckAccounts(s *discordgo.Session) {
	for {
		logger.Log.Info("Starting periodic account check")
		var accounts []models.Account
		if err := database.DB.Find(&accounts).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to fetch accounts from the database")
			time.Sleep(time.Duration(sleepDuration) * time.Minute)
			continue
		}

		// Iterate through each account and perform checks
		for _, account := range accounts {
			// Skip disabled accounts
			if account.IsCheckDisabled || account.IsErrorDisabled {
				logger.Log.Infof("Skipping check for disabled account: %s (User disabled: %v, Error disabled: %v)",
					account.Title, account.IsCheckDisabled, account.IsErrorDisabled)
				continue
			}

			userSettings, err := GetUserSettings(account.UserID)
			if err != nil {
				logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", account.UserID)
				continue
			}

			if account.IsPermabanned {
				handlePermabannedAccount(account, s, userSettings)
				continue
			}

			if account.IsExpiredCookie {
				handleExpiredCookieAccount(account, s, userSettings)
				continue
			}

			checkInterval := userSettings.CheckInterval
			if userSettings.CaptchaAPIKey == "" {
				checkInterval = int(defaultRateLimit.Minutes())
			}

			lastCheck := time.Unix(account.LastCheck, 0)
			if time.Since(lastCheck).Minutes() < float64(checkInterval) {
				logger.Log.Infof("Skipping check for account %s (rate limit)", account.Title)
				continue
			}

			go CheckSingleAccount(account, s)

			if time.Since(time.Unix(account.LastNotification, 0)).Hours() > userSettings.NotificationInterval {
				go sendDailyUpdate(account, s)
			} else {
				logger.Log.WithField("account", account.Title).Infof("Owner of %s recently notified within %.2f hours already, skipping", account.Title, userSettings.NotificationInterval)
			}
		}
		time.Sleep(time.Duration(sleepDuration) * time.Minute)
	}
}

func handlePermabannedAccount(account models.Account, s *discordgo.Session, userSettings models.UserSettings) {
	lastNotification := time.Unix(account.LastNotification, 0)
	if time.Since(lastNotification).Hours() >= 24 { // Send a reminder every 24 hours.
		embed := &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Permanent Ban Status", account.Title),
			Description: fmt.Sprintf("The account %s is still permanently banned. Please remove this account from monitoring using the /removeaccount command.", account.Title),
			Color:       GetColorForStatus(models.StatusPermaban, false, account.IsCheckDisabled),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
		err := sendNotification(s, account, embed, fmt.Sprintf("<@%s>", account.UserID), "permaban")
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send permaban update for account %s", account.Title)
		} else {
			account.LastNotification = time.Now().Unix()
			if err := database.DB.Save(&account).Error; err != nil {
				logger.Log.WithError(err).Errorf("Failed to update LastNotification for account %s", account.Title)
			}
		}
	}
}

// Handle accounts with expired cookies
func handleExpiredCookieAccount(account models.Account, s *discordgo.Session, userSettings models.UserSettings) {
	logger.Log.WithField("account", account.Title).Info("Processing account with expired cookie")
	if time.Since(time.Unix(account.LastNotification, 0)).Hours() > userSettings.NotificationInterval {
		go sendDailyUpdate(account, s)
	} else {
		logger.Log.WithField("account", account.Title).Infof("Owner of %s recently notified within %.2f hours already, skipping", account.Title, userSettings.NotificationInterval)
	}
}

// CheckSingleAccount function: checks the status of a single account
func CheckSingleAccount(account models.Account, discord *discordgo.Session) {
	logger.Log.Infof("Checking account: %s", account.Title)

	timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check SSO cookie expiration for account %s", account.Title)
		// Notify user if the cookie will expire within 24 hours.
	} else if timeUntilExpiration > 0 && timeUntilExpiration <= 24*time.Hour {
		notifyCookieExpiringSoon(account, discord, timeUntilExpiration)
	}

	result, err := CheckAccount(account.SSOCookie, account.UserID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check account %s: possible expired SSO Cookie", account.Title)
		return
	}

	if result == models.StatusInvalidCookie {
		handleInvalidCookie(account, discord)
		return
	}

	updateAccountStatus(account, result, discord)
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
	} else if newStatus == models.StatusGood && account.LastStatus != models.StatusGood {
		embed.Description += "\n\nYour account has returned to good standing."
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

func cleanupDisabledAccounts() {
	var errorDisabledAccounts []models.Account
	if err := database.DB.Where("is_error_disabled = ?", true).Find(&errorDisabledAccounts).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to fetch error-disabled accounts")
		return
	}

	for _, account := range errorDisabledAccounts {
		notifyAdminAboutErrorDisabledAccount(account)
	}
}
func notifyUserAboutDisabledAccount(account models.Account) error {
	logger.Log.Infof("Account %s was automatically disabled due to persistent errors. User should be notified.", account.Title)
	return nil
}
