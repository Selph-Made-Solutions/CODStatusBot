package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bradselph/CODStatusBot/config"
	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

const (
	maxConsecutiveErrors = 5
)

var (
	DBMutex sync.Mutex
)

func init() {
	cfg := config.Get()
	logger.Log.Infof("Loaded rate limits and intervals: CHECK_INTERVAL=%.2f, NOTIFICATION_INTERVAL=%.2f, "+
		"COOLDOWN_DURATION=%.2f, SLEEP_DURATION=%d, COOKIE_CHECK_INTERVAL_PERMABAN=%.2f, "+
		"STATUS_CHANGE_COOLDOWN=%.2f, GLOBAL_NOTIFICATION_COOLDOWN=%.2f, COOKIE_EXPIRATION_WARNING=%.2f, "+
		"TEMP_BAN_UPDATE_INTERVAL=%.2f, CHECK_NOW_RATE_LIMIT=%v, DEFAULT_RATE_LIMIT=%v",
		cfg.CheckInterval, cfg.NotificationInterval, cfg.CooldownDuration, cfg.SleepDuration,
		cfg.CookieCheckIntervalPermaban, cfg.StatusChangeCooldown, cfg.GlobalNotificationCooldown,
		cfg.CookieExpirationWarning, cfg.TempBanUpdateInterval, cfg.CheckNowRateLimit, cfg.DefaultRateLimit)
}

func GetEnvFloat(key string, fallback float64) float64 {
	value := GetEnvFloatRaw(key, fallback)
	if key == "CHECK_INTERVAL" || key == "SLEEP_DURATION" || key == "DEFAULT_RATE_LIMIT" {
		return value
	}
	return value
}

func GetEnvFloatRaw(key string, fallback float64) float64 {
	if value, ok := os.LookupEnv(key); ok {
		floatValue, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return floatValue
		}
		logger.Log.WithError(err).Errorf("Failed to parse %s, using fallback value", key)
	}
	return fallback
}

func GetEnvInt(key string, fallback int) int {
	return GetEnvIntRaw(key, fallback)
}

func GetEnvIntRaw(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		intValue, err := strconv.Atoi(value)
		if err == nil {
			return intValue
		}
		logger.Log.WithError(err).Errorf("Failed to parse %s, using fallback value", key)
	}
	return fallback
}

func CheckAccounts(s *discordgo.Session) {
	logger.Log.Info("Starting periodic account check")

	var accounts []models.Account
	if err := database.DB.Where("is_check_disabled = ? AND is_expired_cookie = ?", false, false).Find(&accounts).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to fetch accounts from database")
		return
	}

	accountsByUser := make(map[string][]models.Account)
	for _, account := range accounts {
		accountsByUser[account.UserID] = append(accountsByUser[account.UserID], account)
	}

	for userID, userAccounts := range accountsByUser {
		processUserAccounts(s, userID, userAccounts)
	}
}

func HandleStatusChange(s *discordgo.Session, account models.Account, newStatus models.Status, userSettings models.UserSettings) {
	cfg := config.Get()

	if account.IsPermabanned && newStatus == models.StatusPermaban {
		if account.LastNotification != 0 {
			logger.Log.Debugf("Account %s already notified of permaban, skipping notification", account.Title)
			return
		}
	}

	if account.LastStatus == newStatus {
		logger.Log.Debugf("No status change for account %s, skipping notification", account.Title)
		return
	}

	DBMutex.Lock()
	defer DBMutex.Unlock()

	now := time.Now()
	lastStatusChange := time.Unix(account.LastStatusChange, 0)
	if now.Sub(lastStatusChange) < time.Duration(userSettings.StatusChangeCooldown)*time.Hour {
		logger.Log.Debugf("Status change notification for account %s is on cooldown", account.Title)
		return
	}

	previousStatus := account.LastStatus
	account.LastStatus = newStatus
	account.LastStatusChange = now.Unix()
	account.IsPermabanned = newStatus == models.StatusPermaban
	account.IsShadowbanned = newStatus == models.StatusShadowban
	account.IsTempbanned = newStatus == models.StatusTempban
	account.LastSuccessfulCheck = now

	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to update account status")
		return
	}

	ban := models.Ban{
		AccountID: account.ID,
		Status:    newStatus,
	}

	if newStatus == models.StatusTempban {
		ban.TempBanDuration = calculateBanDuration(time.Now().Add(24 * time.Hour))
		ban.AffectedGames = getAffectedGames(account.SSOCookie)
	}

	if err := database.DB.Create(&ban).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to create ban record")
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - %s", account.Title, EmbedTitleFromStatus(newStatus)),
		Description: GetStatusDescription(newStatus, account.Title, ban),
		Color:       GetColorForStatus(newStatus, account.IsExpiredCookie, account.IsCheckDisabled),
		Fields:      getStatusFields(account, newStatus),
		Timestamp:   now.Format(time.RFC3339),
	}

	notificationType := getNotificationType(newStatus)

	if previousStatus != models.StatusUnknown {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Previous Status",
			Value:  string(previousStatus),
			Inline: true,
		})
	}

	err := SendNotification(s, account, embed, fmt.Sprintf("<@%s>", account.UserID), notificationType)
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send status update message for account %s", account.Title)
	} else {
		userSettings.LastStatusChangeNotification = now
		if err := database.DB.Save(&userSettings).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to update LastStatusChangeNotification for user %s", account.UserID)
		}
	}

	switch newStatus {
	case models.StatusTempban:
		go ScheduleTempBanNotification(s, account, ban.TempBanDuration)

	case models.StatusPermaban:
		permaBanEmbed := &discordgo.MessageEmbed{
			Title: fmt.Sprintf("%s - Permanent Ban Detected", account.Title),
			Description: "This account has been permanently banned. It's recommended to remove it from monitoring " +
				"using the /removeaccount command to free up your account slot.",
			Color:     GetColorForStatus(newStatus, false, false),
			Timestamp: now.Format(time.RFC3339),
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Account Status",
					Value:  "Permanently Banned",
					Inline: true,
				},
				{
					Name:   "Action Required",
					Value:  "Remove account using /removeaccount",
					Inline: true,
				},
				{
					Name:   "Note",
					Value:  "Removing this account will free up a slot for monitoring another account.",
					Inline: false,
				},
			},
		}

		if ban.AffectedGames != "" {
			permaBanEmbed.Fields = append(permaBanEmbed.Fields, &discordgo.MessageEmbedField{
				Name:   "Affected Games",
				Value:  ban.AffectedGames,
				Inline: false,
			})
		}

		if err := SendNotification(s, account, permaBanEmbed, "", "permaban_notice"); err != nil {
			logger.Log.WithError(err).Error("Failed to send permaban notice")
		}

		account.LastNotification = now.Unix()
	}

	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to save final account status")
	}
}

func getAffectedGames(ssoCookie string) string {
	cfg := config.Get()
	req, err := http.NewRequest("GET", cfg.CheckEndpoint, nil)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create request for affected games")
		return "All Games"
	}

	headers := GenerateHeaders(ssoCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get affected games")
		return "All Games"
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Log.WithError(err).Error("Failed to close response body")
		}
	}(resp.Body)

	var data struct {
		Bans []struct {
			AffectedTitles []string `json:"affectedTitles"`
		} `json:"bans"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		logger.Log.WithError(err).Error("Failed to decode affected games response")
		return "All Games"
	}

	affectedGames := make(map[string]bool)
	for _, ban := range data.Bans {
		for _, title := range ban.AffectedTitles {
			affectedGames[title] = true
		}
	}

	var games []string
	for game := range affectedGames {
		games = append(games, game)
	}

	if len(games) == 0 {
		return "All Games"
	}

	return strings.Join(games, ", ")
}
func getStatusFields(account models.Account, status models.Status) []*discordgo.MessageEmbedField {
	fields := []*discordgo.MessageEmbedField{
		{
			Name:   "Account Status",
			Value:  string(status),
			Inline: true,
		},
		{
			Name:   "Last Checked",
			Value:  time.Unix(account.LastCheck, 0).Format(time.RFC1123),
			Inline: true,
		},
	}

	if !account.IsExpiredCookie {
		timeUntilExpiration, err := CheckSSOCookieExpiration(account.SSOCookieExpiration)
		if err == nil {
			fields = append(fields, &discordgo.MessageEmbedField{
				Name:   "Cookie Expires",
				Value:  FormatDuration(timeUntilExpiration),
				Inline: true,
			})
		}
	}

	if account.Created > 0 {
		creationDate := time.Unix(account.Created, 0)
		accountAge := time.Since(creationDate)
		years := int(accountAge.Hours() / 24 / 365)
		months := int(accountAge.Hours()/24/30.44) % 12
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Account Age",
			Value:  fmt.Sprintf("%d years, %d months", years, months),
			Inline: true,
		})
	}

	switch status {
	case models.StatusPermaban:
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Ban Type",
			Value:  "Permanent",
			Inline: true,
		})

	case models.StatusTempban:
		var latestBan models.Ban
		if err := database.DB.Where("account_id = ?", account.ID).
			Order("created_at DESC").
			First(&latestBan).Error; err == nil {
			fields = append(fields, &discordgo.MessageEmbedField{
				Name:   "Ban Duration",
				Value:  latestBan.TempBanDuration,
				Inline: true,
			})
		}

	case models.StatusShadowban:
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Review Status",
			Value:  "Account Under Review",
			Inline: true,
		})
	}

	if account.ConsecutiveErrors > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Check Errors",
			Value:  fmt.Sprintf("%d consecutive errors", account.ConsecutiveErrors),
			Inline: true,
		})
	}

	return fields
}

func disableAccount(s *discordgo.Session, account models.Account, reason string) {
	account.IsCheckDisabled = true
	account.DisabledReason = reason

	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Failed to disable account %s", account.Title)
		return
	}

	logger.Log.Infof("Account %s has been disabled. Reason: %s", account.Title, reason)

	NotifyUserAboutDisabledAccount(s, account, reason)
}

func ScheduleTempBanNotification(s *discordgo.Session, account models.Account, duration string) {
	parts := strings.Split(duration, ",")
	if len(parts) != 2 {
		logger.Log.Errorf("Invalid duration format for account %s: %s", account.Title, duration)
		return
	}
	days, _ := strconv.Atoi(strings.TrimSpace(strings.Split(parts[0], " ")[0]))
	hours, _ := strconv.Atoi(strings.TrimSpace(strings.Split(parts[1], " ")[0]))

	sleepDuration := time.Duration(days)*24*time.Hour + time.Duration(hours)*time.Hour

	for remainingTime := sleepDuration; remainingTime > 0; remainingTime -= 24 * time.Hour {
		if remainingTime > 24*time.Hour {
			time.Sleep(24 * time.Hour)
		} else {
			time.Sleep(remainingTime)
		}

		embed := &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Update", account.Title),
			Description: fmt.Sprintf("Your account is still temporarily banned. Remaining time: %v", remainingTime),
			Color:       GetColorForStatus(models.StatusTempban, false, account.IsCheckDisabled),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
		err := SendNotification(s, account, embed, "", "temp_ban_update")
		if err != nil {
			logger.Log.WithError(err).Errorf("Failed to send temporary ban update for account %s", account.Title)
		}
	}

	result, err := CheckAccount(account.SSOCookie, account.UserID, "")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to check account %s after temporary ban duration", account.Title)
		return
	}

	var embed *discordgo.MessageEmbed
	if result == models.StatusGood {
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Lifted", account.Title),
			Description: fmt.Sprintf("The temporary ban for account %s has been lifted. The account is now in good standing.", account.Title),
			Color:       GetColorForStatus(result, false, account.IsCheckDisabled),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	} else if result == models.StatusPermaban {
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Escalated", account.Title),
			Description: fmt.Sprintf("The temporary ban for account %s has been escalated to a permanent ban.", account.Title),
			Color:       GetColorForStatus(result, false, account.IsCheckDisabled),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	} else {
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - Temporary Ban Update", account.Title),
			Description: fmt.Sprintf("The temporary ban for account %s is still in effect. Current status: %s", account.Title, result),
			Color:       GetColorForStatus(result, false, account.IsCheckDisabled),
			Timestamp:   time.Now().Format(time.RFC3339),
		}
	}

	err = SendNotification(s, account, embed, fmt.Sprintf("<@%s>", account.UserID), "temp_ban_update")
	if err != nil {
		logger.Log.WithError(err).Errorf("Failed to send temporary ban update message for account %s", account.Title)
	}
}

func getChannelForAnnouncement(s *discordgo.Session, userID string, userSettings models.UserSettings) (string, error) {
	if userSettings.NotificationType == "dm" {
		channel, err := s.UserChannelCreate(userID)
		if err != nil {
			return "", fmt.Errorf("failed to create DM channel: %w", err)
		}
		return channel.ID, nil
	}

	var account models.Account
	if err := database.DB.Where("user_id = ?", userID).Order("updated_at DESC").First(&account).Error; err != nil {
		channel, err := s.UserChannelCreate(userID)
		if err != nil {
			return "", fmt.Errorf("both channel lookup and DM creation failed: %w", err)
		}
		return channel.ID, nil
	}
	return account.ChannelID, nil
}

func calculateBanDuration(endTime time.Time) string {
	duration := time.Until(endTime)
	if duration < 0 {
		return "Expired"
	}

	days := int(duration.Hours()) / 24
	hours := int(duration.Hours()) % 24

	if days > 0 {
		return fmt.Sprintf("%d days, %d hours", days, hours)
	}
	return fmt.Sprintf("%d hours", hours)
}
