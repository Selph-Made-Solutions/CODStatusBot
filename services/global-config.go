package services

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/bradselph/CODStatusBot/logger"
)

type GlobalConfig struct {
	LogDir string

	EZCaptchaClientKey          string
	EZAppID                     string
	SoftID                      string
	SiteAction                  string
	EZCapBalMin                 float64
	TwoCapBalMin                float64
	RecaptchaSiteKey            string
	RecaptchaURL                string
	CheckNowRateLimit           time.Duration
	DefaultRateLimit            time.Duration
	DefaultUserMaxAccounts      int
	PremUserMaxAccounts         int
	CheckEndpoint               string
	ProfileEndpoint             string
	CheckVIPEndpoint            string
	RedeemCodeEndpoint          string
	DefaultCheckInterval        int
	DefaultNotificationInterval float64
	DefaultCooldownDuration     float64
	DefaultStatusChangeCooldown float64
	CheckCircle                 string
	BanCircle                   string
	InfoCircle                  string
	StopWatch                   string
	QuestionCircle              string
}

var Config GlobalConfig

func InitializeConfig() error {
	logger.Log.Info("Initializing global configuration...")

	Config.EZCaptchaClientKey = os.Getenv("EZCAPTCHA_CLIENT_KEY")
	Config.EZAppID = os.Getenv("EZAPPID")
	Config.SoftID = os.Getenv("SOFT_ID")
	Config.SiteAction = os.Getenv("SITE_ACTION")
	Config.RecaptchaSiteKey = os.Getenv("RECAPTCHA_SITE_KEY")
	Config.RecaptchaURL = os.Getenv("RECAPTCHA_URL")
	Config.CheckEndpoint = os.Getenv("CHECK_ENDPOINT")
	Config.ProfileEndpoint = os.Getenv("PROFILE_ENDPOINT")
	Config.CheckVIPEndpoint = os.Getenv("CHECK_VIP_ENDPOINT")
	Config.RedeemCodeEndpoint = os.Getenv("REDEEM_CODE_ENDPOINT")

	if ezCapBalMin, err := strconv.ParseFloat(os.Getenv("EZCAPBALMIN"), 64); err == nil {
		Config.EZCapBalMin = ezCapBalMin
	} else {
		Config.EZCapBalMin = 100
	}

	if twoCapBalMin, err := strconv.ParseFloat(os.Getenv("TWOCAPBALMIN"), 64); err == nil {
		Config.TwoCapBalMin = twoCapBalMin
	} else {
		Config.TwoCapBalMin = 0.25
	}

	if checkNowLimit, err := strconv.Atoi(os.Getenv("CHECK_NOW_RATE_LIMIT")); err == nil {
		Config.CheckNowRateLimit = time.Duration(checkNowLimit) * time.Second
	} else {
		Config.CheckNowRateLimit = 3600 * time.Second // default 1 hour
	}

	if defaultLimit, err := strconv.Atoi(os.Getenv("DEFAULT_RATE_LIMIT")); err == nil {
		Config.DefaultRateLimit = time.Duration(defaultLimit) * time.Minute
	} else {
		Config.DefaultRateLimit = 15 * time.Minute // default 15 minutes
	}

	if interval, err := strconv.Atoi(os.Getenv("CHECK_INTERVAL")); err == nil {
		Config.DefaultCheckInterval = interval
	} else {
		Config.DefaultCheckInterval = 15 // default 15 minutes
	}

	if notifInterval, err := strconv.ParseFloat(os.Getenv("NOTIFICATION_INTERVAL"), 64); err == nil {
		Config.DefaultNotificationInterval = notifInterval
	} else {
		Config.DefaultNotificationInterval = 24 // default 24 hours
	}

	if cooldown, err := strconv.ParseFloat(os.Getenv("COOLDOWN_DURATION"), 64); err == nil {
		Config.DefaultCooldownDuration = cooldown
	} else {
		Config.DefaultCooldownDuration = 6 // default 6 hours
	}

	if statusCooldown, err := strconv.ParseFloat(os.Getenv("STATUS_CHANGE_COOLDOWN"), 64); err == nil {
		Config.DefaultStatusChangeCooldown = statusCooldown
	} else {
		Config.DefaultStatusChangeCooldown = 1 // default 1 hour
	}

	if defaultMax, err := strconv.Atoi(os.Getenv("DEFAULT_USER_MAXACCOUNTS")); err == nil {
		Config.DefaultUserMaxAccounts = defaultMax
	} else {
		Config.DefaultUserMaxAccounts = 3 // default 3 accounts
	}

	if premMax, err := strconv.Atoi(os.Getenv("PREM_USER_MAXACCOUNTS")); err == nil {
		Config.PremUserMaxAccounts = premMax
	} else {
		Config.PremUserMaxAccounts = 10 // default 10 accounts
	}

	Config.CheckCircle = os.Getenv("CHECKCIRCLE")
	Config.BanCircle = os.Getenv("BANCIRCLE")
	Config.InfoCircle = os.Getenv("INFOCIRCLE")
	Config.StopWatch = os.Getenv("STOPWATCH")
	Config.QuestionCircle = os.Getenv("QUESTIONCIRCLE")

	if err := validateConfig(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	logger.Log.Info("Global configuration initialized successfully")
	return nil
}

func validateConfig() error {
	if Config.EZCaptchaClientKey == "" {
		return fmt.Errorf("EZCAPTCHA_CLIENT_KEY is not set")
	}
	if Config.EZAppID == "" {
		return fmt.Errorf("EZAPPID is not set")
	}
	if Config.SiteAction == "" {
		return fmt.Errorf("SITE_ACTION is not set")
	}
	if Config.RecaptchaSiteKey == "" {
		return fmt.Errorf("RECAPTCHA_SITE_KEY is not set")
	}
	if Config.RecaptchaURL == "" {
		return fmt.Errorf("RECAPTCHA_URL is not set")
	}

	if Config.CheckEndpoint == "" {
		return fmt.Errorf("CHECK_ENDPOINT is not set")
	}
	if Config.ProfileEndpoint == "" {
		return fmt.Errorf("PROFILE_ENDPOINT is not set")
	}
	if Config.CheckVIPEndpoint == "" {
		return fmt.Errorf("CHECK_VIP_ENDPOINT is not set")
	}

	return nil
}
