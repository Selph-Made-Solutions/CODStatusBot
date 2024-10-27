package services

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/discordgo"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bradselph/CODStatusBot/webserver"
	"github.com/patrickmn/go-cache"
)

var (
	userNotificationMutex      sync.Mutex
	userNotificationTimestamps = make(map[string]map[string]time.Time)
	userErrorNotifications     = make(map[string][]time.Time)
	userErrorNotificationMutex sync.Mutex
	adminNotificationCache     = cache.New(5*time.Minute, 10*time.Minute)
)

func NotifyAdmin(s *Discordgo.Session, message string) {
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

	embed := &Discordgo.MessageEmbed{
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

func GetNotificationChannel(s *Discordgo.Session, account models.Account, userSettings models.UserSettings) (string, error) {
	if userSettings.NotificationType == "dm" {
		channel, err := s.UserChannelCreate(account.UserID)
		if err != nil {
			return "", fmt.Errorf("failed to create DM channel: %w", err)
		}
		return channel.ID, nil
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

func CheckAndNotifyBalance(s *Discordgo.Session, userID string, balance float64) {
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
		"2captcha":  0.25,
	}

	threshold := thresholds[userSettings.PreferredCaptchaProvider]
	if balance < threshold {
		embed := &Discordgo.MessageEmbed{
			Title: fmt.Sprintf("Low %s Balance Alert", userSettings.PreferredCaptchaProvider),
			Description: fmt.Sprintf("Your %s balance is currently %.2f points, which is below the recommended threshold of %.2f points.",
				userSettings.PreferredCaptchaProvider, balance, threshold),
			Color: 0xFFA500,
			Fields: []*Discordgo.MessageEmbedField{
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

func ScheduleBalanceChecks(s *Discordgo.Session) {
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
			if user.PreferredCaptchaProvider == "2captcha" && user.TwoCaptchaAPIKey != "" {
				apiKey = user.TwoCaptchaAPIKey
				provider = "2captcha"
			} else if user.PreferredCaptchaProvider == "ezcaptcha" && user.EZCaptchaAPIKey != "" {
				apiKey = user.EZCaptchaAPIKey
				provider = "ezcaptcha"
			} else {
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

func DisableUserCaptcha(s *Discordgo.Session, userID string, reason string) error {
	var settings models.UserSettings
	if err := database.DB.Where("user_id = ?", userID).First(&settings).Error; err != nil {
		return err
	}

	settings.TwoCaptchaAPIKey = ""
	if IsServiceEnabled("ezcaptcha") {
		settings.PreferredCaptchaProvider = "ezcaptcha"
	} else if IsServiceEnabled("2captcha") {
		settings.PreferredCaptchaProvider = "2captcha"
	} else {
		settings.PreferredCaptchaProvider = "ezcaptcha"
	}

	settings.EZCaptchaAPIKey = ""
	settings.CustomSettings = false
	settings.CheckInterval = defaultSettings.CheckInterval
	settings.NotificationInterval = defaultSettings.NotificationInterval

	if err := database.DB.Save(&settings).Error; err != nil {
		return err
	}

	embed := &Discordgo.MessageEmbed{
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

func SendNotification(s *Discordgo.Session, account models.Account, embed *Discordgo.MessageEmbed, content, notificationType string) error {
	if account.IsCheckDisabled {
		logger.Log.Infof("Skipping notification for disabled account %s", account.Title)
		return nil
	}

	userSettings, err := GetUserSettings(account.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user settings: %w", err)
	}

	config, ok := notificationConfigs[notificationType]
	if !ok {
		return fmt.Errorf("unknown notification type: %s (account: %s, user: %s)", notificationType, account.Title, account.UserID)
	}

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

	channelID, err := GetNotificationChannel(s, account, userSettings)
	if err != nil {
		return fmt.Errorf("failed to get notification channel: %w", err)
	}

	_, err = s.ChannelMessageSendComplex(channelID, &Discordgo.MessageSend{
		Embed:   embed,
		Content: content,
	})
	if err != nil {
		if strings.Contains(err.Error(), "Missing Access") || strings.Contains(err.Error(), "Unknown Channel") {
			logger.Log.Warnf("Bot might have been removed from the channel or server for user %s", account.UserID)
			return fmt.Errorf("bot might have been removed: %w", err)
		}
		return fmt.Errorf("failed to send notification: %w", err)
	}

	logger.Log.Infof("%s notification sent to user %s", notificationType, account.UserID)
	userNotificationTimestamps[account.UserID][notificationType] = now

	account.LastNotification = now.Unix()
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to update LastNotification for account %s", account.Title)
	}

	return nil
}

func NotifyAdminWithCooldown(s *Discordgo.Session, message string, cooldownDuration time.Duration) {
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

func SendGlobalAnnouncement(s *Discordgo.Session, userID string) error {
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

func SendAnnouncementToAllUsers(s *Discordgo.Session) error {
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

func NotifyUserAboutDisabledAccount(s *Discordgo.Session, account models.Account, reason string) {
	embed := &Discordgo.MessageEmbed{
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

func NotifyCookieExpiringSoon(s *Discordgo.Session, accounts []models.Account) error {
	if len(accounts) == 0 {
		return nil
	}

	userID := accounts[0].UserID
	logger.Log.Infof("Sending cookie expiration warning for user %s", userID)

	var embedFields []*Discordgo.MessageEmbedField

	for _, account := range accounts {
		timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
		if err != nil {
			logger.Log.WithError(err).Errorf("Error checking SSO cookie expiration for account %s", account.Title)
			continue
		}
		embedFields = append(embedFields, &Discordgo.MessageEmbedField{
			Name:   account.Title,
			Value:  fmt.Sprintf("Cookie expires in %s", FormatDuration(timeUntilExpiration)),
			Inline: false,
		})
	}

	embed := &Discordgo.MessageEmbed{
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

func sendConsolidatedCookieExpirationWarning(s *Discordgo.Session, userID string, expiringAccounts []models.Account, userSettings models.UserSettings) {
	var embedFields []*Discordgo.MessageEmbedField

	for _, account := range expiringAccounts {
		timeUntilExpiration, _ := CheckSSOCookieExpiration(account.SSOCookieExpiration)
		embedFields = append(embedFields, &Discordgo.MessageEmbedField{
			Name:   account.Title,
			Value:  fmt.Sprintf("Cookie expires in %s", FormatDuration(timeUntilExpiration)),
			Inline: false,
		})
	}

	embed := &Discordgo.MessageEmbed{
		Title:       "SSO Cookie Expiration Warning",
		Description: "The following accounts have SSO cookies that will expire soon:",
		Color:       0xFFA500, // Orange color for warning
		Fields:      embedFields,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	err := SendNotification(s, expiringAccounts[0], embed, "", "cookie_expiring_soon")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send consolidated cookie expiration warning for user %s", userID)
	} else {
		userSettings.LastCookieExpirationWarning = time.Now()
		if err := database.DB.Save(&userSettings).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to update LastCookieExpirationWarning for user %s", userID)
		}
	}
}

func SendConsolidatedDailyUpdate(s *Discordgo.Session, userID string, userSettings models.UserSettings, accounts []models.Account) {
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

	var embedFields []*Discordgo.MessageEmbedField

	embedFields = append(embedFields, &Discordgo.MessageEmbedField{
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
				description.WriteString(fmt.Sprintf("📛 %s: Cookie expired\n", account.Title))
				continue
			}

			timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
			if err != nil {
				description.WriteString(fmt.Sprintf("❌ %s: Error checking expiration\n", account.Title))
				continue
			}

			icon := getStatusIcon(status)
			description.WriteString(fmt.Sprintf("%s %s: %s\n", icon, account.Title,
				formatAccountStatus(account, status, timeUntilExpiration)))
		}

		if description.Len() > 0 {
			embedFields = append(embedFields, &Discordgo.MessageEmbedField{
				Name:   fmt.Sprintf("%s Accounts", strings.Title(string(status))),
				Value:  description.String(),
				Inline: false,
			})
		}
	}

	embed := &Discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%.2f Hour Update - Account Status Report", userSettings.NotificationInterval),
		Description: "Here's a consolidated update on your monitored accounts:",
		Color:       0x00ff00,
		Fields:      embedFields,
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer: &Discordgo.MessageEmbedFooter{
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

func notifyUserOfCheckError(s *Discordgo.Session, account models.Account, err error) {
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

		embed := &Discordgo.MessageEmbed{
			Title: "Critical Account Check Error",
			Description: fmt.Sprintf("There was a critical error checking your account '%s'. "+
				"The bot developer has been notified and will investigate the issue.", account.Title),
			Color:     0xFF0000, // Red color for critical error
			Timestamp: time.Now().Format(time.RFC3339),
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

// TODO: change emojies to custom application emojis
func getStatusIcon(status models.Status) string {
	switch status {
	case models.StatusGood:
		return "✅"
	case models.StatusPermaban:
		return "🚫"
	case models.StatusShadowban:
		return "👁️"
	case models.StatusTempban:
		return "⏳"
	default:
		return "❓"
	}
}

func formatAccountStatus(account models.Account, status models.Status, timeUntilExpiration time.Duration) string {
	switch status {
	case models.StatusGood:
		return fmt.Sprintf("Good standing (Expires in %s)", FormatDuration(timeUntilExpiration))
	case models.StatusPermaban:
		return "Permanently banned"
	case models.StatusShadowban:
		return "Under review"
	case models.StatusTempban:
		return fmt.Sprintf("Temporarily banned (%s remaining)", account.TempBanDuration)
	default:
		return "Unknown status"
	}
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

func checkAccountsNeedingAttention(s *Discordgo.Session, accounts []models.Account, userSettings models.UserSettings) {
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
