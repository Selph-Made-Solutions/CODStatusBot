package configuration

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
}

var AppConfig Config

func Load() error {
	logger.Log.Info("Loading configuration...")

	AppConfig.Environment = getEnvWithDefault("ENVIRONMENT", "development")
	AppConfig.LogDir = getEnvWithDefault("LOG_DIR", "logs")

	requiredDbFields := map[string]*string{
		"DB_USER":     &AppConfig.Database.User,
		"DB_PASSWORD": &AppConfig.Database.Password,
		"DB_NAME":     &AppConfig.Database.Name,
		"DB_HOST":     &AppConfig.Database.Host,
		"DB_PORT":     &AppConfig.Database.Port,
		"DB_VAR":      &AppConfig.Database.Var,
	}

	for env, field := range requiredDbFields {
		*field = os.Getenv(env)
	}

	AppConfig.Discord.Token = os.Getenv("DISCORD_TOKEN")
	AppConfig.Discord.DeveloperID = os.Getenv("DEVELOPER_ID")
	loadCaptchaConfig()
	loadAPIEndpoints()

	loadRateLimits()
	loadIntervals()
	loadEmojiConfig()

	if err := validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	logConfigurationValues()

	return nil
}

func loadCaptchaConfig() {
	// Capsolver
	AppConfig.CaptchaService.Capsolver.Enabled = os.Getenv("CAPSOLVER_ENABLED") == "true"
	AppConfig.CaptchaService.Capsolver.ClientKey = os.Getenv("CAPSOLVER_CLIENT_KEY")
	AppConfig.CaptchaService.Capsolver.AppID = os.Getenv("CAPSOLVER_APP_ID")
	AppConfig.CaptchaService.Capsolver.BalanceMin = getEnvAsFloat("CAPSOLVER_BALANCE_MIN", 0.10)
	AppConfig.CaptchaService.Capsolver.MaxRetries = getEnvAsInt("CAPSOLVER_MAX_RETRIES", 6)                                     // TODO: Merge with MAX_RETRIES
	AppConfig.CaptchaService.Capsolver.RetryInterval = time.Duration(getEnvAsInt("CAPSOLVER_RETRY_INTERVAL", 10)) * time.Second // TODO: Merge with RETRY_INTERVAL

	// EZCaptcha
	AppConfig.CaptchaService.EZCaptcha.Enabled = os.Getenv("EZCAPTCHA_ENABLED") == "true"
	AppConfig.CaptchaService.EZCaptcha.ClientKey = os.Getenv("EZCAPTCHA_CLIENT_KEY")
	AppConfig.CaptchaService.EZCaptcha.AppID = os.Getenv("EZAPPID")
	AppConfig.CaptchaService.EZCaptcha.BalanceMin = getEnvAsFloat("EZCAPBALMIN", 50)

	// 2Captcha
	AppConfig.CaptchaService.TwoCaptcha.Enabled = os.Getenv("TWOCAPTCHA_ENABLED") == "true"
	AppConfig.CaptchaService.TwoCaptcha.SoftID = os.Getenv("SOFT_ID")
	AppConfig.CaptchaService.TwoCaptcha.BalanceMin = getEnvAsFloat("TWOCAPBALMIN", 0.10)

	// Common Captcha Settings
	AppConfig.CaptchaService.RecaptchaSiteKey = os.Getenv("RECAPTCHA_SITE_KEY")
	AppConfig.CaptchaService.RecaptchaURL = os.Getenv("RECAPTCHA_URL")
	AppConfig.CaptchaService.MaxRetries = getEnvAsInt("MAX_RETRIES", 3)
}

func loadAPIEndpoints() {
	AppConfig.API.CheckEndpoint = os.Getenv("CHECK_ENDPOINT")
	AppConfig.API.ProfileEndpoint = os.Getenv("PROFILE_ENDPOINT")
	AppConfig.API.CheckVIPEndpoint = os.Getenv("CHECK_VIP_ENDPOINT")
	AppConfig.API.RedeemCodeEndpoint = os.Getenv("REDEEM_CODE_ENDPOINT")
}

func loadRateLimits() {
	AppConfig.RateLimits.CheckNow = time.Duration(getEnvAsInt("CHECK_NOW_RATE_LIMIT", 3600)) * time.Second
	AppConfig.RateLimits.Default = time.Duration(getEnvAsInt("DEFAULT_RATE_LIMIT", 180)) * time.Minute
	AppConfig.RateLimits.DefaultMaxAccounts = getEnvAsInt("DEFAULT_USER_MAXACCOUNTS", 3)
	AppConfig.RateLimits.PremiumMaxAccounts = getEnvAsInt("PREM_USER_MAXACCOUNTS", 15)
}

func loadIntervals() {
	AppConfig.Intervals.Check = getEnvAsInt("CHECK_INTERVAL", 15)
	AppConfig.Intervals.Notification = getEnvAsFloat("NOTIFICATION_INTERVAL", 24)
	AppConfig.Intervals.Cooldown = getEnvAsFloat("COOLDOWN_DURATION", 6)
	AppConfig.Intervals.Sleep = getEnvAsInt("SLEEP_DURATION", 1)
	AppConfig.Intervals.PermaBanCheck = getEnvAsFloat("COOKIE_CHECK_INTERVAL_PERMABAN", 24)
	AppConfig.Intervals.StatusChange = getEnvAsFloat("STATUS_CHANGE_COOLDOWN", 1)
	AppConfig.Intervals.GlobalNotification = getEnvAsFloat("GLOBAL_NOTIFICATION_COOLDOWN", 2)
	AppConfig.Intervals.CookieExpiration = getEnvAsFloat("COOKIE_EXPIRATION_WARNING", 24)
	AppConfig.Intervals.TempBanUpdate = getEnvAsFloat("TEMP_BAN_UPDATE_INTERVAL", 24)
}

func loadEmojiConfig() {
	AppConfig.Emojis.CheckCircle = os.Getenv("CHECKCIRCLE")
	AppConfig.Emojis.BanCircle = os.Getenv("BANCIRCLE")
	AppConfig.Emojis.InfoCircle = os.Getenv("INFOCIRCLE")
	AppConfig.Emojis.StopWatch = os.Getenv("STOPWATCH")
	AppConfig.Emojis.QuestionCircle = os.Getenv("QUESTIONCIRCLE")
}

func validate() error {
	var missingVars []string

	requiredVars := map[string]string{
		"DISCORD_TOKEN":      AppConfig.Discord.Token,
		"DEVELOPER_ID":       AppConfig.Discord.DeveloperID,
		"DB_USER":            AppConfig.Database.User,
		"DB_PASSWORD":        AppConfig.Database.Password,
		"DB_NAME":            AppConfig.Database.Name,
		"DB_HOST":            AppConfig.Database.Host,
		"DB_PORT":            AppConfig.Database.Port,
		"DB_VAR":             AppConfig.Database.Var,
		"PROFILE_ENDPOINT":   AppConfig.API.ProfileEndpoint,
		"CHECK_VIP_ENDPOINT": AppConfig.API.CheckVIPEndpoint,
		"CHECK_ENDPOINT":     AppConfig.API.CheckEndpoint,
	}

	for key, value := range requiredVars {
		if value == "" {
			missingVars = append(missingVars, key)
		}
	}

	if len(missingVars) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missingVars)
	}

	if AppConfig.CaptchaService.Capsolver.Enabled && AppConfig.CaptchaService.Capsolver.ClientKey == "" {
		return fmt.Errorf("Capsolver is enabled but no client key provided")
	}

	if AppConfig.CaptchaService.EZCaptcha.Enabled && AppConfig.CaptchaService.EZCaptcha.ClientKey == "" {
		return fmt.Errorf("EZCaptcha is enabled but no client key provided")
	}

	if AppConfig.CaptchaService.TwoCaptcha.Enabled && AppConfig.CaptchaService.TwoCaptcha.ClientKey == "" {
		return fmt.Errorf("2Captcha is enabled but no client key provided")
	}

	return nil
}

func logConfigurationValues() {
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

	// Log enabled captcha services
	var enabledServices []string
	if AppConfig.CaptchaService.Capsolver.Enabled {
		enabledServices = append(enabledServices, "Capsolver")
	}
	if AppConfig.CaptchaService.EZCaptcha.Enabled {
		enabledServices = append(enabledServices, "EZCaptcha")
	}
	if AppConfig.CaptchaService.TwoCaptcha.Enabled {
		enabledServices = append(enabledServices, "2Captcha")
	}

	if len(enabledServices) > 0 {
		logger.Log.Infof("Enabled captcha services: %s", strings.Join(enabledServices, ", "))
	} else {
		logger.Log.Warn("No captcha services are enabled")
	}
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
