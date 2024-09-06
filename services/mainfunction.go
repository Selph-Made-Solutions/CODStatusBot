package services

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"golang.org/x/time/rate"
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
	discordRateLimiter          *rate.Limiter
	userSettingsCache           = make(map[string]models.UserSettings)
	userSettingsCacheMutex      sync.RWMutex
)

func init() {
	loadEnvironmentVariables()
	initializeQueues()
	discordRateLimiter = rate.NewLimiter(rate.Every(time.Second), 50) // Adjust as needed
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
	ticker := time.NewTicker(time.Duration(sleepDuration) * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		logger.Log.Info("Starting periodic account check")
		var accounts []models.Account
		if err := database.DB.Find(&accounts).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to fetch accounts from the database")
			continue
		}

		// Iterate through each account and perform checks
		for _, account := range accounts {
			// Skip disabled accounts
			if account.IsCheckDisabled {
				logger.Log.Infof("Skipping check for disabled account: %s", account.Title)
				continue
			}

			userSettings, err := getCachedUserSettings(account.UserID)
			if err != nil {
				logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", account.UserID)
				continue
			}

			lastCheck := time.Unix(account.LastCheck, 0)
			if time.Since(lastCheck).Minutes() > float64(userSettings.CheckInterval) {
				accountCheckQueue <- account
			}

			// Send daily update if enough time has passed since the last notification
			lastNotification := time.Unix(account.LastNotification, 0)
			if time.Since(lastNotification).Hours() > userSettings.NotificationInterval {
				go sendDailyUpdate(account, s)
			}
		}

		go cleanupStaleData()
	}
}

// CheckSingleAccount function: checks the status of a single account
func CheckSingleAccount(account models.Account, s *discordgo.Session) {
	result, err := CheckAccount(account.SSOCookie, account.UserID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check account %s", account.Title)
		return
	}

	DBMutex.Lock()
	defer DBMutex.Unlock()

	account.LastCheck = time.Now().Unix()
	account.IsExpiredCookie = result == models.StatusInvalidCookie

	if result != account.LastStatus {
		account.LastStatus = result
		account.LastStatusChange = time.Now().Unix()
		account.IsPermabanned = result == models.StatusPermaban

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
	} else {
		if err := database.DB.Save(&account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to update LastCheck for account %s", account.Title)
		}
	}
}

func notifyStatusChange(account models.Account, result models.Status, s *discordgo.Session) {
	if !canSendNotification(account.UserID) {
		logger.Log.Infof("Skipping status change notification for user %s (global cooldown)", account.UserID)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - %s", account.Title, embedTitleFromStatus(result)),
		Description: fmt.Sprintf("The status of account %s has changed to %s", account.Title, result),
		Color:       getColorForStatus(result, account.IsExpiredCookie),
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	if err := sendNotification(s, account, embed, fmt.Sprintf("<@%s>", account.UserID)); err != nil {
		logger.Log.WithError(err).Errorf("Failed to send status update message for account %s", account.Title)
	}
}

func sendDailyUpdate(account models.Account, s *discordgo.Session) {
	if !canSendNotification(account.UserID) {
		logger.Log.Infof("Skipping daily update for user %s (global cooldown)", account.UserID)
		return
	}

	logger.Log.Infof("Sending daily update for account %s", account.Title)

	description := getDailyUpdateDescription(account)

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%.2f Hour Update - %s", notificationInterval, account.Title),
		Description: description,
		Color:       getColorForStatus(account.LastStatus, account.IsExpiredCookie),
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	if err := sendNotification(s, account, embed, ""); err != nil {
		logger.Log.WithError(err).Errorf("Failed to send scheduled update message for account %s", account.Title)
	}

	DBMutex.Lock()
	defer DBMutex.Unlock()

	account.LastCheck = time.Now().Unix()
	account.LastNotification = time.Now().Unix()
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
	}
}

func sendNotification(s *discordgo.Session, account models.Account, embed *discordgo.MessageEmbed, content string) error {
	if err := discordRateLimiter.Wait(s.Context()); err != nil {
		return fmt.Errorf("rate limit exceeded: %v", err)
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
	case models.StatusGood:
		return "ACCOUNT IN GOOD STANDING"
	default:
		return "ACCOUNT STATUS CHANGED"
	}
}

// EmbedTitleFromStatus function: returns the appropriate title for an embed message based on the account status
func getColorForStatus(status models.Status, isExpiredCookie bool) int {
	if isExpiredCookie {
		return 0xff9900 // Orange for expired cookie
	}
	switch status {
	case models.StatusPermaban:
		return 0xff0000 // Red for permanent ban
	case models.StatusShadowban:
		return 0xffff00 // Yellow for shadowban
	case models.StatusGood:
		return 0x00ff00 // Green for good standing
	default:
		return 0x808080 // Gray for unknown status
	}
}

// SendGlobalAnnouncement function: sends a global announcement to users who haven't seen it yet
func getDailyUpdateDescription(account models.Account) string {
	if account.IsExpiredCookie {
		return fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title)
	}

	timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
	if err != nil {
		logger.Log.WithError(err).Errorf("Error checking SSO cookie expiration for account %s", account.Title)
		return fmt.Sprintf("An error occurred while checking the SSO cookie expiration for account %s. Please check the account status manually.", account.Title)
	}

	if timeUntilExpiration > 0 {
		return fmt.Sprintf("The last status of account %s was %s. SSO cookie will expire in %s.", account.Title, account.LastStatus, FormatExpirationTime(account.SSOCookieExpiration))
	}

	return fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title)
}

func cleanupStaleData() {
	// Remove old ban records
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	if err := database.DB.Where("created_at < ?", thirtyDaysAgo).Delete(&models.Ban{}).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to clean up old ban records")
	}

	// Remove inactive accounts (not checked in the last 90 days)
	ninetyDaysAgo := time.Now().AddDate(0, 0, -90)
	if err := database.DB.Where("last_check < ?", ninetyDaysAgo.Unix()).Delete(&models.Account{}).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to clean up inactive accounts")
	}
}

func getCachedUserSettings(userID string) (models.UserSettings, error) {
	userSettingsCacheMutex.RLock()
	settings, ok := userSettingsCache[userID]
	userSettingsCacheMutex.RUnlock()

	if ok {
		return settings, nil
	}

	settings, err := GetUserSettings(userID)
	if err != nil {
		return settings, err
	}

	userSettingsCacheMutex.Lock()
	userSettingsCache[userID] = settings
	userSettingsCacheMutex.Unlock()

	return settings, nil
}

func invalidateUserSettingsCache(userID string) {
	userSettingsCacheMutex.Lock()
	delete(userSettingsCache, userID)
	userSettingsCacheMutex.Unlock()
}
