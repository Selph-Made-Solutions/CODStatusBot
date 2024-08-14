package services

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"os"
	"strconv"
	"sync"
	"time"
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
		timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
		if err != nil {
			logger.Log.WithError(err).Errorf("Error checking SSO cookie expiration for account %s", account.Title)
			description = fmt.Sprintf("An error occurred while checking the SSO cookie expiration for account %s. Please check the account status manually.", account.Title)
		} else if timeUntilExpiration > 0 {
			description = fmt.Sprintf("The last status of account %s was %s. SSO cookie will expire in %s.", account.Title, account.LastStatus, FormatExpirationTime(account.SSOCookieExpiration))
		} else {
			description = fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title)
		}
	}

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
			time.Sleep(time.Duration(sleepDuration) * time.Minute)
			continue
		}

		// Iterate through each account and perform checks
		for _, account := range accounts {
			userSettings, err := GetUserSettings(account.UserID)
			if err != nil {
				logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", account.UserID)
				continue
			}

			lastCheck := time.Unix(account.LastCheck, 0)
			if time.Since(lastCheck).Minutes() < float64(userSettings.CheckInterval) {
				logger.Log.WithField("account", account.Title).Infof("Account %s checked recently, skipping", account.Title)
				continue
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
				if time.Since(lastNotification).Hours() > userSettings.NotificationInterval {
					go sendDailyUpdate(account, s)
				} else {
					logger.Log.WithField("account", account.Title).Infof("Owner of %s recently notified within %.2f hours already, skipping", account.Title, userSettings.NotificationInterval)
				}
				continue
			}

			// Check account status if enough time has passed since the last check
			if time.Since(lastCheck).Minutes() > float64(userSettings.CheckInterval) {
				go CheckSingleAccount(account, s)
			} else {
				logger.Log.WithField("account", account.Title).Infof("Account %s checked recently less than %.2f minutes ago, skipping", account.Title, float64(userSettings.CheckInterval))
			}

			// Send daily update if enough time has passed since the last notification
			if time.Since(lastNotification).Hours() > userSettings.NotificationInterval {
				go sendDailyUpdate(account, s)
			} else {
				logger.Log.WithField("account", account.Title).Infof("Owner of %s recently notified within %.2f hours already, skipping", account.Title, userSettings.NotificationInterval)
			}
		}
		time.Sleep(time.Duration(sleepDuration) * time.Minute)
	}
}

// CheckSingleAccount function: checks the status of a single account
func CheckSingleAccount(account models.Account, discord *discordgo.Session) {
	// Check SSO cookie expiration
	timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check SSO cookie expiration for account %s", account.Title)
	} else if timeUntilExpiration > 0 && timeUntilExpiration <= 24*time.Hour {
		// Notify user if the cookie will expire within 24 hours
		embed := &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - SSO Cookie Expiring Soon", account.Title),
			Description: fmt.Sprintf("The SSO cookie for account %s will expire in %s. Please update the cookie soon using the /updateaccount command.", account.Title, FormatExpirationTime(account.SSOCookieExpiration)),
			Color:       0xFFA500, // Orange color for warning
			Timestamp:   time.Now().Format(time.RFC3339),
		}
		err := sendNotification(discord, account, embed, "")
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send SSO cookie expiration notification for account %s", account.Title)
		}
	}

	// Skip checking if the account is already permanently banned
	if account.IsPermabanned {
		logger.Log.WithField("account", account.Title).Info("Skipping permanently banned account")
		return
	}

	// Get user's captcha key
	userSettings, err := GetUserSettings(account.UserID)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", account.UserID)
		return
	}

	// Check the account status
	result, err := CheckAccount(account.SSOCookie, userSettings.CaptchaAPIKey)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check account %s: possible expired SSO Cookie", account.Title)
		return
	}

	// Handle invalid cookie status
	if result == models.StatusInvalidCookie {
		lastNotification := time.Unix(account.LastCookieNotification, 0)
		if time.Since(lastNotification).Hours() >= userSettings.CooldownDuration || account.LastCookieNotification == 0 {
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
		if time.Since(lastStatusChange).Hours() < userSettings.StatusChangeCooldown {
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
		err = sendNotification(discord, account, embed, fmt.Sprintf("<@%s>", account.UserID))
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

// SendGlobalAnnouncement function: sends a global announcement to users who haven't seen it yet
func SendGlobalAnnouncement(s *discordgo.Session, userID string) error {
	var userSettings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&userSettings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings for global announcement")
		return result.Error
	}

	if !userSettings.HasSeenAnnouncement {
		var channelID string
		var err error

		if userSettings.NotificationType == "dm" {
			channel, err := s.UserChannelCreate(userID)
			if err != nil {
				logger.Log.WithError(err).Error("Error creating DM channel for global announcement")
				return err
			}
			channelID = channel.ID
		} else {
			// Find the most recent channel used by the user
			var account models.Account
			if err := database.DB.Where("user_id = ?", userID).Order("updated_at DESC").First(&account).Error; err != nil {
				logger.Log.WithError(err).Error("Error finding recent channel for user")
				return err
			}
			channelID = account.ChannelID
		}

		announcementEmbed := createAnnouncementEmbed()

		_, err = s.ChannelMessageSendEmbed(channelID, announcementEmbed)
		if err != nil {
			logger.Log.WithError(err).Error("Error sending global announcement")
			return err
		}

		userSettings.HasSeenAnnouncement = true
		if err := database.DB.Save(&userSettings).Error; err != nil {
			logger.Log.WithError(err).Error("Error updating user settings after sending global announcement")
			return err
		}
	}

	return nil
}

func SendAnnouncementToAllUsers(s *discordgo.Session) error {
	var users []models.UserSettings
	if err := database.DB.Find(&users).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching all users")
		return err
	}

	for _, user := range users {
		if err := SendGlobalAnnouncement(s, user.UserID); err != nil {
			logger.Log.WithError(err).Errorf("Failed to send announcement to user %s", user.UserID)
		}
	}

	return nil
}

func createAnnouncementEmbed() *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: "Important Announcement: Changes to COD Status Bot",
		Description: "Due to the high demand and usage of our bot, we've reached the limit of our free EZCaptcha tokens. " +
			"To continue using the check ban feature, users now need to provide their own EZCaptcha API key.\n\n" +
			"Here's what you need to know:",
		Color: 0xFFD700, // Gold color
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "How to Get Your Own API Key",
				Value: "1. Visit our [referral link](https://dashboard.ez-captcha.com/#/register?inviteCode=uyNrRgWlEKy) to sign up for EZCaptcha\n" +
					"2. Request a free trial of 10,000 tokens\n" +
					"3. Use the `/setcaptchaservice` command to set your API key in the bot",
			},
			{
				Name: "Benefits of Using Your Own API Key",
				Value: "• Continue using the check ban feature\n" +
					"• Customize your check intervals\n" +
					"• Support the bot indirectly through our referral program",
			},
			{
				Name:  "Our Commitment",
				Value: "We're working on ways to maintain a free tier for all users. Your support by using our referral link helps us achieve this goal.",
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Thank you for your understanding and continued support!",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
}
