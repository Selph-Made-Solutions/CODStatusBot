package services

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
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
func sendNotification(client bot.Client, account models.Account, embed *discord.Embed, content string) error {
	if !canSendNotification(account.UserID) {
		logger.Log.Infof("Skipping notification for user %s (global cooldown)", account.UserID)
		return nil
	}

	var channelID discord.Snowflake
	var err error
	if account.NotificationType == "dm" {
		channel, err := client.Rest().CreateDM(discord.Snowflake(account.UserID))
		if err != nil {
			return fmt.Errorf("failed to create DM channel: %v", err)
		}
		channelID = channel.ID
	} else {
		channelID = discord.Snowflake(account.ChannelID)
	}

	_, err = client.Rest().CreateMessage(channelID, discord.NewMessageCreateBuilder().
		SetEmbeds(embed).
		SetContent(content).
		Build())
	return err
}

// sendDailyUpdate function: sends a daily update message for a given account
func sendDailyUpdate(account models.Account, client bot.Client) {
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
			description = fmt.Sprintf("The last status of account %s was %s. SSO cookie will expire in %s.", account.Title, account.LastStatus.Overall, FormatExpirationTime(account.SSOCookieExpiration))
		} else {
			description = fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title)
		}
	}

	embed := discord.NewEmbedBuilder().
		SetTitle(fmt.Sprintf("%.2f Hour Update - %s", notificationInterval, account.Title)).
		SetDescription(description).
		SetColor(GetColorForStatus(account.LastStatus.Overall)).
		SetTimestamp(time.Now()).
		Build()

	// Send the notification
	err := sendNotification(client, account, embed, "")
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
func CheckAccounts(client bot.Client) {
	for {
		logger.Log.Info("Starting periodic account check")
		var accounts []models.Account
		if err := database.DB.Find(&accounts).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to fetch accounts from the database")
			time.Sleep(time.Duration(sleepDuration) * time.Minute)
			continue
		}

		for i := range accounts {
			// Ensure LastStatus is properly initialized
			if accounts[i].LastStatus.Games == nil {
				accounts[i].LastStatus.Games = make(map[string]models.GameStatus)
			}
		}

		var wg sync.WaitGroup
		for _, account := range accounts {
			wg.Add(1)
			go func(acc models.Account) {
				defer wg.Done()
				checkSingleAccount(acc, client)
			}(account)
		}
		wg.Wait()

		time.Sleep(time.Duration(sleepDuration) * time.Minute)
	}
}

func checkSingleAccount(account models.Account, client bot.Client) {
	if account.IsCheckDisabled {
		logger.Log.Infof("Skipping check for disabled account: %s", account.Title)
		return
	}

	userSettings, err := GetUserSettings(account.UserID, account.InstallationType)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", account.UserID)
		return
	}

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
				go sendDailyUpdate(account, client)
			}
			account.LastCookieCheck = time.Now().Unix()
			if err := database.DB.Save(&account).Error; err != nil {
				logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
			}
		}
		logger.Log.WithField("account", account.Title).Info("Skipping permanently banned account")
		return
	}

	// Handle accounts with expired cookies
	if account.IsExpiredCookie {
		logger.Log.WithField("account", account.Title).Info("Skipping account with expired cookie")
		if time.Since(lastNotification).Hours() > userSettings.NotificationInterval {
			go sendDailyUpdate(account, client)
		} else {
			logger.Log.WithField("account", account.Title).Infof("Owner of %s recently notified within %.2f hours already, skipping", account.Title, userSettings.NotificationInterval)
		}
		return
	}

	// Check account status if enough time has passed since the last check
	if time.Since(lastCheck).Minutes() > float64(userSettings.CheckInterval) {
		go CheckSingleAccountStatus(account, client)
	} else {
		logger.Log.WithField("account", account.Title).Infof("Account %s checked recently less than %.2f minutes ago, skipping", account.Title, float64(userSettings.CheckInterval))
	}

	// Send daily update if enough time has passed since the last notification
	if time.Since(lastNotification).Hours() > userSettings.NotificationInterval {
		go sendDailyUpdate(account, client)
	} else {
		logger.Log.WithField("account", account.Title).Infof("Owner of %s recently notified within %.2f hours already, skipping", account.Title, userSettings.NotificationInterval)
	}
}

// CheckSingleAccount function: checks the status of a single account
func CheckSingleAccountStatus(account models.Account, client bot.Client) {
	// Check SSO cookie expiration
	timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check SSO cookie expiration for account %s", account.Title)
	} else if timeUntilExpiration > 0 && timeUntilExpiration <= 24*time.Hour {
		// Notify user if the cookie will expire within 24 hours
		embed := discord.NewEmbedBuilder().
			SetTitle(fmt.Sprintf("%s - SSO Cookie Expiring Soon", account.Title)).
			SetDescription(fmt.Sprintf("The SSO cookie for account %s will expire in %s. Please update the cookie soon using the /updateaccount command.", account.Title, FormatExpirationTime(account.SSOCookieExpiration))).
			SetColor(0xFFA500). // Orange color for warning
			SetTimestamp(time.Now()).
			Build()
		err := sendNotification(client, account, embed, "")
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send SSO cookie expiration notification for account %s", account.Title)
		}
	}

	// Skip checking if the account is already permanently banned
	if account.IsPermabanned {
		logger.Log.WithField("account", account.Title).Info("Skipping permanently banned account")
		return
	}

	result, err := CheckAccount(account.SSOCookie, account.UserID, models.InstallTypeUser)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check account %s: possible expired SSO Cookie", account.Title)
		return
	}

	// Handle invalid cookie status
	if result.Overall == models.StatusInvalidCookie {
		handleInvalidCookie(account, client)
		return
	}

	// Update account information
	DBMutex.Lock()
	lastStatus := account.LastStatus
	account.LastStatus = result
	account.LastCheck = time.Now().Unix()
	account.IsExpiredCookie = false
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
		DBMutex.Unlock()
		return
	}
	DBMutex.Unlock()

	// Handle status changes and send notifications
	if result.Overall != lastStatus.Overall {
		handleStatusChange(account, result, client)
	}
}

func handleInvalidCookie(account models.Account, client bot.Client) {
	lastNotification := time.Unix(account.LastCookieNotification, 0)
	userSettings, _ := GetUserSettings(account.UserID, account.InstallationType)
	if time.Since(lastNotification).Hours() >= userSettings.CooldownDuration || account.LastCookieNotification == 0 {
		logger.Log.Infof("Account %s has an invalid SSO cookie", account.Title)
		embed := discord.NewEmbedBuilder().
			SetTitle(fmt.Sprintf("%s - Invalid SSO Cookie", account.Title)).
			SetDescription(fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title)).
			SetColor(0xff9900).
			SetTimestamp(time.Now()).
			Build()

		err := sendNotification(client, account, embed, "")
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
}

func handleStatusChange(account models.Account, newStatus models.AccountStatus, client bot.Client) {
	lastStatusChange := time.Unix(account.LastStatusChange, 0)
	userSettings, _ := GetUserSettings(account.UserID, account.InstallationType)
	if time.Since(lastStatusChange).Hours() < userSettings.StatusChangeCooldown {
		logger.Log.Infof("Skipping status change notification for account %s (cooldown)", account.Title)
		return
	}

	DBMutex.Lock()
	account.LastStatus = newStatus
	account.LastStatusChange = time.Now().Unix()
	if newStatus.Overall == models.StatusPermaban {
		account.IsPermabanned = true
	} else {
		account.IsPermabanned = false
	}
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
		DBMutex.Unlock()
		return
	}
	logger.Log.Infof("Account %s status changed to %s", account.Title, newStatus.Overall)

	// Create a new record for the account
	ban := models.Ban{
		Account:   account,
		Status:    newStatus.Overall,
		AccountID: account.ID,
	}
	if err := database.DB.Create(&ban).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to create new ban record for account %s", account.Title)
	}
	DBMutex.Unlock()
	logger.Log.Infof("Account %s status changed to %s", account.Title, newStatus.Overall)

	embed := createStatusChangeEmbed(account.Title, newStatus)
	err := sendNotification(client, account, embed, fmt.Sprintf("<@%s>", account.UserID))
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send status update message for account %s", account.Title)
	}
}

func createStatusChangeEmbed(accountTitle string, status models.AccountStatus) *discord.Embed {
	embedBuilder := discord.NewEmbedBuilder().
		SetTitle(fmt.Sprintf("%s - Status Change", accountTitle)).
		SetDescription(fmt.Sprintf("The overall status of account %s has changed to %s", accountTitle, status.Overall)).
		SetColor(GetColorForStatus(status.Overall)).
		SetTimestamp(time.Now())

	for game, gameStatus := range status.Games {
		var statusDesc string
		switch gameStatus.Status {
		case models.StatusGood:
			statusDesc = "Good Standing"
		case models.StatusPermaban:
			statusDesc = "Permanently Banned"
		case models.StatusShadowban:
			statusDesc = "Under Review"
		case models.StatusTempban:
			duration := FormatBanDuration(gameStatus.DurationSeconds)
			statusDesc = fmt.Sprintf("Temporarily Banned (%s remaining)", duration)
		default:
			statusDesc = "Unknown Status"
		}

		embedBuilder.AddField(game, statusDesc, true)
	}

	return embedBuilder.Build()
}

// GetColorForStatus function: returns the appropriate color for an embed message based on the account status
func GetColorForStatus(status models.Status) int {
	switch status {
	case models.StatusPermaban:
		return 0xff0000 // Red for permanent ban
	case models.StatusShadowban:
		return 0xffff00 // Yellow for shadowban
	case models.StatusTempban:
		return 0xffa500 // Orange for temporary ban
	case models.StatusGood:
		return 0x00ff00 // Green for no ban
	default:
		return 0x808080 // Gray for unknown status
	}
}

// FormatBanDuration converts the duration in seconds to a human-readable string
func FormatBanDuration(seconds int) string {
	duration := time.Duration(seconds) * time.Second
	if duration < time.Hour {
		return fmt.Sprintf("%d minutes", int(duration.Minutes()))
	}
	return fmt.Sprintf("%d hours", int(duration.Hours()))
}

func SendGlobalAnnouncement(client bot.Client, userID string) error {
	var userSettings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&userSettings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings for global announcement")
		return result.Error
	}

	if !userSettings.HasSeenAnnouncement {
		var channelID discord.Snowflake
		var err error

		if userSettings.NotificationType == "dm" {
			channel, err := client.Rest().CreateDM(discord.Snowflake(userID))
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
			channelID = discord.Snowflake(account.ChannelID)
		}

		announcementEmbed := createAnnouncementEmbed()

		_, err = client.Rest().CreateMessage(channelID, discord.NewMessageCreateBuilder().
			SetEmbeds(announcementEmbed).
			Build())
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

func SendAnnouncementToAllUsers(client bot.Client) error {
	var users []models.UserSettings
	if err := database.DB.Find(&users).Error; err != nil {
		logger.Log.WithError(err).Error("Error fetching all users")
		return err
	}

	for _, user := range users {
		if err := SendGlobalAnnouncement(client, user.UserID); err != nil {
			logger.Log.WithError(err).Errorf("Failed to send announcement to user %s", user.UserID)
		}
	}

	return nil
}

func createAnnouncementEmbed() *discord.Embed {
	return discord.NewEmbedBuilder().
		SetTitle("Important Announcement: Changes to COD Status Bot").
		SetDescription("Due to the high demand and usage of our bot, we've reached the limit of our free EZCaptcha tokens. " +
			"To continue using the check ban feature, users now need to provide their own EZCaptcha API key.\n\n" +
			"Here's what you need to know:").
		SetColor(0xFFD700). // Gold color
		AddField("How to Get Your Own API Key",
			"1. Visit our [referral link](https://dashboard.ez-captcha.com/#/register?inviteCode=uyNrRgWlEKy) to sign up for EZCaptcha\n"+
				"2. Request a free trial of 10,000 tokens\n"+
				"3. Use the `/setcaptchaservice` command to set your API key in the bot", false).
		AddField("Benefits of Using Your Own API Key",
			"• Continue using the check ban feature\n"+
				"• Customize your check intervals\n"+
				"• Support the bot indirectly through our referral program", false).
		AddField("Our Commitment",
			"We're working on ways to maintain a free tier for all users. Your support by using our referral link helps us achieve this goal.", false).
		SetFooter("Thank you for your understanding and continued support!", "").
		SetTimestamp(time.Now()).
		Build()
}