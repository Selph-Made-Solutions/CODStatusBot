package services

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"codstatusbot2.0/database"
	"codstatusbot2.0/logger"
	"codstatusbot2.0/models"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

var (
	checkInterval        float64 // Check interval for accounts (in minutes)
	notificationInterval float64 // Notification interval for daily updates (in hours)
	cooldownDuration     float64 // Cooldown duration for invalid cookie notifications (in hours)
	sleepDuration        int     // Sleep duration for the account checking loop (in minutes)
)

// init loads environment variables and sets up configuration.
func init() {
	err := godotenv.Load()
	if err != nil {
		logger.Log.WithError(err).Error("Failed to load .env file")
	}

	checkInterval, _ = strconv.ParseFloat(os.Getenv("CHECK_INTERVAL"), 64)
	notificationInterval, _ = strconv.ParseFloat(os.Getenv("NOTIFICATION_INTERVAL"), 64)
	cooldownDuration, _ = strconv.ParseFloat(os.Getenv("COOLDOWN_DURATION"), 64)
	sleepDuration, _ = strconv.Atoi(os.Getenv("SLEEP_DURATION"))

	logger.Log.Infof("Loaded config: CHECK_INTERVAL=%.2f, NOTIFICATION_INTERVAL=%.2f, COOLDOWN_DURATION=%.2f, SLEEP_DURATION=%d",
		checkInterval, notificationInterval, cooldownDuration, sleepDuration)
}

// sendDailyUpdate sends a daily update message to the user for a given account.
func sendDailyUpdate(account models.Account, discord *discordgo.Session) {
	logger.Log.Infof("Sending daily update for account %s", account.Title)

	// Prepare the description for the embed message.
	var description string
	if account.IsExpiredCookie {
		description = fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title)
	} else {
		description = fmt.Sprintf("The last status of account %s was %s.", account.Title, account.LastStatus)
	}

	// Create the embed message.
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%.2f Hour Update - %s", notificationInterval, account.Title),
		Description: description,
		Color:       GetColorForStatus(account.LastStatus, account.IsExpiredCookie),
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	// Determine the channel to send the message to (DM or specified channel).
	var channelID string
	if account.NotificationType == "dm" {
		channel, err := discord.UserChannelCreate(account.UserID)
		if err != nil {
			logger.Log.WithError(err).Error("Failed to create DM channel")
			return
		}
		channelID = channel.ID
	} else {
		channelID = account.ChannelID
	}

	// Send the embed message.
	_, err := discord.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed: embed,
	})
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send scheduled update message for account %s: %v ", account.Title, err)
	}

	// Update the account's last check and notification timestamps.
	account.LastCheck = time.Now().Unix()
	account.LastNotification = time.Now().Unix()
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s: %v ", account.Title, err)
	}
}

// CheckAccounts periodically checks all accounts for status changes.
func CheckAccounts(s *discordgo.Session) {
	for {
		logger.Log.Info("Starting periodic account check")
		var accounts []models.Account
		if err := database.DB.Find(&accounts).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to fetch accounts from the database")
			continue
		}

		// Iterate through each account and perform checks.
		for _, account := range accounts {
			var lastCheck time.Time
			if account.LastCheck != 0 {
				lastCheck = time.Unix(account.LastCheck, 0)
			}
			var lastNotification time.Time
			if account.LastNotification != 0 {
				lastNotification = time.Unix(account.LastNotification, 0)
			}

			// Handle accounts with expired cookies.
			if account.IsExpiredCookie {
				logger.Log.WithField("account ", account.Title).Info("Skipping account with expired cookie")
				if time.Since(lastNotification).Hours() > notificationInterval {
					go sendDailyUpdate(account, s)
				} else {
					logger.Log.WithField("account ", account.Title).Infof("Owner of %s recently notified within %.2f hours already, skipping", account.Title, notificationInterval)
				}
				continue
			}

			// Check account status if enough time has passed since the last check.
			if time.Since(lastCheck).Minutes() > checkInterval {
				go CheckSingleAccount(account, s)
			} else {
				logger.Log.WithField("account ", account.Title).Infof("Account %s checked recently less than %.2f minutes ago, skipping", account.Title, checkInterval)
			}
			// Send daily update if enough time has passed since the last notification.
			if time.Since(lastNotification).Hours() > notificationInterval {
				go sendDailyUpdate(account, s)
			} else {
				logger.Log.WithField("account ", account.Title).Infof("Owner of %s recently notified within %.2f hours already, skipping", account.Title, notificationInterval)
			}
		}
		time.Sleep(time.Duration(sleepDuration) * time.Minute)
	}
}

// CheckSingleAccount checks the status of a single account and sends notifications if necessary.
func CheckSingleAccount(account models.Account, discord *discordgo.Session) {
	result, err := CheckAccount(account.SSOCookie)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check account %s: possible expired SSO Cookie", account.Title)
		return
	}

	// Handle invalid cookie scenarios.
	if result == models.StatusInvalidCookie {
		lastNotification := time.Unix(account.LastCookieNotification, 0)
		if time.Since(lastNotification) >= time.Duration(cooldownDuration)*time.Hour || account.LastCookieNotification == 0 {
			logger.Log.Infof("Account %s has an invalid SSO cookie", account.Title)
			// Create an embed message for invalid cookie notification.
			embed := &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("%s - Invalid SSO Cookie", account.Title),
				Description: fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title),
				Color:       0xff9900,
				Timestamp:   time.Now().Format(time.RFC3339),
			}
			// Determine the channel to send the notification to (DM or specified channel).
			var channelID string
			if account.NotificationType == "dm" {
				channel, err := discord.UserChannelCreate(account.UserID)
				if err != nil {
					logger.Log.WithError(err).Error("Failed to create DM channel")
					return
				}
				channelID = channel.ID
			} else {
				channelID = account.ChannelID
			}
			// Send the invalid cookie notification.
			_, err = discord.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Embed: embed,
			})
			if err != nil {
				logger.Log.WithError(err).Errorf("Failed to send invalid cookie notification for account %s: %v ", account.Title, err)
			}

			// Update account information regarding the expired cookie.
			account.LastCookieNotification = time.Now().Unix()
			account.IsExpiredCookie = true
			if err := database.DB.Save(&account).Error; err != nil {
				logger.Log.WithError(err).Errorf("Failed to save account changes for account %s: %v ", account.Title, err)
			}
		} else {
			logger.Log.Infof("Skipping expired cookie notification for account %s (cooldown)", account.Title)
		}
		return
	}

	lastStatus := account.LastStatus
	account.LastCheck = time.Now().Unix()
	account.IsExpiredCookie = false
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s: %v ", account.Title, err)
		return
	}
	// Handle status changes and send notifications.
	if result != lastStatus {
		account.LastStatus = result
		if result == models.StatusPermaban {
			account.IsPermabanned = true
		} else {
			account.IsPermabanned = false
		}
		if err := database.DB.Save(&account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to save account changes for account %s: %v ", account.Title, err)
			return
		}
		logger.Log.Infof("Account %s status changed to %s ", account.Title, result)
		// Create a new ban record for the account.
		ban := models.Ban{
			Account:   account,
			Status:    result,
			AccountID: account.ID,
		}
		if err := database.DB.Create(&ban).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to create new ban record for account %s: %v ", account.Title, err)
		}
		// Create an embed message for the status change notification.
		embed := &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - %s", account.Title, EmbedTitleFromStatus(result)),
			Description: fmt.Sprintf("The status of account %s has changed to %s <@%s> ", account.Title, result, account.UserID),
			Color:       GetColorForStatus(result, account.IsExpiredCookie),
			Timestamp:   time.Now().Format(time.RFC3339),
		}

		// Determine the channel to send the notification to (DM or specified channel).
		var channelID string
		if account.NotificationType == "dm" {
			channel, err := discord.UserChannelCreate(account.UserID)
			if err != nil {
				logger.Log.WithError(err).Error("Failed to create DM channel")
				return
			}
			channelID = channel.ID
		} else {
			channelID = account.ChannelID
		}

		_, err = discord.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Embed:   embed,
			Content: fmt.Sprintf("<@%s>", account.UserID),
		})
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send status update message for account %s: %v ", account.Title, err)
		}
	}
}

// GetColorForStatus returns the appropriate color for an embed message based on the account status.
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

// EmbedTitleFromStatus returns the appropriate title for an embed message based on the account status.
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
