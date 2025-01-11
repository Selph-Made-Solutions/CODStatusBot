package services

import (
	"fmt"
	"time"

	"github.com/bradselph/CODStatusBot/configuration"
	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
)

var defaultSettings models.UserSettings

func initDefaultSettings() {
	cfg := configuration.Get()
	defaultSettings = models.UserSettings{
		CheckInterval:            cfg.Intervals.Check,
		NotificationInterval:     cfg.Intervals.Notification,
		CooldownDuration:         cfg.Intervals.Cooldown,
		StatusChangeCooldown:     cfg.Intervals.StatusChange,
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

	if defaultSettings.CheckInterval == 0 {
		initDefaultSettings()
	}

	if settings.LastDailyUpdateNotification.IsZero() {
		settings.LastDailyUpdateNotification = time.Now().Add(-24 * time.Hour)
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

	settings.EnsureMapsInitialized()

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

	cfg := configuration.Get()

	switch settings.PreferredCaptchaProvider {
	case "2captcha":
		if !cfg.CaptchaService.TwoCaptcha.Enabled {
			logger.Log.Warn("Attempt to use disabled 2captcha service")
			if cfg.CaptchaService.EZCaptcha.Enabled {
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
		if !cfg.CaptchaService.EZCaptcha.Enabled {
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

	if settings.PreferredCaptchaProvider == "ezcaptcha" && cfg.CaptchaService.EZCaptcha.Enabled {
		defaultKey := cfg.CaptchaService.EZCaptcha.ClientKey
		isValid, balance, err := ValidateCaptchaKey(defaultKey, "ezcaptcha")
		if err != nil {
			return "", 0, err
		}
		if !isValid {
			return "", 0, fmt.Errorf("invalid default ezcaptcha API key")
		}
		return defaultKey, balance, nil
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
	if defaultSettings.CheckInterval == 0 {
		initDefaultSettings()
	}
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

	cfg := configuration.Get()
	defaultMax := cfg.RateLimits.DefaultMaxAccounts

	settings.EZCaptchaAPIKey = ""
	settings.TwoCaptchaAPIKey = ""
	settings.PreferredCaptchaProvider = "ezcaptcha"
	settings.CustomSettings = false
	settings.CheckInterval = defaultSettings.CheckInterval
	settings.NotificationInterval = defaultSettings.NotificationInterval
	settings.CooldownDuration = defaultSettings.CooldownDuration
	settings.StatusChangeCooldown = defaultSettings.StatusChangeCooldown

	settings.EnsureMapsInitialized()

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
