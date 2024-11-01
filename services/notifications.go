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
	"github.com/bradselph/CODStatusBot/webserver"
	"github.com/bwmarrin/discordgo"
	"github.com/patrickmn/go-cache"
)

var (
	userNotificationMutex      sync.Mutex
	userNotificationTimestamps = make(map[string]map[string]time.Time)
	adminNotificationCache     = cache.New(5*time.Minute, 10*time.Minute)
	checkCircle                = os.Getenv("CHECKCIRCLE")
	banCircle                  = os.Getenv("BANCIRCLE")
	infoCircle                 = os.Getenv("INFOCIRCLE")
	stopWatch                  = os.Getenv("STOPWATCH")
	questionCircle             = os.Getenv("QUESTIONCIRCLE")
)

type NotificationLimiter struct {
	sync.RWMutex
	notifications map[string]*NotificationState
	cleanupTicker *time.Ticker
}

type NotificationState struct {
	Count     int
	LastReset time.Time
	LastSent  time.Time
}

var (
	globalLimiter           = NewNotificationLimiter()
	maxNotificationsPerHour = 4
	maxNotificationsPerDay  = 10
	minNotificationInterval = 5 * time.Minute
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

	if account.ChannelID == "" {
		return "", fmt.Errorf("no channel ID set for account")
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
		"2captcha":  0.50,
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
			switch {
			case user.PreferredCaptchaProvider == "2captcha" && user.TwoCaptchaAPIKey != "":
				apiKey = user.TwoCaptchaAPIKey
				provider = "2captcha"
			case user.PreferredCaptchaProvider == "ezcaptcha" && user.EZCaptchaAPIKey != "":
				apiKey = user.EZCaptchaAPIKey
				provider = "ezcaptcha"
			default:
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
	switch {
	case IsServiceEnabled("ezcaptcha"):
		settings.PreferredCaptchaProvider = "ezcaptcha"
	case IsServiceEnabled("2captcha"):
		settings.PreferredCaptchaProvider = "2captcha"
	default:
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

func NewNotificationLimiter() *NotificationLimiter {
	nl := &NotificationLimiter{
		notifications: make(map[string]*NotificationState),
		cleanupTicker: time.NewTicker(1 * time.Hour),
	}

	go nl.cleanup()
	return nl
}

func (nl *NotificationLimiter) cleanup() {
	for range nl.cleanupTicker.C {
		nl.Lock()
		now := time.Now()
		for userID, state := range nl.notifications {
			if now.Sub(state.LastSent) > 24*time.Hour {
				delete(nl.notifications, userID)
			}
		}
		nl.Unlock()
	}
}

func (nl *NotificationLimiter) CanSendNotification(userID string) bool {
	nl.Lock()
	defer nl.Unlock()

	now := time.Now()
	state, exists := nl.notifications[userID]

	if !exists {
		nl.notifications[userID] = &NotificationState{
			Count:     1,
			LastReset: now,
			LastSent:  now,
		}
		return true
	}

	if now.Sub(state.LastReset) >= 24*time.Hour {
		state.Count = 0
		state.LastReset = now
	}

	if now.Sub(state.LastSent) < minNotificationInterval {
		return false
	}

	hourlyCount := 0
	if now.Sub(state.LastSent) < time.Hour {
		hourlyCount = state.Count
	}

	if hourlyCount >= maxNotificationsPerHour || state.Count >= maxNotificationsPerDay {
		return false
	}

	state.Count++
	state.LastSent = now
	return true
}

func SendNotification(s *discordgo.Session, account models.Account, embed *discordgo.MessageEmbed, content, notificationType string) error {
	// Rate limit and disabled checks
	if !globalLimiter.CanSendNotification(account.UserID) {
		logger.Log.Infof("Notification suppressed due to rate limiting for user %s", account.UserID)
		return nil
	}

	if account.IsCheckDisabled {
		logger.Log.Infof("Skipping notification for disabled account %s", account.Title)
		return nil
	}

	// Get user settings and validate notification type
	userSettings, err := GetUserSettings(account.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user settings: %w", err)
	}

	config, ok := notificationConfigs[notificationType]
	if !ok {
		return fmt.Errorf("unknown notification type: %s", notificationType)
	}

	// Check cooldown
	userNotificationMutex.Lock()
	defer userNotificationMutex.Unlock()

	if _, exists := userNotificationTimestamps[account.UserID]; !exists {
		userNotificationTimestamps[account.UserID] = make(map[string]time.Time)
	}

	lastNotification, exists := userNotificationTimestamps[account.UserID][notificationType]
	now := time.Now()
	cooldownDuration := GetCooldownDuration(userSettings, notificationType, config.Cooldown)

	if exists && now.Sub(lastNotification) < cooldownDuration {
		logger.Log.Infof("Skipping %s notification for user %s (cooldown)", notificationType, account.UserID)
		return nil
	}

	// Get channel and send message
	channelID, err := GetNotificationChannel(s, account, userSettings)
	if err != nil {
		if userSettings.NotificationType == "dm" {
			// If user prefers DMs and DM failed, no fallback
			return fmt.Errorf("failed to send DM notification: %w", err)
		}

		// Only try DM fallback for channel access errors
		if strings.Contains(err.Error(), "Missing Access") || strings.Contains(err.Error(), "Unknown Channel") {
			logger.Log.Warnf("Channel notification failed for account %s, falling back to DM", account.Title)
			channel, dmErr := s.UserChannelCreate(account.UserID)
			if dmErr != nil {
				return fmt.Errorf("both channel and DM notification failed: %w", err)
			}
			channelID = channel.ID
		} else {
			return fmt.Errorf("failed to send notification: %w", err)
		}
	}

	// Send the actual message
	_, err = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed:   embed,
		Content: content,
	})
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Update timestamps and save
	logger.Log.Infof("%s notification sent to user %s", notificationType, account.UserID)
	userNotificationTimestamps[account.UserID][notificationType] = now

	account.LastNotification = now.Unix()
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to update LastNotification for account %s", account.Title)
	}

	return nil
}

func NotifyAdminWithCooldown(s *discordgo.Session, message string, cooldownDuration time.Duration) {
	webserver.NotificationMutex.Lock()
	defer webserver.NotificationMutex.Unlock()

	notificationType := "admin_" + strings.Split(message, " ")[0]

	_, found := adminNotificationCache.Get(notificationType)
	if !found {
		NotifyAdmin(s, message)
		adminNotificationCache.Set(notificationType, time.Now(), cooldownDuration)
	} else {
		logger.Log.Infof("Skipping admin notification '%s' due to cooldown", notificationType)
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
		channelID, err := getChannelForAnnouncement(s, userID, userSettings)
		if err != nil {
			logger.Log.WithError(err).Error("Error finding recent channel for user")
			return err
		}

		announcementEmbed := CreateAnnouncementEmbed()

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

func NotifyUserAboutDisabledAccount(s *discordgo.Session, account models.Account, reason string) {
	embed := &discordgo.MessageEmbed{
		Title: "Account Disabled",
		Description: fmt.Sprintf("Your account '%s' has been disabled. Reason: %s\n\n"+
			"To re-enable monitoring, please address the issue and use the /togglecheck command to re-enable your account.", account.Title, reason),
		Color:     0xFF0000,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	err := SendNotification(s, account, embed, "", "account_disabled")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send account disabled notification to user %s", account.UserID)
	}
}

func NotifyCookieExpiringSoon(s *discordgo.Session, accounts []models.Account) error {
	if len(accounts) == 0 {
		return nil
	}

	userID := accounts[0].UserID
	logger.Log.Infof("Sending cookie expiration warning for user %s", userID)

	var embedFields []*discordgo.MessageEmbedField

	for _, account := range accounts {
		timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
		if err != nil {
			logger.Log.WithError(err).Errorf("Error checking SSO cookie expiration for account %s", account.Title)
			continue
		}
		embedFields = append(embedFields, &discordgo.MessageEmbedField{
			Name:   account.Title,
			Value:  fmt.Sprintf("Cookie expires in %s", FormatDuration(timeUntilExpiration)),
			Inline: false,
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:       "SSO Cookie Expiration Warning",
		Description: "The following accounts have SSO cookies that will expire soon:",
		Color:       0xFFA500,
		Fields:      embedFields,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	return SendNotification(s, accounts[0], embed, "", "cookie_expiring_soon")
}

func CheckNotificationCooldown(userID string, notificationType string, cooldownDuration time.Duration) (bool, error) {
	var settings models.UserSettings
	if err := database.DB.Where("user_id = ?", userID).First(&settings).Error; err != nil {
		return false, err
	}

	var lastNotification time.Time
	switch notificationType {
	case "balance":
		lastNotification = settings.LastBalanceNotification
	case "error":
		lastNotification = settings.LastErrorNotification
	case "disabled":
		lastNotification = settings.LastDisabledNotification
	default:
		return false, fmt.Errorf("unknown notification type: %s", notificationType)
	}

	if time.Since(lastNotification) >= cooldownDuration {
		return true, nil
	}
	return false, nil
}

func UpdateNotificationTimestamp(userID string, notificationType string) error {
	var settings models.UserSettings
	if err := database.DB.Where("user_id = ?", userID).First(&settings).Error; err != nil {
		return err
	}

	now := time.Now()
	switch notificationType {
	case "balance":
		settings.LastBalanceNotification = now
	case "error":
		settings.LastErrorNotification = now
	case "disabled":
		settings.LastDisabledNotification = now
	default:
		return fmt.Errorf("unknown notification type: %s", notificationType)
	}

	return database.DB.Save(&settings).Error
}

// TODO: Fix this function as it returns inconsistent amount of accounts for users
func SendConsolidatedDailyUpdate(s *discordgo.Session, userID string, userSettings models.UserSettings, accounts []models.Account) {
	if len(accounts) == 0 {
		return
	}

	userSettings, err := GetUserSettings(userID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", userID)
		return
	}

	accountsByStatus := make(map[models.Status][]models.Account)
	for _, account := range accounts {
		accountsByStatus[account.LastStatus] = append(accountsByStatus[account.LastStatus], account)
	}

	var embedFields []*discordgo.MessageEmbedField

	embedFields = append(embedFields, &discordgo.MessageEmbedField{
		Name: "Summary",
		Value: fmt.Sprintf("Total Accounts: %d\nGood Standing: %d\nBanned: %d\nUnder Review: %d",
			len(accounts),
			len(accountsByStatus[models.StatusGood]),
			len(accountsByStatus[models.StatusPermaban])+len(accountsByStatus[models.StatusTempban]),
			len(accountsByStatus[models.StatusShadowban])),
		Inline: false,
	})

	for status, statusAccounts := range accountsByStatus {
		var description strings.Builder
		for _, account := range statusAccounts {
			if account.IsExpiredCookie {
				description.WriteString(fmt.Sprintf("⚠ %s: Cookie expired\n", account.Title))
				continue
			}

			timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
			if err != nil {
				description.WriteString(fmt.Sprintf("⛔ %s: Error checking expiration\n", account.Title))
				continue
			}

			statusSymbol := GetStatusIcon(status)
			description.WriteString(fmt.Sprintf("%s %s: %s\n", statusSymbol, account.Title,
				formatAccountStatus(account, status, timeUntilExpiration)))
		}

		if description.Len() > 0 {
			embedFields = append(embedFields, &discordgo.MessageEmbedField{
				Name:   fmt.Sprintf("%s Accounts", strings.Title(string(status))),
				Value:  description.String(),
				Inline: false,
			})
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%.2f Hour Update - Account Status Report", userSettings.NotificationInterval),
		Description: "Here's a consolidated update on your monitored accounts:",
		Color:       0x00ff00,
		Fields:      embedFields,
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Use /checknow to check any account immediately",
		},
	}

	err = SendNotification(s, accounts[0], embed, "", "daily_update")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send consolidated daily update for user %s", userID)
	} else {
		userSettings.LastDailyUpdateNotification = time.Now()
		if err := database.DB.Save(&userSettings).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to update LastDailyUpdateNotification for user %s", userID)
		}
	}

	checkAccountsNeedingAttention(s, accounts, userSettings)
}

// TODO: do not notify user of errors only send a notification to admin without flooding the admin with a large amount of messages
/*
func notifyUserOfCheckError(s *discordgo.Session, account models.Account, err error) {
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
*/

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

func GetStatusIcon(status models.Status) string {
	switch status {
	case models.StatusGood:
		return checkCircle
	case models.StatusPermaban:
		return banCircle
	case models.StatusShadowban:
		return infoCircle
	case models.StatusTempban:
		return stopWatch
	default:
		return questionCircle
	}
}

func formatAccountStatus(account models.Account, status models.Status, timeUntilExpiration time.Duration) string {
	var statusDesc strings.Builder

	switch status {
	case models.StatusGood:
		statusDesc.WriteString(fmt.Sprintf("%s Good standing | Expires in %s", checkCircle, FormatDuration(timeUntilExpiration)))
	case models.StatusPermaban:
		statusDesc.WriteString(fmt.Sprintf("%s Permanently banned", banCircle))
	case models.StatusShadowban:
		statusDesc.WriteString(fmt.Sprintf("%s Under review", infoCircle))
	case models.StatusTempban:
		var latestBan models.Ban
		if err := database.DB.Where("account_id = ?", account.ID).
			Order("created_at DESC").
			First(&latestBan).Error; err == nil {
			statusDesc.WriteString(fmt.Sprintf("%s Temporarily banned (%s remaining)", stopWatch, latestBan.TempBanDuration))
		} else {
			statusDesc.WriteString(fmt.Sprintf("%s Temporarily banned (duration unknown)", stopWatch))
		}
	default:
		statusDesc.WriteString(fmt.Sprintf("%s Unknown status", questionCircle))
	}

	if isVIP, err := CheckVIPStatus(account.SSOCookie); err == nil {
		statusDesc.WriteString(fmt.Sprintf(" | %s", formatVIPStatus(isVIP)))
	}

	statusDesc.WriteString(fmt.Sprintf(" | Checks: %s", formatCheckStatus(account.IsCheckDisabled)))

	return statusDesc.String()
}

func formatVIPStatus(isVIP bool) string {
	if isVIP {
		return "VIP Account"
	}
	return "Regular Account"
}

func formatCheckStatus(isDisabled bool) string {
	if isDisabled {
		return "DISABLED"
	}
	return "ENABLED"
}

func getNotificationType(status models.Status) string {
	switch status {
	case models.StatusPermaban:
		return "permaban"
	case models.StatusShadowban:
		return "shadowban"
	case models.StatusTempban:
		return "tempban"
	default:
		return "status_change"
	}
}

func checkAccountsNeedingAttention(s *discordgo.Session, accounts []models.Account, userSettings models.UserSettings) {
	var expiringAccounts []models.Account
	var errorAccounts []models.Account

	for _, account := range accounts {
		if !account.IsExpiredCookie {
			timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
			if err != nil {
				errorAccounts = append(errorAccounts, account)
			} else if timeUntilExpiration <= time.Duration(cookieExpirationWarning)*time.Hour {
				expiringAccounts = append(expiringAccounts, account)
			}
		}

		if account.ConsecutiveErrors >= maxConsecutiveErrors {
			errorAccounts = append(errorAccounts, account)
		}
	}

	if len(expiringAccounts) > 0 {
		NotifyCookieExpiringSoon(s, expiringAccounts)
	}

	if len(errorAccounts) > 0 {
		notifyAccountErrors(s, errorAccounts, userSettings)
	}
}

func notifyAccountErrors(s *discordgo.Session, errorAccounts []models.Account, userSettings models.UserSettings) {
	if len(errorAccounts) == 0 {
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Account Check Errors",
		Description: "The following accounts have encountered errors during status checks:",
		Color:       0xFF0000,
		Fields:      make([]*discordgo.MessageEmbedField, 0),
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	for _, account := range errorAccounts {
		var errorDescription string
		if account.IsCheckDisabled {
			errorDescription = fmt.Sprintf("Checks disabled - Reason: %s", account.DisabledReason)
		} else if account.ConsecutiveErrors >= maxConsecutiveErrors {
			errorDescription = fmt.Sprintf("Multiple check failures - Last error time: %s",
				account.LastErrorTime.Format("2006-01-02 15:04:05"))
		} else {
			errorDescription = "Unknown error"
		}

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   account.Title,
			Value:  errorDescription,
			Inline: false,
		})
	}

	err := SendNotification(s, errorAccounts[0], embed, "", "error")
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send account errors notification")
	}

	userSettings.LastErrorNotification = time.Now()
	if err = database.DB.Save(&userSettings).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to update LastErrorNotification timestamp")
	}
}
