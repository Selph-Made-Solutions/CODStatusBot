package services

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/joho/godotenv"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	checkInterval               float64 // Check interval for accounts (in minutes)
	notificationInterval        float64 // Notification interval for daily updates (in hours)
	cooldownDuration            float64 // Cooldown duration for invalid cookie notifications (in hours)
	sleepDuration               int     // Sleep duration for the account checking loop (in minutes)
	cookieCheckIntervalPermaban float64 // Check interval for permabanned accounts (in hours)
	statusChangeCooldown        float64 // Cooldown duration for status change notifications (in hours)
	globalNotificationCooldown  float64 // Global cooldown for notifications per user (in hours)
	userNotificationTimestamps  = make(map[string]time.Time)
	userNotificationMutex       sync.Mutex
	DBMutex                     sync.Mutex
)

func init() {
	err := godotenv.Load()
	if err != nil {
		logger.Log.WithError(err).Error("Failed to load .env file")
	}

	checkInterval, _ = strconv.ParseFloat(os.Getenv("CHECK_INTERVAL"), 64)
	notificationInterval, _ = strconv.ParseFloat(os.Getenv("NOTIFICATION_INTERVAL"), 64)
	cooldownDuration, _ = strconv.ParseFloat(os.Getenv("COOLDOWN_DURATION"), 64)
	sleepDuration, _ = strconv.Atoi(os.Getenv("SLEEP_DURATION"))
	cookieCheckIntervalPermaban, _ = strconv.ParseFloat(os.Getenv("COOKIE_CHECK_INTERVAL_PERMABAN"), 64)
	statusChangeCooldown, _ = strconv.ParseFloat(os.Getenv("STATUS_CHANGE_COOLDOWN"), 64)
	globalNotificationCooldown, _ = strconv.ParseFloat(os.Getenv("GLOBAL_NOTIFICATION_COOLDOWN"), 64)

	logger.Log.Infof("Loaded config: CHECK_INTERVAL=%.2f minutes, NOTIFICATION_INTERVAL=%.2f hours, COOLDOWN_DURATION=%.2f hours, SLEEP_DURATION=%d minutes, COOKIE_CHECK_INTERVAL_PERMABAN=%.2f hours, STATUS_CHANGE_COOLDOWN=%.2f hours, GLOBAL_NOTIFICATION_COOLDOWN=%.2f hours",
		checkInterval, notificationInterval, cooldownDuration, sleepDuration, cookieCheckIntervalPermaban, statusChangeCooldown, globalNotificationCooldown)
}

func canSendNotification(userID string) bool {
	userNotificationMutex.Lock()
	defer userNotificationMutex.Unlock()

	lastNotification, exists := userNotificationTimestamps[userID]
	if !exists || time.Since(lastNotification).Hours() >= globalNotificationCooldown {
		userNotificationTimestamps[userID] = time.Now()
		return true
	}
	return false
}

// sendNotification function: sends notifications based on user preference
func sendNotification(discord *discordgo.Session, account models.Account, embed *discordgo.MessageEmbed, content string) error {
	if !canSendNotification(account.UserID) {
		logger.Log.Infof("Skipping notification for user %s (global cooldown)", account.UserID)
		return nil
	}

	var channelID string
	if account.NotificationType == "dm" {
		channel, err := discord.UserChannelCreate(account.UserID)
		if err != nil {
			return fmt.Errorf("failed to create DM channel: %v", err)
		}
		channelID = channel.ID
	} else {
		channelID = account.ChannelID
	}

	_, err := discord.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed:   embed,
		Content: content,
	})
	return err
}

// sendDailyUpdate function: sends a daily update message for a given account
func sendDailyUpdate(account models.Account, discord *discordgo.Session) {
	logger.Log.Infof("Sending daily update for account %s", account.Title)

	// Prepare the description based on the account's cookie status
	var description string
	if account.IsExpiredCookie {
		description = fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title)
	} else {
		description = fmt.Sprintf("The last status of account %s was %s.", account.Title, account.LastStatus)
	}

	// Create the embed message
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%.2f Hour Update - %s", notificationInterval, account.Title),
		Description: description,
		Color:       GetColorForStatus(account.LastStatus, account.IsExpiredCookie),
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	// Send the notification
	err := sendNotification(discord, account, embed, "")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send scheduled update message for account %s", account.Title)
	}

	// Update the account's last check and notification timestamps
	account.LastCheck = time.Now().Unix()
	account.LastNotification = time.Now().Unix()
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
	}
}

// CheckAccounts function: periodically checks all accounts for status changes
func CheckAccounts(s *discordgo.Session) {
	for {
		logger.Log.Info("Starting periodic account check")
		var accounts []models.Account
		if err := database.DB.Find(&accounts).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to fetch accounts from the database")
			continue
		}

		// Iterate through each account and perform checks
		for _, account := range accounts {
			var lastCheck time.Time
			if account.LastCheck != 0 {
				lastCheck = time.Unix(account.LastCheck, 0)
			}
			var lastNotification time.Time
			if account.LastNotification != 0 {
				lastNotification = time.Unix(account.LastNotification, 0)
			}

			// Handle permabanned accounts
			if account.IsPermabanned {
				lastCookieCheck := time.Unix(account.LastCookieCheck, 0)
				if time.Since(lastCookieCheck).Hours() > cookieCheckIntervalPermaban {
					isValid := VerifySSOCookie(account.SSOCookie)
					if !isValid {
						account.IsExpiredCookie = true
						if err := database.DB.Save(&account).Error; err != nil {
							logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
						}
						go sendDailyUpdate(account, s)
					}
					account.LastCookieCheck = time.Now().Unix()
					if err := database.DB.Save(&account).Error; err != nil {
						logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
					}
				}
				logger.Log.WithField("account", account.Title).Info("Skipping permanently banned account")
				continue
			}

			// Handle accounts with expired cookies
			if account.IsExpiredCookie {
				logger.Log.WithField("account", account.Title).Info("Skipping account with expired cookie")
				if time.Since(lastNotification).Hours() > notificationInterval {
					go sendDailyUpdate(account, s)
				} else {
					logger.Log.WithField("account", account.Title).Infof("Owner of %s recently notified within %.2f hours already, skipping", account.Title, notificationInterval)
				}
				continue
			}

			// Check account status if enough time has passed since the last check
			if time.Since(lastCheck).Minutes() > checkInterval {
				go CheckSingleAccount(account, s)
			} else {
				logger.Log.WithField("account", account.Title).Infof("Account %s checked recently less than %.2f minutes ago, skipping", account.Title, checkInterval)
			}

			// Send daily update if enough time has passed since the last notification
			if time.Since(lastNotification).Hours() > notificationInterval {
				go sendDailyUpdate(account, s)
			} else {
				logger.Log.WithField("account", account.Title).Infof("Owner of %s recently notified within %.2f hours already, skipping", account.Title, notificationInterval)
			}
		}
		time.Sleep(time.Duration(sleepDuration) * time.Minute)
	}
}

// CheckSingleAccount function: checks the status of a single account
func CheckSingleAccount(account models.Account, discord *discordgo.Session) {
	// Skip checking if the account is already permanently banned
	if account.IsPermabanned {
		logger.Log.WithField("account", account.Title).Info("Skipping permanently banned account")
		return
	}

	// Check the account status
	result, err := CheckAccount(account.SSOCookie, account.CaptchaService, account.CaptchaAPIKey)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check account %s: possible expired SSO Cookie", account.Title)
		return
	}

	// Handle invalid cookie status
	if result == models.StatusInvalidCookie {
		lastNotification := time.Unix(account.LastCookieNotification, 0)
		if time.Since(lastNotification).Hours() >= cooldownDuration || account.LastCookieNotification == 0 {
			logger.Log.Infof("Account %s has an invalid SSO cookie", account.Title)
			embed := &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("%s - Invalid SSO Cookie", account.Title),
				Description: fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title),
				Color:       0xff9900,
				Timestamp:   time.Now().Format(time.RFC3339),
			}

			err := sendNotification(discord, account, embed, "")
			if err != nil {
				logger.Log.WithError(err).Errorf("Failed to send invalid cookie notification for account %s", account.Title)
			}

			// Update account information regarding the expired cookie
			DBMutex.Lock()
			account.LastCookieNotification = time.Now().Unix()
			account.IsExpiredCookie = true
			if err := database.DB.Save(&account).Error; err != nil {
				logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
			}
			DBMutex.Unlock()
		} else {
			logger.Log.Infof("Skipping expired cookie notification for account %s (cooldown)", account.Title)
		}
		return
	}

	// Update account information
	DBMutex.Lock()
	lastStatus := account.LastStatus
	account.LastCheck = time.Now().Unix()
	account.IsExpiredCookie = false
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
		DBMutex.Unlock()
		return
	}
	DBMutex.Unlock()

	// Handle status changes and send notifications
	if result != lastStatus {
		lastStatusChange := time.Unix(account.LastStatusChange, 0)
		if time.Since(lastStatusChange).Hours() < statusChangeCooldown {
			logger.Log.Infof("Skipping status change notification for account %s (cooldown)", account.Title)
			return
		}

		DBMutex.Lock()
		account.LastStatus = result
		account.LastStatusChange = time.Now().Unix()
		if result == models.StatusPermaban {
			account.IsPermabanned = true
		} else {
			account.IsPermabanned = false
		}
		if err := database.DB.Save(&account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
			DBMutex.Unlock()
			return
		}
		logger.Log.Infof("Account %s status changed to %s", account.Title, result)

		// Create a new record for the account
		ban := models.Ban{
			Account:   account,
			Status:    result,
			AccountID: account.ID,
		}
		if err := database.DB.Create(&ban).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to create new ban record for account %s", account.Title)
		}
		// Create an embed message for the status change notification
		DBMutex.Unlock()
		logger.Log.Infof("Account %s status changed to %s", account.Title, result)

		embed := &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - %s", account.Title, EmbedTitleFromStatus(result)),
			Description: fmt.Sprintf("The status of account %s has changed to %s", account.Title, result),
			Color:       GetColorForStatus(result, account.IsExpiredCookie),
			Timestamp:   time.Now().Format(time.RFC3339),
		}

		err := sendNotification(discord, account, embed, fmt.Sprintf("<@%s>", account.UserID))
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send status update message for account %s", account.Title)
		}
	}
}

// GetColorForStatus function: returns the appropriate color for an embed message based on the account status
func GetColorForStatus(status models.Status, isExpiredCookie bool) int {
	if isExpiredCookie {
		return 0xff9900 // Orange for expired cookie
	}
	switch status {
	case models.StatusPermaban:
		return 0xff0000 // Red for permanent ban
	case models.StatusShadowban:
		return 0xffff00 // Yellow for shadowban
	default:
		return 0x00ff00 // Green for no ban
	}
}

// EmbedTitleFromStatus function: returns the appropriate title for an embed message based on the account status
func EmbedTitleFromStatus(status models.Status) string {
	switch status {
	case models.StatusPermaban:
		return "PERMANENT BAN DETECTED"
	case models.StatusShadowban:
		return "SHADOWBAN DETECTED"
	default:
		return "ACCOUNT NOT BANNED"
	}
}
