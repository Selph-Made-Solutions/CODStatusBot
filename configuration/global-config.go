package configuration

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/bradselph/CODStatusBot/logger"
)

type Config struct {
	// Environment
	Environment string
	LogDir      string

	// Database Settings
	Database struct {
		User     string
		Password string
		Name     string
		Host     string
		Port     string
		Var      string
	}

	// Discord Settings
	Discord struct {
		Token       string
		DeveloperID string
	}

	// Captcha Service Settings
	CaptchaService struct {
		Capsolver struct {
			Enabled       bool
			ClientKey     string
			AppID         string
			BalanceMin    float64
			MaxRetries    int
			RetryInterval time.Duration
		}
		EZCaptcha struct {
			Enabled    bool
			ClientKey  string
			AppID      string
			BalanceMin float64
		}
		TwoCaptcha struct {
			Enabled    bool
			ClientKey  string
			SoftID     string
			BalanceMin float64
		}
		RecaptchaSiteKey string
		RecaptchaURL     string
		MaxRetries       int
	}

	CaptchaEndpoints struct {
		Capsolver struct {
			Create string
			Result string
		}
		EZCaptcha struct {
			Create string
			Result string
		}
		TwoCaptcha struct {
			Create string
			Result string
		}
		MaxRetries    int
		RetryInterval time.Duration
	}

	// API Endpoints
	API struct {
		CheckEndpoint      string
		ProfileEndpoint    string
		CheckVIPEndpoint   string
		RedeemCodeEndpoint string
	}

	// Admin Panel Settings
	AdminPanel struct {
		SessionKey     string
		StaticDir      string
		TemplatesDir   string
		Port           string
		Username       string
		Password       string
		StatsRateLimit float64
	}

	// Rate Limits and Intervals
	RateLimits struct {
		CheckNow           time.Duration
		Default            time.Duration
		DefaultMaxAccounts int
		PremiumMaxAccounts int
	}

	// Intervals
	Intervals struct {
		Check              int
		Notification       float64
		Cooldown           float64
		Sleep              int
		PermaBanCheck      float64
		StatusChange       float64
		GlobalNotification float64
		CookieExpiration   float64
		TempBanUpdate      float64
	}

	// Emoji Settings
	Emojis struct {
		CheckCircle    string
		BanCircle      string
		InfoCircle     string
		StopWatch      string
		QuestionCircle string
	}

	Donations struct {
		Enabled        bool
		BitcoinAddress string
		CashAppID      string
	}
}

type DonationsConfig struct {
	Enabled        bool
	BitcoinAddress string
	CashAppID      string
}

var AppConfig Config

func Load() error {
	logger.Log.Info("Loading configuration...")

	// Environment
	AppConfig.Environment = getEnvWithDefault("ENVIRONMENT", "development")
	AppConfig.LogDir = getEnvWithDefault("LOG_DIR", "logs")

	// Database Settings
	AppConfig.Database.User = os.Getenv("DB_USER")
	AppConfig.Database.Password = os.Getenv("DB_PASSWORD")
	AppConfig.Database.Name = os.Getenv("DB_NAME")
	AppConfig.Database.Host = os.Getenv("DB_HOST")
	AppConfig.Database.Port = os.Getenv("DB_PORT")
	AppConfig.Database.Var = os.Getenv("DB_VAR")

	// Discord Settings
	AppConfig.Discord.Token = os.Getenv("DISCORD_TOKEN")
	AppConfig.Discord.DeveloperID = os.Getenv("DEVELOPER_ID")

	// Captcha Service Settings
	AppConfig.CaptchaService.Capsolver.Enabled = os.Getenv("CAPSOLVER_ENABLED") == "true"
	AppConfig.CaptchaService.Capsolver.ClientKey = os.Getenv("CAPSOLVER_CLIENT_KEY")
	AppConfig.CaptchaService.Capsolver.AppID = os.Getenv("CAPSOLVER_APP_ID")
	AppConfig.CaptchaService.Capsolver.BalanceMin = getEnvAsFloat("CAPSOLVER_BALANCE_MIN", 0.10)
	AppConfig.CaptchaService.Capsolver.MaxRetries = getEnvAsInt("CAPSOLVER_MAX_RETRIES", 6)                                     // TODO: Merge with MAX_RETRIES
	AppConfig.CaptchaService.Capsolver.RetryInterval = time.Duration(getEnvAsInt("CAPSOLVER_RETRY_INTERVAL", 10)) * time.Second // TODO: Merge with RETRY_INTERVAL

	AppConfig.CaptchaService.EZCaptcha.Enabled = os.Getenv("EZCAPTCHA_ENABLED") == "true"
	AppConfig.CaptchaService.EZCaptcha.ClientKey = os.Getenv("EZCAPTCHA_CLIENT_KEY")
	AppConfig.CaptchaService.EZCaptcha.AppID = os.Getenv("EZAPPID")
	AppConfig.CaptchaService.EZCaptcha.BalanceMin = getEnvAsFloat("EZCAPBALMIN", 50)

	AppConfig.CaptchaService.TwoCaptcha.Enabled = os.Getenv("TWOCAPTCHA_ENABLED") == "true"
	AppConfig.CaptchaService.TwoCaptcha.SoftID = os.Getenv("SOFT_ID")
	AppConfig.CaptchaService.TwoCaptcha.BalanceMin = getEnvAsFloat("TWOCAPBALMIN", 0.10)

	AppConfig.CaptchaService.RecaptchaSiteKey = os.Getenv("RECAPTCHA_SITE_KEY")
	AppConfig.CaptchaService.RecaptchaURL = os.Getenv("RECAPTCHA_URL")
	AppConfig.CaptchaService.MaxRetries = getEnvAsInt("MAX_RETRIES", 3)

	// API Endpoints
	AppConfig.API.CheckEndpoint = os.Getenv("CHECK_ENDPOINT")
	AppConfig.API.ProfileEndpoint = os.Getenv("PROFILE_ENDPOINT")
	AppConfig.API.CheckVIPEndpoint = os.Getenv("CHECK_VIP_ENDPOINT")
	AppConfig.API.RedeemCodeEndpoint = os.Getenv("REDEEM_CODE_ENDPOINT")

	// Admin Panel Settings
	AppConfig.AdminPanel.SessionKey = os.Getenv("SESSION_KEY")
	AppConfig.AdminPanel.StaticDir = getEnvWithDefault("STATIC_DIR", "./static")
	AppConfig.AdminPanel.TemplatesDir = getEnvWithDefault("TEMPLATES_DIR", "templates")
	AppConfig.AdminPanel.Port = os.Getenv("ADMIN_PORT")
	AppConfig.AdminPanel.Username = os.Getenv("ADMIN_USERNAME")
	AppConfig.AdminPanel.Password = os.Getenv("ADMIN_PASSWORD")
	AppConfig.AdminPanel.StatsRateLimit = getEnvAsFloat("STATS_RATE_LIMIT", 25.0)

	// Rate Limits
	AppConfig.RateLimits.CheckNow = time.Duration(getEnvAsInt("CHECK_NOW_RATE_LIMIT", 3600)) * time.Second
	AppConfig.RateLimits.Default = time.Duration(getEnvAsInt("DEFAULT_RATE_LIMIT", 180)) * time.Minute
	AppConfig.RateLimits.DefaultMaxAccounts = getEnvAsInt("DEFAULT_USER_MAXACCOUNTS", 3)
	AppConfig.RateLimits.PremiumMaxAccounts = getEnvAsInt("PREM_USER_MAXACCOUNTS", 10)

	// Intervals
	AppConfig.Intervals.Check = getEnvAsInt("CHECK_INTERVAL", 15)
	AppConfig.Intervals.Notification = getEnvAsFloat("NOTIFICATION_INTERVAL", 24)
	AppConfig.Intervals.Cooldown = getEnvAsFloat("COOLDOWN_DURATION", 6)
	AppConfig.Intervals.Sleep = getEnvAsInt("SLEEP_DURATION", 1)
	AppConfig.Intervals.PermaBanCheck = getEnvAsFloat("COOKIE_CHECK_INTERVAL_PERMABAN", 24)
	AppConfig.Intervals.StatusChange = getEnvAsFloat("STATUS_CHANGE_COOLDOWN", 1)
	AppConfig.Intervals.GlobalNotification = getEnvAsFloat("GLOBAL_NOTIFICATION_COOLDOWN", 2)
	AppConfig.Intervals.CookieExpiration = getEnvAsFloat("COOKIE_EXPIRATION_WARNING", 24)
	AppConfig.Intervals.TempBanUpdate = getEnvAsFloat("TEMP_BAN_UPDATE_INTERVAL", 24)

	// Emoji Settings
	AppConfig.Emojis.CheckCircle = os.Getenv("CHECKCIRCLE")
	AppConfig.Emojis.BanCircle = os.Getenv("BANCIRCLE")
	AppConfig.Emojis.InfoCircle = os.Getenv("INFOCIRCLE")
	AppConfig.Emojis.StopWatch = os.Getenv("STOPWATCH")
	AppConfig.Emojis.QuestionCircle = os.Getenv("QUESTIONCIRCLE")

	// Donation Settings
	AppConfig.Donations.Enabled = os.Getenv("DONATIONS_ENABLED") == "true"
	AppConfig.Donations.BitcoinAddress = os.Getenv("BITCOIN_ADDRESS")
	AppConfig.Donations.CashAppID = os.Getenv("CASHAPP_ID")

	if err := validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	logger.Log.Infof("Loaded rate limits and intervals: CHECK_INTERVAL=%d, NOTIFICATION_INTERVAL=%.2f, "+
		"COOLDOWN_DURATION=%.2f, SLEEP_DURATION=%d, COOKIE_CHECK_INTERVAL_PERMABAN=%.2f, "+
		"STATUS_CHANGE_COOLDOWN=%.2f, GLOBAL_NOTIFICATION_COOLDOWN=%.2f, COOKIE_EXPIRATION_WARNING=%.2f, "+
		"TEMP_BAN_UPDATE_INTERVAL=%.2f, CHECK_NOW_RATE_LIMIT=%v, DEFAULT_RATE_LIMIT=%v",
		AppConfig.Intervals.Check,
		AppConfig.Intervals.Notification,
		AppConfig.Intervals.Cooldown,
		AppConfig.Intervals.Sleep,
		AppConfig.Intervals.PermaBanCheck,
		AppConfig.Intervals.StatusChange,
		AppConfig.Intervals.GlobalNotification,
		AppConfig.Intervals.CookieExpiration,
		AppConfig.Intervals.TempBanUpdate,
		AppConfig.RateLimits.CheckNow,
		AppConfig.RateLimits.Default)

	return nil
}

func validate() error {
	required := map[string]string{
		"DISCORD_TOKEN":      AppConfig.Discord.Token,
		"DEVELOPER_ID":       AppConfig.Discord.DeveloperID,
		"DB_USER":            AppConfig.Database.User,
		"DB_PASSWORD":        AppConfig.Database.Password,
		"DB_NAME":            AppConfig.Database.Name,
		"DB_HOST":            AppConfig.Database.Host,
		"DB_PORT":            AppConfig.Database.Port,
		"PROFILE_ENDPOINT":   AppConfig.API.ProfileEndpoint,
		"CHECK_VIP_ENDPOINT": AppConfig.API.CheckVIPEndpoint,
		"CHECK_ENDPOINT":     AppConfig.API.CheckEndpoint,
	}

	var missing []string
	for key, value := range required {
		if value == "" {
			val := os.Getenv(key)
			if val == "" {
				missing = append(missing, key)
			} else {
				switch key {
				case "DISCORD_TOKEN":
					AppConfig.Discord.Token = val
				case "DEVELOPER_ID":
					AppConfig.Discord.DeveloperID = val
				case "DB_USER":
					AppConfig.Database.User = val
				case "DB_PASSWORD":
					AppConfig.Database.Password = val
				case "DB_NAME":
					AppConfig.Database.Name = val
				case "DB_HOST":
					AppConfig.Database.Host = val
				case "DB_PORT":
					AppConfig.Database.Port = val
				case "CHECK_ENDPOINT":
					AppConfig.API.CheckEndpoint = val
				case "PROFILE_ENDPOINT":
					AppConfig.API.ProfileEndpoint = val
				case "CHECK_VIP_ENDPOINT":
					AppConfig.API.CheckVIPEndpoint = val
				}
			}
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missing)
	}

	return nil
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
		logger.Log.WithField("key", key).WithField("default", defaultValue).
			Error("Failed to parse integer from environment variable")
	}
	return defaultValue
}

func getEnvAsFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
		logger.Log.WithField("key", key).WithField("default", defaultValue).
			Error("Failed to parse float from environment variable")
	}
	return defaultValue
}

func Get() *Config {
	return &AppConfig
}

func IsDonationsEnabled() bool {
	return AppConfig.Donations.Enabled
}

func GetDefaultSettings() struct {
	CheckInterval        int
	NotificationInterval float64
	CooldownDuration     float64
	StatusChangeCooldown float64
} {
	return struct {
		CheckInterval        int
		NotificationInterval float64
		CooldownDuration     float64
		StatusChangeCooldown float64
	}{
		CheckInterval:        AppConfig.Intervals.Check,
		NotificationInterval: AppConfig.Intervals.Notification,
		CooldownDuration:     AppConfig.Intervals.Cooldown,
		StatusChangeCooldown: AppConfig.Intervals.StatusChange,
	}
}
