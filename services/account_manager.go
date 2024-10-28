package services

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/discordgo"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
)

func processUserAccountBatch(s *discordgo.Session, userID string, accounts []models.Account) {
	userSettings, err := GetUserSettings(userID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", userID)
		return
	}

	if err := validateUserCaptchaService(userID, userSettings); err != nil {
		logger.Log.WithError(err).Errorf("Captcha service validation failed for user %s", userID)
		notifyUserOfServiceIssue(s, userID, err)
		return
	}

	var (
		accountsToUpdate    []models.Account
		dailyUpdateAccounts []models.Account
		expiringAccounts    []models.Account
		now                 = time.Now()
	)

	for _, account := range accounts {
		if shouldCheckAccount(account, userSettings, now) {
			accountsToUpdate = append(accountsToUpdate, account)
		}

		if shouldIncludeInDailyUpdate(account, userSettings, now) {
			dailyUpdateAccounts = append(dailyUpdateAccounts, account)
		}

		if shouldCheckExpiration(account, now) {
			expiringAccounts = append(expiringAccounts, account)
		}
	}

	for _, account := range accountsToUpdate {
		checkSemaphore <- struct{}{}
		go func(acc models.Account) {
			defer func() { <-checkSemaphore }()
			processAccountCheck(s, acc, userSettings)
		}(account)
	}

	if len(dailyUpdateAccounts) > 0 {
		SendConsolidatedDailyUpdate(s, userID, userSettings, dailyUpdateAccounts)
	}

	if len(expiringAccounts) > 0 {
		NotifyCookieExpiringSoon(s, expiringAccounts)
	}
}

func processAccountCheck(s *discordgo.Session, account models.Account, userSettings models.UserSettings) {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		if err := checkAccountWithContext(ctx, s, account, userSettings); err != nil {
			if attempt == maxRetryAttempts {
				handleCheckFailure(s, account, err)
				return
			}
			time.Sleep(retryDelay)
			continue
		}
		break
	}
}

func checkAccountWithContext(ctx context.Context, s *discordgo.Session, account models.Account, userSettings models.UserSettings) error {
	captchaAPIKey, err := getCaptchaKeyForUser(userSettings)
	if err != nil {
		return fmt.Errorf("failed to get captcha key: %w", err)
	}

	status, err := CheckAccountWithContext(ctx, account.SSOCookie, account.UserID, captchaAPIKey)
	if err != nil {
		return fmt.Errorf("failed to check account status: %w", err)
	}

	DBMutex.Lock()
	defer DBMutex.Unlock()

	previousStatus := account.LastStatus
	account.LastStatus = status
	account.LastCheck = time.Now().Unix()
	account.LastSuccessfulCheck = time.Now()
	account.ConsecutiveErrors = 0

	if err := database.DB.Save(&account).Error; err != nil {
		return fmt.Errorf("failed to update account state: %w", err)
	}

	if previousStatus != status {
		HandleStatusChange(s, account, status, userSettings)
	}

	return nil
}

func handleCheckFailure(s *discordgo.Session, account models.Account, err error) {
	DBMutex.Lock()
	defer DBMutex.Unlock()

	account.ConsecutiveErrors++
	account.LastErrorTime = time.Now()

	if shouldDisableAccount(account, err) {
		disableAccount(s, account, getDisableReason(err))
	} else {
		if err := database.DB.Save(&account).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to update account error state")
		}
		notifyUserOfError(s, account, err)
	}
}

func shouldCheckAccount(account models.Account, userSettings models.UserSettings, now time.Time) bool {
	if account.IsCheckDisabled {
		return false
	}

	if account.IsPermabanned {
		return time.Unix(account.LastCookieCheck, 0).Add(time.Duration(cookieCheckIntervalPermaban) * time.Hour).Before(now)
	}

	checkInterval := time.Duration(userSettings.CheckInterval) * time.Minute
	return time.Unix(account.LastCheck, 0).Add(checkInterval).Before(now)
}

func shouldIncludeInDailyUpdate(account models.Account, userSettings models.UserSettings, now time.Time) bool {
	return time.Unix(account.LastNotification, 0).Add(time.Duration(userSettings.NotificationInterval) * time.Hour).Before(now)
}

func shouldCheckExpiration(account models.Account, now time.Time) bool {
	if account.IsExpiredCookie {
		return false
	}

	timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
	if err != nil {
		return false
	}

	return timeUntilExpiration > 0 && timeUntilExpiration <= time.Duration(cookieExpirationWarning)*time.Hour
}

func shouldDisableAccount(account models.Account, err error) bool {
	if account.ConsecutiveErrors >= maxConsecutiveErrors {
		return true
	}

	return strings.Contains(err.Error(), "Missing Access") ||
		strings.Contains(err.Error(), "Unknown Channel") ||
		strings.Contains(err.Error(), "insufficient balance") ||
		strings.Contains(err.Error(), "invalid captcha API key")
}

func getDisableReason(err error) string {
	switch {
	case strings.Contains(err.Error(), "Missing Access"):
		return "Bot removed from server/channel"
	case strings.Contains(err.Error(), "insufficient balance"):
		return "Insufficient captcha balance"
	case strings.Contains(err.Error(), "invalid captcha API key"):
		return "Invalid captcha API key"
	default:
		return fmt.Sprintf("Too many consecutive errors: %v", err)
	}
}

func validateUserCaptchaService(userID string, userSettings models.UserSettings) error {
	if !services.IsServiceEnabled(userSettings.PreferredCaptchaProvider) {
		return fmt.Errorf("captcha service %s is disabled", userSettings.PreferredCaptchaProvider)
	}

	if userSettings.EZCaptchaAPIKey != "" || userSettings.TwoCaptchaAPIKey != "" {
		_, balance, err := services.GetUserCaptchaKey(userID)
		if err != nil {
			return fmt.Errorf("failed to validate captcha key: %w", err)
		}
		if balance <= 0 {
			return fmt.Errorf("insufficient captcha balance: %.2f", balance)
		}
	}

	return nil
}

func getCaptchaKeyForUser(userSettings models.UserSettings) (string, error) {
	switch userSettings.PreferredCaptchaProvider {
	case "ezcaptcha":
		if userSettings.EZCaptchaAPIKey != "" {
			return userSettings.EZCaptchaAPIKey, nil
		}
		return os.Getenv("EZCAPTCHA_CLIENT_KEY"), nil
	case "2captcha":
		if userSettings.TwoCaptchaAPIKey != "" {
			return userSettings.TwoCaptchaAPIKey, nil
		}
		return "", fmt.Errorf("no 2captcha API key available")
	default:
		return "", fmt.Errorf("unsupported captcha provider: %s", userSettings.PreferredCaptchaProvider)
	}
}
