package services

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
)

var defaultSettings models.UserSettings

func init() {
	checkInterval, err := strconv.Atoi(os.Getenv("CHECK_INTERVAL"))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse CHECK_INTERVAL, using default of 15 minutes")
		checkInterval = 15
	}

	defaultInterval, err := strconv.ParseFloat(os.Getenv("NOTIFICATION_INTERVAL"), 64)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse NOTIFICATION_INTERVAL from .env, using default of 24 hours")
		defaultInterval = 24
	}

	cooldownDuration, err := strconv.ParseFloat(os.Getenv("COOLDOWN_DURATION"), 64)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse COOLDOWN_DURATION, using default of 6 hours")
		cooldownDuration = 6
	}

	statusChangeCooldown, err := strconv.ParseFloat(os.Getenv("STATUS_CHANGE_COOLDOWN"), 64)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse STATUS_CHANGE_COOLDOWN, using default of 1 hour")
		statusChangeCooldown = 1
	}

	defaultSettings = models.UserSettings{
		CheckInterval:            checkInterval,
		NotificationInterval:     defaultInterval,
		CooldownDuration:         cooldownDuration,
		StatusChangeCooldown:     statusChangeCooldown,
		NotificationType:         "channel",
		PreferredCaptchaProvider: "ezcaptcha",
	}

	logger.Log.Infof("Default settings loaded: CheckInterval=%d, NotificationInterval=%.2f, CooldownDuration=%.2f, StatusChangeCooldown=%.2f",
		defaultSettings.CheckInterval, defaultSettings.NotificationInterval, defaultSettings.CooldownDuration, defaultSettings.StatusChangeCooldown)
}

func GetUserSettings(userID string) (models.UserSettings, error) {
	logger.Log.Infof("Getting user settings for user: %s", userID)
	var settings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&settings)
	if result.Error != nil {
		return models.UserSettings{}, fmt.Errorf("error getting user settings: %w", result.Error)
	}

	if settings.CheckInterval == 0 {
		settings.CheckInterval = defaultSettings.CheckInterval
	}
	if settings.NotificationInterval == 0 {
		settings.NotificationInterval = defaultSettings.NotificationInterval
	}
	if settings.CooldownDuration == 0 {
		settings.CooldownDuration = defaultSettings.CooldownDuration
	}
	if settings.StatusChangeCooldown == 0 {
		settings.StatusChangeCooldown = defaultSettings.StatusChangeCooldown
	}
	if settings.NotificationType == "" {
		settings.NotificationType = defaultSettings.NotificationType
	}
	if settings.PreferredCaptchaProvider == "" {
		settings.PreferredCaptchaProvider = defaultSettings.PreferredCaptchaProvider
	}

	if settings.NotificationTimes == nil {
		settings.NotificationTimes = make(map[string]time.Time)
	}
	if settings.ActionCounts == nil {
		settings.ActionCounts = make(map[string]int)
	}
	if settings.LastActionTimes == nil {
		settings.LastActionTimes = make(map[string]time.Time)
	}
	if settings.LastCommandTimes == nil {
		settings.LastCommandTimes = make(map[string]time.Time)
	}
	if settings.RateLimitExpiration == nil {
		settings.RateLimitExpiration = make(map[string]time.Time)
	}

	if result.RowsAffected > 0 {
		if err := database.DB.Save(&settings).Error; err != nil {
			return settings, fmt.Errorf("error saving default settings: %w", err)
		}
	}

	logger.Log.Infof("Got user settings for user: %s", userID)
	return settings, nil
}

func GetUserCaptchaKey(userID string) (string, float64, error) {
	var settings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).First(&settings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings")
		return "", 0, result.Error
	}

	switch settings.PreferredCaptchaProvider {
	case "2captcha":
		if !IsServiceEnabled("2captcha") {
			logger.Log.Warn("Attempt to use disabled 2captcha service")
			if IsServiceEnabled("ezcaptcha") {
				settings.PreferredCaptchaProvider = "ezcaptcha"
				if settings.EZCaptchaAPIKey != "" {
					isValid, balance, err := ValidateCaptchaKey(settings.EZCaptchaAPIKey, "ezcaptcha")
					if err != nil {
						return "", 0, err
					}
					if !isValid {
						return "", 0, fmt.Errorf("invalid ezcaptcha API key")
					}
					database.DB.Save(&settings)
					return settings.EZCaptchaAPIKey, balance, nil
				}
			}
			return "", 0, fmt.Errorf("2captcha service is currently disabled")
		}
		if settings.TwoCaptchaAPIKey != "" {
			isValid, balance, err := ValidateCaptchaKey(settings.TwoCaptchaAPIKey, "2captcha")
			if err != nil {
				return "", 0, err
			}
			if !isValid {
				return "", 0, fmt.Errorf("invalid 2captcha API key")
			}
			return settings.TwoCaptchaAPIKey, balance, nil
		}
	case "ezcaptcha":
		if !IsServiceEnabled("ezcaptcha") {
			return "", 0, fmt.Errorf("ezcaptcha service is currently disabled")
		}
		if settings.EZCaptchaAPIKey != "" {
			isValid, balance, err := ValidateCaptchaKey(settings.EZCaptchaAPIKey, "ezcaptcha")
			if err != nil {
				return "", 0, err
			}
			if !isValid {
				return "", 0, fmt.Errorf("invalid ezcaptcha API key")
			}
			return settings.EZCaptchaAPIKey, balance, nil
		}
	}

	if settings.PreferredCaptchaProvider == "ezcaptcha" && IsServiceEnabled("ezcaptcha") {
		defaultKey := os.Getenv("EZCAPTCHA_CLIENT_KEY")
		isValid, _, err := ValidateCaptchaKey(defaultKey, "ezcaptcha")
		if err != nil {
			return "", 0, err
		}
		if !isValid {
			return "", 0, fmt.Errorf("invalid default ezcaptcha API key")
		}
		return defaultKey, 0, nil
	}

	return "", 0, fmt.Errorf("no valid API key found for provider %s", settings.PreferredCaptchaProvider)
}

func GetCaptchaSolver(userID string) (CaptchaSolver, error) {
	settings, err := GetUserSettings(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user settings: %w", err)
	}

	apiKey, _, err := GetUserCaptchaKey(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user captcha key: %w", err)
	}

	return NewCaptchaSolver(apiKey, settings.PreferredCaptchaProvider)
}

func GetDefaultSettings() (models.UserSettings, error) {
	return defaultSettings, nil
}

func RemoveCaptchaKey(userID string) error {
	var settings models.UserSettings
	result := database.DB.Where("user_id = ?", userID).First(&settings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error removing apikey in settings")
		return result.Error
	}

	hadCustomKey := settings.EZCaptchaAPIKey != "" || settings.TwoCaptchaAPIKey != ""

	var accountCount int64
	if err := database.DB.Model(&models.Account{}).Where("user_id = ?", userID).Count(&accountCount).Error; err != nil {
		return fmt.Errorf("failed to count user accounts: %w", err)
	}

	defaultMax, err := strconv.Atoi(os.Getenv("DEFAULT_USER_MAXACCOUNTS"))
	if err != nil || defaultMax <= 0 {
		defaultMax = 3
	}

	settings.EZCaptchaAPIKey = ""
	settings.TwoCaptchaAPIKey = ""
	settings.PreferredCaptchaProvider = "ezcaptcha"
	settings.CustomSettings = false
	settings.CheckInterval = defaultSettings.CheckInterval
	settings.NotificationInterval = defaultSettings.NotificationInterval
	settings.CooldownDuration = defaultSettings.CooldownDuration
	settings.StatusChangeCooldown = defaultSettings.StatusChangeCooldown

	if settings.NotificationTimes == nil {
		settings.NotificationTimes = make(map[string]time.Time)
	}
	if settings.ActionCounts == nil {
		settings.ActionCounts = make(map[string]int)
	}
	if settings.LastActionTimes == nil {
		settings.LastActionTimes = make(map[string]time.Time)
	}
	if settings.LastCommandTimes == nil {
		settings.LastCommandTimes = make(map[string]time.Time)
	}
	if settings.RateLimitExpiration == nil {
		settings.RateLimitExpiration = make(map[string]time.Time)
	}

	settings.LastCommandTimes["api_key_removed"] = time.Now()

	if err := database.DB.Save(&settings).Error; err != nil {
		logger.Log.WithError(err).Error("Error saving user settings")
		return err
	}

	if hadCustomKey {
		logger.Log.Infof("Removed custom captcha key and reset settings for user: %s", userID)
	} else {
		logger.Log.Infof("Reset settings to default for user: %s (no custom key was present)", userID)
	}

	if int64(defaultMax) < accountCount {
		var accounts []models.Account
		if err := database.DB.Where("user_id = ?", userID).Find(&accounts).Error; err != nil {
			logger.Log.WithError(err).Error("Error fetching user accounts while removing API key")
			return err
		}

		for _, account := range accounts {
			account.NotificationType = defaultSettings.NotificationType
			if err := database.DB.Save(&account).Error; err != nil {
				logger.Log.WithError(err).Errorf("Error updating account %s settings after API key removal", account.Title)
			}
		}

		embed := &discordgo.MessageEmbed{
			Title: "Account Limit Warning",
			Description: fmt.Sprintf("You currently have %d accounts monitored, which exceeds the default limit of %d accounts. "+
				"You will not be able to add new accounts until you remove some existing ones or add a custom API key.",
				accountCount, defaultMax),
			Color: 0xFFA500,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Current Accounts",
					Value:  fmt.Sprintf("%d", accountCount),
					Inline: true,
				},
				{
					Name:   "Default Limit",
					Value:  fmt.Sprintf("%d", defaultMax),
					Inline: true,
				},
				{
					Name:   "Action Required",
					Value:  "Please remove excess accounts or add a custom API key using /setcaptchaservice",
					Inline: false,
				},
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}

		if len(accounts) > 0 {
			if err := SendNotification(nil, accounts[0], embed, "", "api_key_removal_warning"); err != nil {
				logger.Log.WithError(err).Error("Failed to send API key removal warning")
			}
		}
	}

	return nil
}
