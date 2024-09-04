package services

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
	"strconv"
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
	accountCheckQueue           chan models.Account
	maxConcurrentChecks         int
)

func InitializeServices() error {
	loadEnvironmentVariables()
	initializeQueues()
	return nil
}

func loadEnvironmentVariables() {
	var err error
	checkInterval, err = strconv.ParseFloat(os.Getenv("CHECK_INTERVAL"), 64)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse CHECK_INTERVAL, using default of 15 minutes")
		checkInterval = 15
	}

	notificationInterval, err = strconv.ParseFloat(os.Getenv("NOTIFICATION_INTERVAL"), 64)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse NOTIFICATION_INTERVAL, using default of 24 hours")
		notificationInterval = 24
	}

	cooldownDuration, err = strconv.ParseFloat(os.Getenv("COOLDOWN_DURATION"), 64)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse COOLDOWN_DURATION, using default of 6 hours")
		cooldownDuration = 6
	}

	sleepDuration, err = strconv.Atoi(os.Getenv("SLEEP_DURATION"))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse SLEEP_DURATION, using default of 5 minutes")
		sleepDuration = 5
	}

	cookieCheckIntervalPermaban, err = strconv.ParseFloat(os.Getenv("COOKIE_CHECK_INTERVAL_PERMABAN"), 64)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse COOKIE_CHECK_INTERVAL_PERMABAN, using default of 24 hours")
		cookieCheckIntervalPermaban = 24
	}

	statusChangeCooldown, err = strconv.ParseFloat(os.Getenv("STATUS_CHANGE_COOLDOWN"), 64)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse STATUS_CHANGE_COOLDOWN, using default of 1 hour")
		statusChangeCooldown = 1
	}

	globalNotificationCooldown, err = strconv.ParseFloat(os.Getenv("GLOBAL_NOTIFICATION_COOLDOWN"), 64)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse GLOBAL_NOTIFICATION_COOLDOWN, using default of 1 hour")
		globalNotificationCooldown = 1
	}

	maxConcurrentChecks, err = strconv.Atoi(os.Getenv("MAX_CONCURRENT_CHECKS"))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse MAX_CONCURRENT_CHECKS, using default of 10")
		maxConcurrentChecks = 10
	}

	logger.Log.Info("Environment variables loaded successfully")
}

func initializeQueues() {
	accountCheckQueue = make(chan models.Account, 1000)
	for i := 0; i < maxConcurrentChecks; i++ {
		go accountCheckWorker()
	}
}

func accountCheckWorker() {
	for account := range accountCheckQueue {
		CheckSingleAccount(account, nil)
	}
}

func CheckAccounts(s *discordgo.Session) {
	for {
		logger.Log.Info("Starting periodic account check")
		var accounts []models.Account
		if err := database.DB.Find(&accounts).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to fetch accounts from the database")
			time.Sleep(time.Duration(sleepDuration) * time.Minute)
			continue
		}

		for _, account := range accounts {
			if account.IsCheckDisabled {
				logger.Log.Infof("Skipping check for disabled account: %s", account.Title)
				continue
			}

			userSettings, err := GetUserSettings(account.UserID)
			if err != nil {
				logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", account.UserID)
				continue
			}

			lastCheck := time.Unix(account.LastCheck, 0)
			if time.Since(lastCheck).Minutes() > float64(userSettings.CheckInterval) {
				accountCheckQueue <- account
			}

			lastNotification := time.Unix(account.LastNotification, 0)
			if time.Since(lastNotification).Hours() > userSettings.NotificationInterval {
				go sendDailyUpdate(account, s)
			}
		}

		time.Sleep(time.Duration(sleepDuration) * time.Minute)
	}
}

func CheckSingleAccount(account models.Account, s *discordgo.Session) {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("Recovered from panic in CheckSingleAccount: %v", r)
		}
	}()

	// Check SSO cookie expiration
	timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check SSO cookie expiration for account %s", account.Title)
	} else if timeUntilExpiration > 0 && timeUntilExpiration <= 24*time.Hour {
		notifyCookieExpiration(account, s, timeUntilExpiration)
	}

	if account.IsPermabanned {
		logger.Log.WithField("account", account.Title).Info("Skipping permanently banned account")
		return
	}

	result, err := CheckAccount(account.SSOCookie, account.UserID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check account %s", account.Title)
		return
	}

	updateAccountStatus(account, result, s)
}

func notifyCookieExpiration(account models.Account, s *discordgo.Session, timeUntilExpiration time.Duration) {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - SSO Cookie Expiring Soon", account.Title),
		Description: fmt.Sprintf("The SSO cookie for account %s will expire in %s. Please update the cookie soon using the /updateaccount command.", account.Title, FormatExpirationTime(account.SSOCookieExpiration)),
		Color:       0xFFA500, // Orange color for warning
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	err := sendNotification(s, account, embed, "")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send SSO cookie expiration notification for account %s", account.Title)
	}
}

func updateAccountStatus(account models.Account, result models.Status, s *discordgo.Session) {
	DBMutex.Lock()
	defer DBMutex.Unlock()

	account.LastCheck = time.Now().Unix()
	account.IsExpiredCookie = false

	if result != account.LastStatus {
		account.LastStatus = result
		account.LastStatusChange = time.Now().Unix()
		if result == models.StatusPermaban {
			account.IsPermabanned = true
		} else {
			account.IsPermabanned = false
		}

		if err := database.DB.Save(&account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
			return
		}

		logger.Log.Infof("Account %s status changed to %s", account.Title, result)

		ban := models.Ban{
			Account:   account,
			Status:    result,
			AccountID: account.ID,
		}
		if err := database.DB.Create(&ban).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to create new ban record for account %s", account.Title)
		}

		notifyStatusChange(account, result, s)
	}
}

func notifyStatusChange(account models.Account, result models.Status, s *discordgo.Session) {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - %s", account.Title, embedTitleFromStatus(result)),
		Description: fmt.Sprintf("The status of account %s has changed to %s", account.Title, result),
		Color:       getColorForStatus(result, account.IsExpiredCookie),
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	err := sendNotification(s, account, embed, fmt.Sprintf("<@%s>", account.UserID))
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send status update message for account %s", account.Title)
	}
}

func sendDailyUpdate(account models.Account, s *discordgo.Session) {
	logger.Log.Infof("Sending daily update for account %s", account.Title)

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
		Color:       getColorForStatus(account.LastStatus, account.IsExpiredCookie),
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	err := sendNotification(s, account, embed, "")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send scheduled update message for account %s", account.Title)
	}

	account.LastCheck = time.Now().Unix()
	account.LastNotification = time.Now().Unix()
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
	}
}

func sendNotification(s *discordgo.Session, account models.Account, embed *discordgo.MessageEmbed, content string) error {
	if !canSendNotification(account.UserID) {
		logger.Log.Infof("Skipping notification for user %s (global cooldown)", account.UserID)
		return nil
	}

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

	_, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed:   embed,
		Content: content,
	})
	return err
}

func canSendNotification(userID string) bool {
	userNotificationMutex.Lock()
	defer userNotificationMutex.Unlock()

	lastNotification, exists := userNotificationTimestamps[userID]
	if !exists || time.Since(lastNotification).Hours() >= globalNotificationCooldown {
		userNotificationTimestamps[userID] = time.Now()
		return true
	}
	return false
}

func embedTitleFromStatus(status models.Status) string {
	switch status {
	case models.StatusPermaban:
		return "PERMANENT BAN DETECTED"
	case models.StatusShadowban:
		return "SHADOWBAN DETECTED"
	case models.StatusTempban:
		return "TEMPORARY BAN DETECTED"
	case models.StatusUnderInvestigation:
		return "ACCOUNT UNDER INVESTIGATION"
	case models.StatusBanFinal:
		return "BAN FINAL"
	default:
		return "ACCOUNT STATUS CHANGED"
	}
}

func getColorForStatus(status models.Status, isExpiredCookie bool) int {
	if isExpiredCookie {
		return 0xff9900 // Orange for expired cookie
	}
	switch status {
	case models.StatusPermaban:
		return 0xff0000 // Red for permanent ban
	case models.StatusShadowban:
		return 0xffff00 // Yellow for shadowban
	case models.StatusTempban:
		return 0xffa500 // Orange for temporary ban
	case models.StatusUnderInvestigation:
		return 0x0000ff // Blue for under investigation
	case models.StatusBanFinal:
		return 0x800080 // Purple for ban final
	default:
		return 0x00ff00 // Green for no ban or unknown status
	}
}

func SendGlobalAnnouncement(s *discordgo.Session, userID string) error {
	var userSettings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&userSettings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings for global announcement")
		return result.Error
	}

	if !userSettings.HasSeenAnnouncement {
		var channelID string
		var err error

		if userSettings.NotificationType == "dm" {
			channel, err := s.UserChannelCreate(userID)
			if err != nil {
				logger.Log.WithError(err).Error("Error creating DM channel for global announcement")
				return err
			}
			channelID = channel.ID
		} else {
			var account models.Account
			if err := database.DB.Where("user_id = ?", userID).Order("updated_at DESC").First(&account).Error; err != nil {
				logger.Log.WithError(err).Error("Error finding recent channel for user")
				return err
			}
			channelID = account.ChannelID
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
