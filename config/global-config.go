package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/bradselph/CODStatusBot/logger"
)

type Config struct {
	LogDir string

	// Environment
	Environment string

	// Database Settings
	DBUser     string
	DBPassword string
	DBName     string
	DBHost     string
	DBPort     string
	DBVar      string

	// Discord Settings
	DiscordToken string
	DeveloperID  string

	// Captcha Service Settings
	EZCaptchaEnabled   bool
	TwoCaptchaEnabled  bool
	EZCaptchaClientKey string
	EZAppID            string
	SoftID             string
	SiteAction         string
	EZCapBalMin        float64
	TwoCapBalMin       float64
	RecaptchaSiteKey   string
	RecaptchaURL       string
	MaxRetries         int

	// API Endpoints
	CheckEndpoint      string
	ProfileEndpoint    string
	CheckVIPEndpoint   string
	RedeemCodeEndpoint string

	// Admin Panel Settings
	SessionKey     string
	StaticDir      string
	TemplatesDir   string
	AdminPort      string
	AdminUsername  string
	AdminPassword  string
	StatsRateLimit float64

	// Rate Limits and Intervals
	CheckNowRateLimit           time.Duration
	DefaultRateLimit            time.Duration
	DefaultUserMaxAccounts      int
	PremUserMaxAccounts         int
	CheckInterval               int
	NotificationInterval        float64
	CooldownDuration            float64
	SleepDuration               int
	CookieCheckIntervalPermaban float64
	StatusChangeCooldown        float64
	GlobalNotificationCooldown  float64
	CookieExpirationWarning     float64
	TempBanUpdateInterval       float64

	// Emoji Settings
	CheckCircle    string
	BanCircle      string
	InfoCircle     string
	StopWatch      string
	QuestionCircle string

	// Donation Settings
	DonationsEnabled bool
	BitcoinAddress   string
	CashAppID        string
}

var AppConfig Config

func Load() error {
	logger.Log.Info("Loading configuration...")

	// Environment
	AppConfig.Environment = getEnvWithDefault("ENVIRONMENT", "development")

	// Database Settings
	AppConfig.DBUser = os.Getenv("DB_USER")
	AppConfig.DBPassword = os.Getenv("DB_PASSWORD")
	AppConfig.DBName = os.Getenv("DB_NAME")
	AppConfig.DBHost = os.Getenv("DB_HOST")
	AppConfig.DBPort = os.Getenv("DB_PORT")
	AppConfig.DBVar = os.Getenv("DB_VAR")

	// Discord Settings
	AppConfig.DiscordToken = os.Getenv("DISCORD_TOKEN")
	AppConfig.DeveloperID = os.Getenv("DEVELOPER_ID")

	// Captcha Service Settings
	AppConfig.EZCaptchaEnabled = os.Getenv("EZCAPTCHA_ENABLED") == "true"
	AppConfig.TwoCaptchaEnabled = os.Getenv("TWOCAPTCHA_ENABLED") == "true"
	AppConfig.EZCaptchaClientKey = os.Getenv("EZCAPTCHA_CLIENT_KEY")
	AppConfig.EZAppID = os.Getenv("EZAPPID")
	AppConfig.SoftID = os.Getenv("SOFT_ID")
	AppConfig.SiteAction = os.Getenv("SITE_ACTION")
	AppConfig.RecaptchaSiteKey = os.Getenv("RECAPTCHA_SITE_KEY")
	AppConfig.RecaptchaURL = os.Getenv("RECAPTCHA_URL")
	AppConfig.MaxRetries = getEnvAsInt("MAX_RETRIES", 3)

	// Balance Minimums
	AppConfig.EZCapBalMin = getEnvAsFloat("EZCAPBALMIN", 100)
	AppConfig.TwoCapBalMin = getEnvAsFloat("TWOCAPBALMIN", 0.25)

	// API Endpoints
	AppConfig.CheckEndpoint = os.Getenv("CHECK_ENDPOINT")
	AppConfig.ProfileEndpoint = os.Getenv("PROFILE_ENDPOINT")
	AppConfig.CheckVIPEndpoint = os.Getenv("CHECK_VIP_ENDPOINT")
	AppConfig.RedeemCodeEndpoint = os.Getenv("REDEEM_CODE_ENDPOINT")

	// Admin Panel Settings
	AppConfig.SessionKey = os.Getenv("SESSION_KEY")
	AppConfig.StaticDir = getEnvWithDefault("STATIC_DIR", "./static")
	AppConfig.TemplatesDir = getEnvWithDefault("TEMPLATES_DIR", "templates")
	AppConfig.AdminPort = os.Getenv("ADMIN_PORT")
	AppConfig.AdminUsername = os.Getenv("ADMIN_USERNAME")
	AppConfig.AdminPassword = os.Getenv("ADMIN_PASSWORD")
	AppConfig.StatsRateLimit = getEnvAsFloat("STATS_RATE_LIMIT", 25.0)

	// Rate Limits and Intervals
	AppConfig.CheckNowRateLimit = time.Duration(getEnvAsInt("CHECK_NOW_RATE_LIMIT", 3600)) * time.Second
	AppConfig.DefaultRateLimit = time.Duration(getEnvAsInt("DEFAULT_RATE_LIMIT", 180)) * time.Minute
	AppConfig.DefaultUserMaxAccounts = getEnvAsInt("DEFAULT_USER_MAXACCOUNTS", 3)
	AppConfig.PremUserMaxAccounts = getEnvAsInt("PREM_USER_MAXACCOUNTS", 10)
	AppConfig.CheckInterval = getEnvAsInt("CHECK_INTERVAL", 15)
	AppConfig.NotificationInterval = getEnvAsFloat("NOTIFICATION_INTERVAL", 24)
	AppConfig.CooldownDuration = getEnvAsFloat("COOLDOWN_DURATION", 6)
	AppConfig.SleepDuration = getEnvAsInt("SLEEP_DURATION", 1)
	AppConfig.CookieCheckIntervalPermaban = getEnvAsFloat("COOKIE_CHECK_INTERVAL_PERMABAN", 24)
	AppConfig.StatusChangeCooldown = getEnvAsFloat("STATUS_CHANGE_COOLDOWN", 1)
	AppConfig.GlobalNotificationCooldown = getEnvAsFloat("GLOBAL_NOTIFICATION_COOLDOWN", 2)
	AppConfig.CookieExpirationWarning = getEnvAsFloat("COOKIE_EXPIRATION_WARNING", 24)
	AppConfig.TempBanUpdateInterval = getEnvAsFloat("TEMP_BAN_UPDATE_INTERVAL", 24)

	// Emoji Settings
	AppConfig.CheckCircle = os.Getenv("CHECKCIRCLE")
	AppConfig.BanCircle = os.Getenv("BANCIRCLE")
	AppConfig.InfoCircle = os.Getenv("INFOCIRCLE")
	AppConfig.StopWatch = os.Getenv("STOPWATCH")
	AppConfig.QuestionCircle = os.Getenv("QUESTIONCIRCLE")

	// Donation Settings
	AppConfig.DonationsEnabled = os.Getenv("DONATIONS_ENABLED") == "true"
	AppConfig.BitcoinAddress = os.Getenv("BITCOIN_ADDRESS")
	AppConfig.CashAppID = os.Getenv("CASHAPP_ID")

	if err := validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	logger.Log.Info("Configuration loaded successfully")
	return nil
}

func validate() error {
	required := map[string]string{
		"DISCORD_TOKEN":      AppConfig.DiscordToken,
		"DEVELOPER_ID":       AppConfig.DeveloperID,
		"DB_USER":            AppConfig.DBUser,
		"DB_PASSWORD":        AppConfig.DBPassword,
		"DB_NAME":            AppConfig.DBName,
		"DB_HOST":            AppConfig.DBHost,
		"DB_PORT":            AppConfig.DBPort,
		"CHECK_ENDPOINT":     AppConfig.CheckEndpoint,
		"PROFILE_ENDPOINT":   AppConfig.ProfileEndpoint,
		"CHECK_VIP_ENDPOINT": AppConfig.CheckVIPEndpoint,
	}

	var missing []string
	for key, value := range required {
		if value == "" {
			missing = append(missing, key)
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
	}
	return defaultValue
}

func getEnvAsFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

// Get returns the global configuration instance
func Get() *Config {
	return &AppConfig
}
