package services

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

var (
	checkInterval               float64                      // Check interval for accounts (in minutes)
	notificationInterval        float64                      // Notification interval for daily updates (in hours)
	cooldownDuration            float64                      // Cooldown duration for invalid cookie notifications (in hours)
	sleepDuration               int                          // Sleep duration for the account checking loop (in minutes)
	cookieCheckIntervalPermaban float64                      // Check interval for permabanned accounts (in hours)
	statusChangeCooldown        float64                      // Cooldown duration for status change notifications (in hours)
	globalNotificationCooldown  float64                      // Global cooldown for notifications per user (in hours)
	userNotificationTimestamps  = make(map[string]time.Time) // Map to track user notification timestamps
	userNotificationMutex       sync.Mutex                   // Mutex to protect user notification timestamps
	DBMutex                     sync.Mutex                   // Mutex to protect database access
)

func init() {
	if err := godotenv.Load(); err != nil {
		logger.Log.WithError(err).Error("Failed to load .env file")
	}

	loadEnvironmentVariables()
}

func loadEnvironmentVariables() {
	var err error
	checkInterval, err = strconv.ParseFloat(getEnvWithDefault("CHECK_INTERVAL", "15"), 64)
	notificationInterval, err = strconv.ParseFloat(getEnvWithDefault("NOTIFICATION_INTERVAL", "24"), 64)
	cooldownDuration, err = strconv.ParseFloat(getEnvWithDefault("COOLDOWN_DURATION", "6"), 64)
	sleepDuration, err = strconv.Atoi(getEnvWithDefault("SLEEP_DURATION", "5"))
	cookieCheckIntervalPermaban, err = strconv.ParseFloat(getEnvWithDefault("COOKIE_CHECK_INTERVAL_PERMABAN", "24"), 64)
	statusChangeCooldown, err = strconv.ParseFloat(getEnvWithDefault("STATUS_CHANGE_COOLDOWN", "1"), 64)
	globalNotificationCooldown, err = strconv.ParseFloat(getEnvWithDefault("GLOBAL_NOTIFICATION_COOLDOWN", "1"), 64)

	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse environment variables")
	}

	logger.Log.Infof("Loaded config: CHECK_INTERVAL=%.2f, NOTIFICATION_INTERVAL=%.2f, COOLDOWN_DURATION=%.2f, SLEEP_DURATION=%d, COOKIE_CHECK_INTERVAL_PERMABAN=%.2f, STATUS_CHANGE_COOLDOWN=%.2f, GLOBAL_NOTIFICATION_COOLDOWN=%.2f",
		checkInterval, notificationInterval, cooldownDuration, sleepDuration, cookieCheckIntervalPermaban, statusChangeCooldown, globalNotificationCooldown)
}

func getEnvWithDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
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

	channelID, err := getNotificationChannel(discord, account)
	if err != nil {
		return err
	}

	_, err = discord.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed:   embed,
		Content: content,
	})
	return err
}

// sendDailyUpdate function: sends a daily update message for a given account
func getNotificationChannel(discord *discordgo.Session, account models.Account) (string, error) {
	if account.NotificationType == "dm" {
		channel, err := discord.UserChannelCreate(account.UserID)
		if err != nil {
			return "", fmt.Errorf("failed to create DM channel: %v", err)
		}
		return channel.ID, nil
	}
	return account.ChannelID, nil
}

func sendDailyUpdate(account models.Account, discord *discordgo.Session) {
	logger.Log.Infof("Sending daily update for account %s", account.Title)

	// Prepare the description based on the account's cookie status
	description := getDailyUpdateDescription(account)
	embed := createDailyUpdateEmbed(account, description)

	err := sendNotification(discord, account, embed, "")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send scheduled update message for account %s", account.Title)
		return
	}

	updateAccountTimestamps(&account)
}

func getDailyUpdateDescription(account models.Account) string {
	if account.IsExpiredCookie {
		return fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title)
	}

	timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
	if err != nil {
		logger.Log.WithError(err).Errorf("Error checking SSO cookie expiration for account %s", account.Title)
		return fmt.Sprintf("An error occurred while checking the SSO cookie expiration for account %s. Please check the account status manually.", account.Title)
	}

	if timeUntilExpiration > 0 {
		return fmt.Sprintf("The last status of account %s was %s. SSO cookie will expire in %s.", account.Title, account.LastStatus.Overall, FormatExpirationTime(account.SSOCookieExpiration))
	}

	return fmt.Sprintf("The SSO cookie for account %s has expired. Please update the cookie using the /updateaccount command or delete the account using the /removeaccount command.", account.Title)
}

func createDailyUpdateEmbed(account models.Account, description string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%.2f Hour Update - %s", notificationInterval, account.Title),
		Description: description,
		Color:       GetColorForStatus(account.LastStatus.Overall),
		Timestamp:   time.Now().Format(time.RFC3339),
	}
}

// Update the account's last check and notification timestamps
func updateAccountTimestamps(account *models.Account) {
	DBMutex.Lock()
	defer DBMutex.Unlock()

	account.LastCheck = time.Now().Unix()
	account.LastNotification = time.Now().Unix()
	if err := database.GetDB().Save(account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
	}
}

// CheckAccounts function: periodically checks all accounts for status changes
func CheckAccounts(s *discordgo.Session) {
	for {
		logger.Log.Info("Starting periodic account check")
		var accounts []models.Account
		if err := database.GetDB().Find(&accounts).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to fetch accounts from the database")
			time.Sleep(time.Duration(sleepDuration) * time.Minute)
			continue
		}

		for _, account := range accounts {
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Log.Errorf("Recovered from panic in CheckAccounts: %v", r)
					}
				}()
				checkSingleAccount(account, s)
			}()
			// Add a small delay between account checks
			time.Sleep(5 * time.Second)
		}

		logger.Log.Info("Finished periodic account check")
		time.Sleep(time.Duration(sleepDuration) * time.Minute)
	}
}

func checkAccountsBatch(accounts []models.Account, s *discordgo.Session) {
	batchSize := 50
	for i := 0; i < len(accounts); i += batchSize {
		end := i + batchSize
		if end > len(accounts) {
			end = len(accounts)
		}

		var wg sync.WaitGroup
		for _, account := range accounts[i:end] {
			wg.Add(1)
			go func(acc models.Account) {
				defer wg.Done()
				checkSingleAccount(acc, s)
			}(account)
		}
		wg.Wait()
	}
}

func checkSingleAccount(account models.Account, s *discordgo.Session) {
	if account.IsCheckDisabled {
		logger.Log.Infof("Skipping check for disabled account: %s", account.Title)
		return
	}

	userSettings, err := GetUserSettings(account.UserID, account.InstallationType)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to get user settings for user %s", account.UserID)
		return
	}

	if account.IsPermabanned {
		handlePermabannedAccount(&account, s)
		return
	}

	if account.IsExpiredCookie {
		handleExpiredCookieAccount(account, s, userSettings)
		return
	}

	if time.Since(time.Unix(account.LastCheck, 0)).Minutes() > float64(userSettings.CheckInterval) {
		go CheckSingleAccountStatus(account, s)
	} else {
		logger.Log.WithField("account", account.Title).Infof("Account %s checked recently less than %.2f minutes ago, skipping", account.Title, float64(userSettings.CheckInterval))
	}

	if time.Since(time.Unix(account.LastNotification, 0)).Hours() > userSettings.NotificationInterval {
		go sendDailyUpdate(account, s)
	} else {
		logger.Log.WithField("account", account.Title).Infof("Owner of %s recently notified within %.2f hours already, skipping", account.Title, userSettings.NotificationInterval)
	}
}

func handlePermabannedAccount(account *models.Account, s *discordgo.Session) {
	lastCookieCheck := time.Unix(account.LastCookieCheck, 0)
	if time.Since(lastCookieCheck).Hours() > cookieCheckIntervalPermaban {
		if !VerifySSOCookie(account.SSOCookie) {
			account.IsExpiredCookie = true
			if err := database.GetDB().Save(account).Error; err != nil {
				logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
			}
			go sendDailyUpdate(*account, s)
		}
		account.LastCookieCheck = time.Now().Unix()
		if err := database.GetDB().Save(account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
		}
	}
	logger.Log.WithField("account", account.Title).Info("Skipping permanently banned account")
}

// Handle accounts with expired cookies
func handleExpiredCookieAccount(account models.Account, s *discordgo.Session, userSettings models.UserSettings) {
	logger.Log.WithField("account", account.Title).Info("Skipping account with expired cookie")
	if time.Since(time.Unix(account.LastNotification, 0)).Hours() > userSettings.NotificationInterval {
		go sendDailyUpdate(account, s)
	} else {
		logger.Log.WithField("account", account.Title).Infof("Owner of %s recently notified within %.2f hours already, skipping", account.Title, userSettings.NotificationInterval)
	}
}

// CheckSingleAccount function: checks the status of a single account
func CheckSingleAccountStatus(account models.Account, discord *discordgo.Session) {
	// Check SSO cookie expiration
	checkSSOCookieExpiration(account, discord)

	if account.IsPermabanned {
		logger.Log.WithField("account", account.Title).Info("Skipping permanently banned account")
		return
	}

	result, err := CheckAccount(account.SSOCookie, account.UserID, models.InstallTypeUser)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check account %s: possible expired SSO Cookie", account.Title)
		return
	}

	if result.Overall == models.StatusInvalidCookie {
		handleInvalidCookie(account, discord)
		return
	}

	updateAccountStatus(&account, result)
	if result.Overall != account.LastStatus.Overall {
		handleStatusChange(account, result, discord)
	}
}

func checkSSOCookieExpiration(account models.Account, discord *discordgo.Session) {
	timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check SSO cookie expiration for account %s", account.Title)
		return
	}

	if timeUntilExpiration > 0 && timeUntilExpiration <= 24*time.Hour {
		// Notify user if the cookie will expire within 24 hours
		sendCookieExpirationNotification(account, discord, timeUntilExpiration)
	}
}

func sendCookieExpirationNotification(account models.Account, discord *discordgo.Session, timeUntilExpiration time.Duration) {
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
func updateAccountStatus(account *models.Account, result models.AccountStatus) {
	DBMutex.Lock()
	defer DBMutex.Unlock()
	account.LastStatus = result
	account.LastCheck = time.Now().Unix()
	account.IsExpiredCookie = false
	if err := database.GetDB().Save(account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
	}
}

func handleInvalidCookie(account models.Account, discord *discordgo.Session) {
	lastNotification := time.Unix(account.LastCookieNotification, 0)
	userSettings, _ := GetUserSettings(account.UserID, account.InstallationType)
	if time.Since(lastNotification).Hours() >= userSettings.CooldownDuration || account.LastCookieNotification == 0 {
		logger.Log.Infof("Account %s has an invalid SSO cookie", account.Title)
		sendInvalidCookieNotification(account, discord)
		updateInvalidCookieStatus(&account)
	} else {
		logger.Log.Infof("Skipping expired cookie notification for account %s (cooldown)", account.Title)
	}
}

func sendInvalidCookieNotification(account models.Account, discord *discordgo.Session) {
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
}

// Update account information regarding the expired cookie
func updateInvalidCookieStatus(account *models.Account) {
	DBMutex.Lock()
	defer DBMutex.Unlock()

	account.LastCookieNotification = time.Now().Unix()
	account.IsExpiredCookie = true
	if err := database.GetDB().Save(account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
	}
}

func handleStatusChange(account models.Account, newStatus models.AccountStatus, discord *discordgo.Session) {
	lastStatusChange := time.Unix(account.LastStatusChange, 0)
	userSettings, _ := GetUserSettings(account.UserID, account.InstallationType)
	if time.Since(lastStatusChange).Hours() < userSettings.StatusChangeCooldown {
		logger.Log.Infof("Skipping status change notification for account %s (cooldown)", account.Title)
		return
	}

	updateAccountStatusChange(&account, newStatus)
	logger.Log.Infof("Account %s status changed to %s", account.Title, newStatus.Overall)

	createBanRecord(account, newStatus)
	sendStatusChangeNotification(account, newStatus, discord)
}

func updateAccountStatusChange(account *models.Account, newStatus models.AccountStatus) {
	DBMutex.Lock()
	defer DBMutex.Unlock()

	account.LastStatus = newStatus
	account.LastStatusChange = time.Now().Unix()
	account.IsPermabanned = newStatus.Overall == models.StatusPermaban
	if err := database.GetDB().Save(account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to save account changes for account %s", account.Title)
	}
}

// Create a new record for the account
func createBanRecord(account models.Account, newStatus models.AccountStatus) {
	ban := models.Ban{
		Account:   account,
		Status:    newStatus.Overall,
		AccountID: account.ID,
	}
	if err := database.GetDB().Create(&ban).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to create new ban record for account %s", account.Title)
	}
}

func sendStatusChangeNotification(account models.Account, newStatus models.AccountStatus, discord *discordgo.Session) {
	embed := createStatusChangeEmbed(account.Title, newStatus)
	err := sendNotification(discord, account, embed, fmt.Sprintf("<@%s>", account.UserID))
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send status update message for account %s", account.Title)
	}
}

func createStatusChangeEmbed(accountTitle string, status models.AccountStatus) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Status Change", accountTitle),
		Description: fmt.Sprintf("The overall status of account %s has changed to %s", accountTitle, status.Overall),
		Color:       GetColorForStatus(status.Overall),
		Timestamp:   time.Now().Format(time.RFC3339),
		Fields:      []*discordgo.MessageEmbedField{},
	}

	for game, gameStatus := range status.Games {
		statusDesc := getGameStatusDescription(gameStatus)
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   game,
			Value:  statusDesc,
			Inline: true,
		})
	}

	return embed
}

func getGameStatusDescription(gameStatus models.GameStatus) string {
	switch gameStatus.Status {
	case models.StatusGood:
		return "Good Standing"
	case models.StatusPermaban:
		return "Permanently Banned"
	case models.StatusShadowban:
		return "Under Review"
	case models.StatusTempban:
		duration := FormatBanDuration(gameStatus.DurationSeconds)
		return fmt.Sprintf("Temporarily Banned (%s remaining)", duration)
	default:
		return "Unknown Status"
	}
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
	// SendGlobalAnnouncement function: sends a global announcement to users who haven't seen it yet
	return fmt.Sprintf("%d hours", int(duration.Hours()))
}

func SendGlobalAnnouncement(s *discordgo.Session, userID string) error {
	var userSettings models.UserSettings
	result := database.GetDB().Where(models.UserSettings{UserID: userID}).FirstOrCreate(&userSettings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings for global announcement")
		return result.Error
	}

	if !userSettings.HasSeenAnnouncement {
		channelID, err := getAnnouncementChannel(s, userSettings)
		if err != nil {
			logger.Log.WithError(err).Error("Error creating DM channel for global announcement")
			return err
		}

		announcementEmbed := createAnnouncementEmbed()

		_, err = s.ChannelMessageSendEmbed(channelID, announcementEmbed)
		if err != nil {
			logger.Log.WithError(err).Error("Error sending global announcement")
			return err
		}

		userSettings.HasSeenAnnouncement = true
		if err := database.GetDB().Save(&userSettings).Error; err != nil {
			logger.Log.WithError(err).Error("Error updating user settings after sending global announcement")
			return err
		}
	}

	return nil
}

func getAnnouncementChannel(s *discordgo.Session, userSettings models.UserSettings) (string, error) {
	if userSettings.NotificationType == "dm" {
		channel, err := s.UserChannelCreate(userSettings.UserID)
		if err != nil {
			logger.Log.WithError(err).Error("Error creating DM channel for global announcement")
			return "", err
		}
		return channel.ID, nil
		logger.Log.WithError(err).Errorf("Failed to send announcement to user %s")

	}

	var account models.Account
	if err := database.GetDB().Where("user_id = ?", userSettings.UserID).Order("updated_at DESC").First(&account).Error; err != nil {
		logger.Log.WithError(err).Error("Error finding recent channel for user")
		return "", err
	}
	return account.ChannelID, nil
}

func createAnnouncementEmbed() *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "Important Update: Changes to COD Status Bot",
		Description: "Due to high demand, we've reached our limit of free EZCaptcha tokens. To ensure continued functionality, we're introducing some changes:",
		Color:       0xFFD700, // Gold color
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "What's Changing",
				Value: "• The check ban feature now requires users to provide their own EZCaptcha API key.\n" +
					"• Without an API key, the bot's check ban functionality will be limited.",
			},
			{
				Name: "How to Get Your Own API Key",
				Value: "1. Sign up at [EZ-Captcha](https://dashboard.ez-captcha.com/#/register?inviteCode=uyNrRgWlEKy) using our referral link.\n" +
					"2. Request a free trial of 10,000 tokens.\n" +
					"3. Use the `/setcaptchaservice` command to set your API key in the bot.",
			},
			{
				Name: "Benefits of Using Your Own API Key",
				Value: "• Uninterrupted access to the check ban feature\n" +
					"• Ability to customize check intervals\n" +
					"• Support the bot's development through our referral program",
			},
			{
				Name: "Next Steps",
				Value: "1. Obtain your API key as soon as possible.\n" +
					"2. Set up your key using the `/setcaptchaservice` command.\n" +
					"3. Adjust your check interval preferences if desired.",
			},
			{
				Name:  "Our Commitment",
				Value: "We're actively exploring ways to maintain a free tier for all users. Your support through the referral program directly contributes to this goal.",
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Thank you for your understanding and continued support!",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
}
