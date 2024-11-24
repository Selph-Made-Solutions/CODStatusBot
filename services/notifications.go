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
	"github.com/patrickmn/go-cache"
)

const (
	defaultCooldown = 1 * time.Hour
	//	defaultNotifyInterval = 24 * time.Hour
	//	minNotifyInterval     = 1 * time.Hour
	//	maxNotifyInterval     = 72 * time.Hour
)

var (
	checkCircle                = os.Getenv("CHECKCIRCLE")
	banCircle                  = os.Getenv("BANCIRCLE")
	infoCircle                 = os.Getenv("INFOCIRCLE")
	stopWatch                  = os.Getenv("STOPWATCH")
	questionCircle             = os.Getenv("QUESTIONCIRCLE")
	userNotificationMutex      sync.Mutex
	userNotificationTimestamps = make(map[string]map[string]time.Time)
	adminNotificationCache     = cache.New(5*time.Minute, 10*time.Minute)
	notificationConfigs        = map[string]NotificationConfig{
		"channel_change":       {Type: "channel_change", Cooldown: time.Hour, AllowConsolidated: false, MaxPerHour: 4},
		"status_change":        {Type: "status_change", Cooldown: time.Hour, AllowConsolidated: false, MaxPerHour: 4},
		"permaban":             {Type: "permaban", Cooldown: 24 * time.Hour, AllowConsolidated: false, MaxPerHour: 2},
		"shadowban":            {Type: "shadowban", Cooldown: 12 * time.Hour, AllowConsolidated: false, MaxPerHour: 3},
		"daily_update":         {Type: "daily_update", Cooldown: 24 * time.Hour, AllowConsolidated: true, MaxPerHour: 1},
		"invalid_cookie":       {Type: "invalid_cookie", Cooldown: 6 * time.Hour, AllowConsolidated: true, MaxPerHour: 2},
		"cookie_expiring_soon": {Type: "cookie_expiring_soon", Cooldown: 24 * time.Hour, AllowConsolidated: true, MaxPerHour: 1},
		"temp_ban_update":      {Type: "temp_ban_update", Cooldown: time.Hour, AllowConsolidated: false, MaxPerHour: 4},
		"error":                {Type: "error", Cooldown: time.Hour, AllowConsolidated: false, MaxPerHour: 3},
		"account_added":        {Type: "account_added", Cooldown: time.Hour, AllowConsolidated: false, MaxPerHour: 5},
	}
)

type NotificationLimiter struct {
	sync.RWMutex
	userCounts map[string]*NotificationState
}

type NotificationState struct {
	hourlyCount int
	dailyCount  int
	lastReset   time.Time
	lastSent    time.Time
}

type NotificationConfig struct {
	Type              string
	Cooldown          time.Duration
	AllowConsolidated bool
	MaxPerHour        int
}

var (
	globalLimiter = NewNotificationLimiter()
	//	maxNotificationsPerHour = 4
	//	maxNotificationsPerDay  = 10
	//	minNotificationInterval = 5 * time.Minute
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

func IsDonationsEnabled() bool {
	return os.Getenv("DONATIONS_ENABLED") == "true"
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

	if balance == 0 {
		var err error
		apiKey, balance, err := GetUserCaptchaKey(userID)
		if err != nil {
			logger.Log.WithError(err).Error("Failed to get captcha balance")
			return
		}

		if apiKey == os.Getenv("EZCAPTCHA_CLIENT_KEY") {
			if balance < getBalanceThreshold(userSettings.PreferredCaptchaProvider) {
				var fields []*discordgo.MessageEmbedField

				fields = append(fields, &discordgo.MessageEmbedField{
					Name: "Option 1: Use Your Own API Key (Recommended)",
					Value: "Get your own API key using `/setcaptchaservice` for:\n" +
						"• Faster check intervals\n" +
						"• No rate limits\n" +
						"• More account slots",
					Inline: false,
				})

				if IsDonationsEnabled() {
					bitcoinAddress := os.Getenv("BITCOIN_ADDRESS")
					cashappID := os.Getenv("CASHAPP_ID")

					if bitcoinAddress != "" || cashappID != "" {
						donationText := "If you'd like to help keep the default API key funded:\n"
						if bitcoinAddress != "" {
							donationText += fmt.Sprintf("Bitcoin: %s\n", bitcoinAddress)
						}
						if cashappID != "" {
							donationText += fmt.Sprintf("CashApp: %s", cashappID)
						}

						fields = append(fields, &discordgo.MessageEmbedField{
							Name:   "Option 2: Help Support the Default API Key",
							Value:  donationText,
							Inline: false,
						})
					}
				}

				embed := &discordgo.MessageEmbed{
					Title: "Default API Key Balance Low",
					Description: fmt.Sprintf("The bot's default API key balance is currently low (%.2f points). "+
						"To ensure uninterrupted service, consider the following options:", balance),
					Color:     0xFFA500,
					Fields:    fields,
					Timestamp: time.Now().Format(time.RFC3339),
					Footer: &discordgo.MessageEmbedFooter{
						Text: "Thank you for using COD Status Bot!",
					},
				}

				var account models.Account
				if err := database.DB.Where("user_id = ?", userID).First(&account).Error; err != nil {
					logger.Log.WithError(err).Error("Failed to get account for balance notification")
					return
				}

				if err := SendNotification(s, account, embed, "", "default_key_balance"); err != nil {
					logger.Log.WithError(err).Error("Failed to send default key balance notification")
				}

				NotifyAdminWithCooldown(s, fmt.Sprintf("Default API key balance is low: %.2f", balance), time.Hour*6)
			}
			return
		}
	}

	var threshold float64
	switch userSettings.PreferredCaptchaProvider {
	case "ezcaptcha":
		threshold = 250
	case "2captcha":
		threshold = 0.50
	default:
		return
	}

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
		userSettings.CaptchaBalance = balance
		userSettings.LastBalanceCheck = time.Now()

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
	return &NotificationLimiter{
		userCounts: make(map[string]*NotificationState),
	}
}

func (nl *NotificationLimiter) CanSendNotification(userID string) bool {
	nl.Lock()
	defer nl.Unlock()

	now := time.Now()
	state, exists := nl.userCounts[userID]

	if !exists {
		state = &NotificationState{
			lastReset: now,
			lastSent:  now.Add(-time.Hour),
		}
		nl.userCounts[userID] = state
	}

	if now.Sub(state.lastReset) >= time.Hour {
		state.hourlyCount = 0
		state.lastReset = now
	}

	var userSettings models.UserSettings
	if err := database.DB.Where("user_id = ?", userID).First(&userSettings).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching user settings for rate limit check")
		return false
	}

	var maxPerHour int
	var minInterval time.Duration

	if userSettings.EZCaptchaAPIKey != "" || userSettings.TwoCaptchaAPIKey != "" {
		maxPerHour = 10
		minInterval = time.Minute * 5
	} else {
		maxPerHour = 4
		minInterval = time.Minute * 15
	}

	if state.hourlyCount >= maxPerHour {
		logger.Log.Debugf("User %s exceeded hourly notification limit", userID)
		return false
	}

	if now.Sub(state.lastSent) < minInterval {
		logger.Log.Debugf("User %s notification interval too short", userID)
		return false
	}

	state.hourlyCount++
	state.lastSent = now

	userSettings.LastCommandTimes["notification"] = now
	if err := database.DB.Save(&userSettings).Error; err != nil {
		logger.Log.WithError(err).Error("Error saving notification timestamp")
	}

	return true
}

func SendNotification(s *discordgo.Session, account models.Account, embed *discordgo.MessageEmbed, content, notificationType string) error {
	if !globalLimiter.CanSendNotification(account.UserID) {
		logger.Log.Infof("Notification suppressed due to rate limiting for user %s", account.UserID)
		return nil
	}

	if account.IsCheckDisabled {
		logger.Log.Infof("Skipping notification for disabled account %s", account.Title)
		return nil
	}

	userSettings, err := GetUserSettings(account.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user settings: %w", err)
	}

	// Check cooldown
	now := time.Now()
	lastNotification := userSettings.LastCommandTimes[notificationType]
	cooldownDuration := GetCooldownDuration(userSettings, notificationType, defaultCooldown)

	if !lastNotification.IsZero() && now.Sub(lastNotification) < cooldownDuration {
		logger.Log.Infof("Skipping %s notification for user %s (cooldown)", notificationType, account.UserID)
		return nil
	}

	channelID, err := GetNotificationChannel(s, account, userSettings)
	if err != nil {
		if userSettings.NotificationType == "dm" {
			channel, dmErr := s.UserChannelCreate(account.UserID)
			if dmErr != nil {
				return fmt.Errorf("failed to create DM channel: %w", dmErr)
			}
			channelID = channel.ID
		} else {
			return fmt.Errorf("failed to send notification: %w", err)
		}
	}

	_, err = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed:   embed,
		Content: content,
	})
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	userSettings.LastCommandTimes[notificationType] = now
	if err := database.DB.Save(&userSettings).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to update notification timestamp")
	}

	account.LastNotification = now.Unix()
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to update account last notification")
	}

	return nil
}

func NotifyAdminWithCooldown(s *discordgo.Session, message string, cooldownDuration time.Duration) {
	var admin models.UserSettings
	if err := database.DB.Where("user_id = ?", os.Getenv("DEVELOPER_ID")).FirstOrCreate(&admin).Error; err != nil {
		logger.Log.WithError(err).Error("Error getting admin settings")
		return
	}

	now := time.Now()
	notificationType := "admin_" + strings.Split(message, " ")[0]
	lastNotification := admin.LastCommandTimes[notificationType]

	if lastNotification.IsZero() || now.Sub(lastNotification) >= cooldownDuration {
		NotifyAdmin(s, message)
		admin.LastCommandTimes[notificationType] = now
		if err := database.DB.Save(&admin).Error; err != nil {
			logger.Log.WithError(err).Error("Error saving admin settings")
		}
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
		if !account.IsCheckDisabled && !account.IsExpiredCookie {
			accountsByStatus[account.LastStatus] = append(accountsByStatus[account.LastStatus], account)
		}
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
			//goland:noinspection GoDeprecation
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
		statusDesc.WriteString(fmt.Sprintf("Good standing | Expires in %s", FormatDuration(timeUntilExpiration)))
	case models.StatusPermaban:
		statusDesc.WriteString("Permanently banned")
	case models.StatusShadowban:
		statusDesc.WriteString("Under review")
	case models.StatusTempban:
		var latestBan models.Ban
		if err := database.DB.Where("account_id = ?", account.ID).
			Order("created_at DESC").
			First(&latestBan).Error; err == nil {
			statusDesc.WriteString(fmt.Sprintf("Temporarily banned (%s remaining)", latestBan.TempBanDuration))
		} else {
			statusDesc.WriteString("Temporarily banned (duration unknown)")
		}
	default:
		statusDesc.WriteString("Unknown status")
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
