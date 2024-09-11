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
	checkInterval               float64 // Check interval for accounts (in minutes)
	notificationInterval        float64 // Notification interval for daily updates (in hours)
	cooldownDuration            float64 // Cooldown duration for invalid cookie notifications (in hours)
	sleepDuration               int     // Sleep duration for the account checking loop (in minutes)
	cookieCheckIntervalPermaban float64 // Check interval for permabanned accounts (in hours)
	statusChangeCooldown        float64 // Cooldown duration for status change notifications (in hours)
	globalNotificationCooldown  float64 // Global cooldown for notifications per user (in hours)
	userNotificationTimestamps  = make(map[string]time.Time)
	userNotificationMutex       sync.Mutex
	DBMutex                     sync.Mutex
	defaultRateLimit            = 5 * time.Minute
)

func init() {
	err := godotenv.Load()
	if err != nil {
		logger.Log.WithError(err).Error("Failed to load .env file")
	}

	checkInterval, _ = strconv.ParseFloat(os.Getenv("CHECK_INTERVAL"), 64)
	notificationInterval, _ = strconv.ParseFloat(os.Getenv("NOTIFICATION_INTERVAL"), 64)
	cooldownDuration, _ = strconv.ParseFloat(os.Getenv("COOLDOWN_DURATION"), 64)
	sleepDuration, _ = strconv.Atoi(os.Getenv("SLEEP_DURATION"))
	cookieCheckIntervalPermaban, _ = strconv.ParseFloat(os.Getenv("COOKIE_CHECK_INTERVAL_PERMABAN"), 64)
	statusChangeCooldown, _ = strconv.ParseFloat(os.Getenv("STATUS_CHANGE_COOLDOWN"), 64)
	globalNotificationCooldown, _ = strconv.ParseFloat(os.Getenv("GLOBAL_NOTIFICATION_COOLDOWN"), 64)

	logger.Log.Infof("Loaded config: CHECK_INTERVAL=%.2f minutes, NOTIFICATION_INTERVAL=%.2f hours, COOLDOWN_DURATION=%.2f hours, SLEEP_DURATION=%d minutes, COOKIE_CHECK_INTERVAL_PERMABAN=%.2f hours, STATUS_CHANGE_COOLDOWN=%.2f hours, GLOBAL_NOTIFICATION_COOLDOWN=%.2f hours",
		checkInterval, notificationInterval, cooldownDuration, sleepDuration, cookieCheckIntervalPermaban, statusChangeCooldown, globalNotificationCooldown)
}

func canSendNotification(userID string, userSettings models.UserSettings) bool {
	userNotificationMutex.Lock()
	defer userNotificationMutex.Unlock()

	lastNotification, exists := userNotificationTimestamps[userID]
	if !exists || time.Since(lastNotification).Hours() >= userSettings.NotificationInterval {
		userNotificationTimestamps[userID] = time.Now()
		return true
	}
	logger.Log.Infof("Notification suppressed for user %s due to cooldown", userID)
	return false
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
			cooldownDuration = time.Duration(userSettings.NotificationInterval) * time.Hour
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

	_, err = discord.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed:   embed,
		Content: content,
	})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send notification")
	} else {
		logger.Log.Infof("%s notification sent to user %s", notificationType, account.UserID)
		userNotificationMutex.Lock()
		userNotificationTimestamps[account.UserID] = time.Now()
		userNotificationMutex.Unlock()
	}
	return err
}

// sendDailyUpdate function: sends a daily update message for a given account
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
		Color:       GetColorForStatus(account.LastStatus, account.IsExpiredCookie),
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
			if account.IsCheckDisabled {
				logger.Log.Infof("Skipping check for disabled account: %s", account.Title)
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
	if time.Since(lastNotification).Hours() >= float64(userSettings.NotificationInterval) {
		embed := &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Permanent Ban Status", account.Title),
			Description: fmt.Sprintf("The account %s is still permanently banned.", account.Title),
			Color:       GetColorForStatus(models.StatusPermaban, false),
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
		// Notify user if the cookie will expire within 24 hours}
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
	account.IsPermabanned = (newStatus == models.StatusPermaban)
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
		DBMutex.Unlock()
		return
	}
	logger.Log.Infof("Account %s status changed to %s", account.Title, newStatus)

	// Create a new record for the account
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
		Color:       GetColorForStatus(newStatus, account.IsExpiredCookie),
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

func sendStatusChangeNotification(account models.Account, newStatus models.Status, discord *discordgo.Session, ban models.Ban) {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - %s", account.Title, EmbedTitleFromStatus(newStatus)),
		Description: getStatusDescription(newStatus, account.Title, ban),
		Color:       GetColorForStatus(newStatus, account.IsExpiredCookie),
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	if newStatus == models.StatusPermaban {
		embed.Description += "\n\nThis account has been marked as permanently banned. It will no longer be checked automatically. You will receive daily reminders about this account. Please use the /removeaccount command to remove this account from monitoring if you no longer need it."
	}

	err := sendNotification(discord, account, embed, "", "daily_update")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send status update message for account %s", account.Title)
	}
}

func scheduleTempBanNotification(account models.Account, duration string, discord *discordgo.Session) {
	// Parse the duration string (assuming it's in the format "X days, Y hours")
	parts := strings.Split(duration, ",")
	if len(parts) != 2 {
		logger.Log.Errorf("Invalid duration format for account %s: %s", account.Title, duration)
		return
	}
	days, _ := strconv.Atoi(strings.TrimSpace(strings.Split(parts[0], " ")[0]))
	hours, _ := strconv.Atoi(strings.TrimSpace(strings.Split(parts[1], " ")[0]))

	sleepDuration := time.Duration(days)*24*time.Hour + time.Duration(hours)*time.Hour
	time.Sleep(sleepDuration)

	// Check the account status again after the temporary ban duration
	result, err := CheckAccount(account.SSOCookie, account.UserID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check account %s after temporary ban duration", account.Title)
		return
	}

	var embed *discordgo.MessageEmbed
	if result == models.StatusGood {
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Lifted", account.Title),
			Description: fmt.Sprintf("The temporary ban for account %s has been lifted. The account is now in good standing.", account.Title),
			Color:       GetColorForStatus(result, false),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	} else if result == models.StatusPermaban {
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Escalated", account.Title),
			Description: fmt.Sprintf("The temporary ban for account %s has been escalated to a permanent ban.", account.Title),
			Color:       GetColorForStatus(result, false),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	} else {
		// If the status is still temporary ban or any other status, send a notification stating this.
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Update", account.Title),
			Description: fmt.Sprintf("The temporary ban for account %s is still in effect. Current status: %s", account.Title, result),
			Color:       GetColorForStatus(result, false),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	}

	err = sendNotification(discord, account, embed, fmt.Sprintf("<@%s>", account.UserID), "temp_ban_update")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send temporary ban update message for account %s", account.Title)
	}
}

// GetColorForStatus function: returns the appropriate color for an embed message based on the account status
func GetColorForStatus(status models.Status, isExpiredCookie bool) int {
	if isExpiredCookie {
		return 0xff9900 // Orange for expired cookie
	}
	switch status {
	case models.StatusPermaban:
		return 0xff0000 // Red for permanent ban
	case models.StatusShadowban:
		return 0xffff00 // Yellow for shadowban
	default:
		return 0x00ff00 // Green for no ban
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

// SendGlobalAnnouncement function: sends a global announcement to users who haven't seen it yet
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
