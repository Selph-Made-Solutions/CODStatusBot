package services

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

type NotificationConfig struct {
	Type              string
	Cooldown          time.Duration
	AllowConsolidated bool
}

const (
	maxConsecutiveErrors          = 5
	balanceNotificationThreshold  = 1000
	maxUserErrorNotifications     = 3
	userErrorNotificationCooldown = 24 * time.Hour
	balanceNotificationInterval   = 24 * time.Hour
)

var (
	checkInterval               float64
	notificationInterval        float64
	cooldownDuration            float64
	sleepDuration               int
	cookieCheckIntervalPermaban float64
	statusChangeCooldown        float64
	globalNotificationCooldown  float64
	cookieExpirationWarning     float64
	tempBanUpdateInterval       float64
	defaultRateLimit            time.Duration
	checkNowRateLimit           time.Duration
	DBMutex                     sync.Mutex
	notificationConfigs         = map[string]NotificationConfig{
		"status_change":        {Cooldown: time.Hour, AllowConsolidated: false},
		"permaban":             {Cooldown: 24 * time.Hour, AllowConsolidated: false},
		"shadowban":            {Cooldown: 12 * time.Hour, AllowConsolidated: false},
		"daily_update":         {Cooldown: 0, AllowConsolidated: true},
		"invalid_cookie":       {Cooldown: 6 * time.Hour, AllowConsolidated: true},
		"cookie_expiring_soon": {Cooldown: 24 * time.Hour, AllowConsolidated: true},
		"temp_ban_update":      {Cooldown: time.Hour, AllowConsolidated: false},
		"error":                {Cooldown: time.Hour, AllowConsolidated: false},
		"account_added":        {Cooldown: time.Hour, AllowConsolidated: false},
	}
)

func init() {
	if err := godotenv.Load(); err != nil {
		logger.Log.WithError(err).Error("Failed to load .env file")
	}

	checkInterval = GetEnvFloat("CHECK_INTERVAL", 15)
	notificationInterval = GetEnvFloat("NOTIFICATION_INTERVAL", 24)
	cooldownDuration = GetEnvFloat("COOLDOWN_DURATION", 6)
	sleepDuration = GetEnvInt("SLEEP_DURATION", 1)
	cookieCheckIntervalPermaban = GetEnvFloat("COOKIE_CHECK_INTERVAL_PERMABAN", 24)
	statusChangeCooldown = GetEnvFloat("STATUS_CHANGE_COOLDOWN", 1)
	globalNotificationCooldown = GetEnvFloat("GLOBAL_NOTIFICATION_COOLDOWN", 2)
	cookieExpirationWarning = GetEnvFloat("COOKIE_EXPIRATION_WARNING", 24)
	tempBanUpdateInterval = GetEnvFloat("TEMP_BAN_UPDATE_INTERVAL", 24)
	defaultRateLimit = time.Duration(GetEnvInt("DEFAULT_RATE_LIMIT", 5)) * time.Minute
	checkNowRateLimit = time.Duration(GetEnvInt("CHECK_NOW_RATE_LIMIT", 3600)) * time.Second

	logger.Log.Infof("Loaded config: CHECK_INTERVAL=%.2f, NOTIFICATION_INTERVAL=%.2f, COOLDOWN_DURATION=%.2f, SLEEP_DURATION=%d, COOKIE_CHECK_INTERVAL_PERMABAN=%.2f, STATUS_CHANGE_COOLDOWN=%.2f, GLOBAL_NOTIFICATION_COOLDOWN=%.2f, COOKIE_EXPIRATION_WARNING=%.2f, TEMP_BAN_UPDATE_INTERVAL=%.2f, CHECK_NOW_RATE_LIMIT=%v, DEFAULT_RATE_LIMIT=%v",
		checkInterval, notificationInterval, cooldownDuration, sleepDuration, cookieCheckIntervalPermaban, statusChangeCooldown, globalNotificationCooldown, cookieExpirationWarning, tempBanUpdateInterval, checkNowRateLimit, defaultRateLimit)
}

func GetEnvFloat(key string, fallback float64) float64 {
	value := GetEnvFloatRaw(key, fallback)
	// Convert hours to minutes for certain settings
	if key == "CHECK_INTERVAL" || key == "SLEEP_DURATION" || key == "DEFAULT_RATE_LIMIT" {
		return value
	}
	// All other values are in hours, so we don't need to convert them
	return value
}

func GetEnvFloatRaw(key string, fallback float64) float64 {
	if value, ok := os.LookupEnv(key); ok {
		floatValue, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return floatValue
		}
		logger.Log.WithError(err).Errorf("Failed to parse %s, using fallback value", key)
	}
	return fallback
}

func GetEnvInt(key string, fallback int) int {
	return GetEnvIntRaw(key, fallback)
}

func GetEnvIntRaw(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		intValue, err := strconv.Atoi(value)
		if err == nil {
			return intValue
		}
		logger.Log.WithError(err).Errorf("Failed to parse %s, using fallback value", key)
	}
	return fallback
}

func checkAccountAfterTempBan(s *discordgo.Session, account models.Account) {
	result, err := CheckAccount(account.SSOCookie, account.UserID, "")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check account %s after temporary ban duration", account.Title)
		return
	}

	var embed *discordgo.MessageEmbed
	switch result {
	case models.StatusGood:
		embed = createTempBanLiftedEmbed(account)
	case models.StatusPermaban:
		embed = createTempBanEscalatedEmbed(account)
	default:
		embed = createTempBanStillActiveEmbed(account, result)
	}

	err = SendNotification(s, account, embed, fmt.Sprintf("<@%s>", account.UserID), "temp_ban_update")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send temporary ban update message for account %s", account.Title)
	}
}

func HandleStatusChange(s *discordgo.Session, account models.Account, newStatus models.Status, userSettings models.UserSettings) {
	DBMutex.Lock()
	defer DBMutex.Unlock()

	now := time.Now()

	if now.Sub(userSettings.LastStatusChangeNotification) < time.Duration(userSettings.StatusChangeCooldown)*time.Hour {
		logger.Log.Infof("Skipping status change notification for account %s (cooldown)", account.Title)
		return
	}

	account.LastStatus = newStatus
	account.LastStatusChange = now.Unix()
	account.IsPermabanned = newStatus == models.StatusPermaban
	account.IsShadowbanned = newStatus == models.StatusShadowban
	account.LastSuccessfulCheck = now

	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
		return
	}

	ban := models.Ban{
		Account:   account,
		Status:    newStatus,
		AccountID: account.ID,
	}

	if newStatus == models.StatusTempban {
		ban.TempBanDuration = calculateBanDuration(now)
		ban.AffectedGames = getAffectedGames(account.SSOCookie)
	}

	if err := database.DB.Create(&ban).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to create ban record for account %s", account.Title)
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - %s", account.Title, EmbedTitleFromStatus(newStatus)),
		Description: getStatusDescription(newStatus, account.Title, ban),
		Color:       GetColorForStatus(newStatus, account.IsExpiredCookie, account.IsCheckDisabled),
		Timestamp:   now.Format(time.RFC3339),
		Fields:      getStatusFields(account, newStatus),
	}

	notificationType := getNotificationType(newStatus)

	err := SendNotification(s, account, embed, fmt.Sprintf("<@%s>", account.UserID), notificationType)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send status update message for account %s", account.Title)
	} else {
		userSettings.LastStatusChangeNotification = now
		if err := database.DB.Save(&userSettings).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to update LastStatusChangeNotification for user %s", account.UserID)
		}
	}

	if newStatus == models.StatusTempban {
		go ScheduleTempBanNotification(s, account, ban.TempBanDuration)
	}
}

func CheckAccounts(s *discordgo.Session) {
	logger.Log.Info("Starting periodic account check")
	var accounts []models.Account
	if err := database.DB.Find(&accounts).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to fetch accounts from the database")
		return
	}

	accountsByUser := make(map[string][]models.Account)
	for _, account := range accounts {
		accountsByUser[account.UserID] = append(accountsByUser[account.UserID], account)
	}

	for userID, userAccounts := range accountsByUser {
		go func(uid string, accounts []models.Account) {
			userSettings, err := GetUserSettings(uid)
			if err != nil {
				logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", uid)
				return
			}

			var accountsToUpdate []models.Account
			var dailyUpdateAccounts []models.Account
			now := time.Now()

			for _, account := range accounts {
				checkInterval := time.Duration(userSettings.CheckInterval) * time.Minute
				lastCheck := time.Unix(account.LastCheck, 0)

				if now.Sub(lastCheck) < checkInterval {
					logger.Log.Infof("Skipping check for account %s (not due yet)", account.Title)
					continue
				}

				if account.IsCheckDisabled {
					logger.Log.Infof("Skipping check for disabled account: %s", account.Title)
					continue
				}

				accountsToUpdate = append(accountsToUpdate, account)

				if now.Sub(time.Unix(account.LastNotification, 0)).Hours() >= userSettings.NotificationInterval {
					dailyUpdateAccounts = append(dailyUpdateAccounts, account)
				}
			}

			for _, account := range accountsToUpdate {
				var captchaAPIKey string
				if userSettings.PreferredCaptchaProvider == "2captcha" {
					captchaAPIKey = userSettings.TwoCaptchaAPIKey
				} else {
					captchaAPIKey = userSettings.EZCaptchaAPIKey
				}
				status, err := CheckAccount(account.SSOCookie, account.UserID, captchaAPIKey)
				if err != nil {
					logger.Log.WithError(err).Errorf("Error checking account %s", account.Title)
					NotifyAdminWithCooldown(s, fmt.Sprintf("Error checking account %s: %v", account.Title, err), 5*time.Minute)
					continue
				}

				previousStatus := account.LastStatus
				account.LastStatus = status
				account.LastCheck = now.Unix()
				if err := database.DB.Save(&account).Error; err != nil {
					logger.Log.WithError(err).Errorf("Failed to update account %s after check", account.Title)
					continue
				}

				if previousStatus != status {
					HandleStatusChange(s, account, status, userSettings)
				}

				// Check for cookie expiration
				if !account.IsExpiredCookie {
					timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
					if err == nil && timeUntilExpiration > 0 && timeUntilExpiration <= time.Duration(cookieExpirationWarning)*time.Hour {
						if err := NotifyCookieExpiringSoon(s, []models.Account{account}); err != nil {
							logger.Log.WithError(err).Errorf("Failed to send cookie expiration notification for account %s", account.Title)
						}
					}
				}
			}

			if len(dailyUpdateAccounts) > 0 {
				SendConsolidatedDailyUpdate(s, userID, userSettings, dailyUpdateAccounts)
			}

		}(userID, userAccounts)
	}
}

func CheckAndSendNotifications(s *discordgo.Session, userID string) {
	var userSettings models.UserSettings
	if err := database.DB.Where("user_id = ?", userID).First(&userSettings).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", userID)
		return
	}

	now := time.Now()

	if now.Sub(userSettings.LastDailyUpdateNotification) >= time.Duration(userSettings.NotificationInterval)*time.Hour {
		SendConsolidatedDailyUpdate(s, userID, userSettings, nil)
	}

	var accounts []models.Account
	if err := database.DB.Where("user_id = ?", userID).Find(&accounts).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to fetch accounts for user %s", userID)
		return
	}

	for _, account := range accounts {
		if !account.IsExpiredCookie {
			timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
			if err != nil {
				logger.Log.WithError(err).Errorf("Error checking SSO cookie expiration for account %s", account.Title)
				continue
			}
			if timeUntilExpiration > 0 && timeUntilExpiration <= time.Duration(cookieExpirationWarning)*time.Hour {
				if err := NotifyCookieExpiringSoon(s, []models.Account{account}); err != nil {
					logger.Log.WithError(err).Errorf("Failed to send cookie expiration notification for account %s", account.Title)
				}
			}
		}
	}
}

func CheckAndNotifyCookieExpiration(s *discordgo.Session, account models.Account) error {
	timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
	if err != nil {
		return fmt.Errorf("failed to check SSO cookie expiration: %w", err)
	}

	if timeUntilExpiration > 0 && timeUntilExpiration <= time.Duration(cookieExpirationWarning)*time.Hour {
		return NotifyCookieExpiringSoon(s, []models.Account{account})
	}

	return nil
}

func checkCookieExpirations(s *discordgo.Session, userID string, userSettings models.UserSettings) {
	var accounts []models.Account
	if err := database.DB.Where("user_id = ?", userID).Find(&accounts).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to fetch accounts for user %s", userID)
		return
	}

	var expiringAccounts []models.Account

	for _, account := range accounts {
		if !account.IsExpiredCookie {
			timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
			if err == nil && timeUntilExpiration > 0 && timeUntilExpiration <= time.Duration(cookieExpirationWarning)*time.Hour {
				expiringAccounts = append(expiringAccounts, account)
			}
		}
	}

	if len(expiringAccounts) > 0 {
		sendConsolidatedCookieExpirationWarning(s, userID, expiringAccounts, userSettings)
	}
}

func processUserAccounts(s *discordgo.Session, userID string, accounts []models.Account) {
	captchaAPIKey, balance, err := GetUserCaptchaKey(userID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to get user captcha key for user %s", userID)
		return
	}

	logger.Log.Infof("User %s captcha balance: %.2f", userID, balance)

	userSettings, err := GetUserSettings(userID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", userID)
		return
	}

	var accountsToUpdate []models.Account
	var dailyUpdateAccounts []models.Account
	var cookieExpiringAccounts []models.Account
	now := time.Now()

	for _, account := range accounts {
		if account.IsCheckDisabled || account.IsPermabanned {
			logger.Log.Infof("Skipping check for disabled account: %s", account.Title)
			continue
		}

		checkInterval := userSettings.CheckInterval
		lastCheck := time.Unix(account.LastCheck, 0)
		if now.Sub(lastCheck).Minutes() >= float64(checkInterval) {
			accountsToUpdate = append(accountsToUpdate, account)
		} else {
			logger.Log.Infof("Skipping check for account %s (not due yet)", account.Title)
		}

		if now.Sub(time.Unix(account.LastNotification, 0)).Hours() >= userSettings.NotificationInterval {
			dailyUpdateAccounts = append(dailyUpdateAccounts, account)
		}

		if !account.IsExpiredCookie {
			timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
			if err == nil && timeUntilExpiration > 0 && timeUntilExpiration <= time.Duration(cookieExpirationWarning)*time.Hour {
				cookieExpiringAccounts = append(cookieExpiringAccounts, account)
			}
		}
	}

	for _, account := range accountsToUpdate {
		go CheckSingleAccount(s, account, captchaAPIKey)
	}

	if len(dailyUpdateAccounts) > 0 {
		go SendConsolidatedDailyUpdate(s, userID, userSettings, dailyUpdateAccounts)
	}

	if len(cookieExpiringAccounts) > 0 {
		go func() {
			if err := NotifyCookieExpiringSoon(s, cookieExpiringAccounts); err != nil {
				logger.Log.WithError(err).Error("Failed to send cookie expiration notifications")
			}
		}()
	}

	CheckAndNotifyBalance(s, userID, balance)
}

func handlePermabannedAccount(s *discordgo.Session, account models.Account) {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Permanent Ban Status", account.Title),
		Description: fmt.Sprintf("The account %s is still permanently banned. Please remove this account from monitoring using the /removeaccount command.", account.Title),
		Color:       GetColorForStatus(models.StatusPermaban, false, account.IsCheckDisabled),
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	err := SendNotification(s, account, embed, fmt.Sprintf("<@%s>", account.UserID), "permaban")
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

func CheckSingleAccount(s *discordgo.Session, account models.Account, captchaAPIKey string) {
	logger.Log.Infof("Checking account: %s", account.Title)

	if account.IsCheckDisabled {
		logger.Log.Infof("Account %s is disabled. Reason: %s", account.Title, account.DisabledReason)
		return
	}

	timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check SSO cookie expiration for account %s", account.Title)
		handleCheckAccountError(s, account, err)
		return
	} else if timeUntilExpiration > 0 && timeUntilExpiration <= 24*time.Hour {
		if err := NotifyCookieExpiringSoon(s, []models.Account{account}); err != nil {
			logger.Log.WithError(err).Errorf("Failed to send cookie expiration notification for account %s", account.Title)
		}
	}

	result, err := CheckAccount(account.SSOCookie, account.UserID, captchaAPIKey)
	if err != nil {
		logger.Log.WithError(err).Errorf("Error checking account %s", account.Title)
		handleCheckAccountError(s, account, err)
		NotifyAdminWithCooldown(s, fmt.Sprintf("Error checking account %s: %v", account.Title, err), 5*time.Minute)
		return
	}

	account.ConsecutiveErrors = 0
	account.LastSuccessfulCheck = time.Now()

	updateAccountStatus(s, account, result)
}

func handleCheckAccountError(s *discordgo.Session, account models.Account, err error) {
	account.ConsecutiveErrors++
	account.LastErrorTime = time.Now()

	switch {
	case strings.Contains(err.Error(), "Missing Access") || strings.Contains(err.Error(), "Unknown Channel"):
		disableAccount(s, account, "Bot removed from server/channel")
	case strings.Contains(err.Error(), "insufficient balance"):
		disableAccount(s, account, "Insufficient captcha balance")
	case strings.Contains(err.Error(), "invalid captcha API key"):
		disableAccount(s, account, "Invalid captcha API key")
	default:
		if account.ConsecutiveErrors >= maxConsecutiveErrors {
			disableAccount(s, account, fmt.Sprintf("Too many consecutive errors: %v", err))
		} else {
			logger.Log.WithError(err).Errorf("Failed to check account %s: possible expired SSO Cookie", account.Title)
			notifyUserOfCheckError(s, account, err)
		}
	}

	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to update account %s after error", account.Title)
	}
}

func disableAccount(s *discordgo.Session, account models.Account, reason string) {
	account.IsCheckDisabled = true
	account.DisabledReason = reason

	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to disable account %s", account.Title)
		return
	}

	logger.Log.Infof("Account %s has been disabled. Reason: %s", account.Title, reason)

	NotifyUserAboutDisabledAccount(s, account, reason)
}

func updateAccountStatus(s *discordgo.Session, account models.Account, result models.Status) {
	DBMutex.Lock()
	defer DBMutex.Unlock()

	lastStatus := account.LastStatus
	now := time.Now()
	account.LastCheck = now.Unix()
	account.IsExpiredCookie = false
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
		return
	}

	if result != lastStatus {
		lastStatusChange := time.Unix(account.LastStatusChange, 0)
		if now.Sub(lastStatusChange).Hours() >= statusChangeCooldown {
			userSettings, _ := GetUserSettings(account.UserID)
			HandleStatusChange(s, account, result, userSettings)
		} else {
			logger.Log.Infof("Skipping status change notification for account %s (cooldown)", account.Title)
		}
	}
}

func ScheduleTempBanNotification(s *discordgo.Session, account models.Account, duration string) {
	parts := strings.Split(duration, ",")
	if len(parts) != 2 {
		logger.Log.Errorf("Invalid duration format for account %s: %s", account.Title, duration)
		return
	}
	days, _ := strconv.Atoi(strings.TrimSpace(strings.Split(parts[0], " ")[0]))
	hours, _ := strconv.Atoi(strings.TrimSpace(strings.Split(parts[1], " ")[0]))

	sleepDuration := time.Duration(days)*24*time.Hour + time.Duration(hours)*time.Hour

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
		err := SendNotification(s, account, embed, "", "temp_ban_update")
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send temporary ban update for account %s", account.Title)
		}
	}

	result, err := CheckAccount(account.SSOCookie, account.UserID, "")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check account %s after temporary ban duration", account.Title)
		return
	}

	var embed *discordgo.MessageEmbed
	if result == models.StatusGood {
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Lifted", account.Title),
			Description: fmt.Sprintf("The temporary ban for account %s has been lifted. The account is now in good standing.", account.Title),
			Color:       GetColorForStatus(result, false, account.IsCheckDisabled),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	} else if result == models.StatusPermaban {
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Escalated", account.Title),
			Description: fmt.Sprintf("The temporary ban for account %s has been escalated to a permanent ban.", account.Title),
			Color:       GetColorForStatus(result, false, account.IsCheckDisabled),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	} else {
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Update", account.Title),
			Description: fmt.Sprintf("The temporary ban for account %s is still in effect. Current status: %s", account.Title, result),
			Color:       GetColorForStatus(result, false, account.IsCheckDisabled),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	}

	err = SendNotification(s, account, embed, fmt.Sprintf("<@%s>", account.UserID), "temp_ban_update")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send temporary ban update message for account %s", account.Title)
	}
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

func calculateBanDuration(banEndTime time.Time) string {
	duration := time.Until(banEndTime)
	if duration < 0 {
		duration = 0
	}
	days := int(duration.Hours()) / 24
	hours := int(duration.Hours()) % 24
	return fmt.Sprintf("%d days, %d hours", days, hours)
}
