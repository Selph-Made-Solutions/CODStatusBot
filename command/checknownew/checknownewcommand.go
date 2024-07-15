package checknownew

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	rateLimiter     = make(map[string]time.Time)
	rateLimiterLock sync.Mutex
	rateLimit       time.Duration
)

func init() {
	rateLimitStr := os.Getenv("CHECK_NOW_RATE_LIMIT")
	rateLimitSeconds, err := strconv.Atoi(rateLimitStr)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse CHECK_NOW_RATE_LIMIT, using default of 60 seconds")
		rateLimitSeconds = 60
	}
	rateLimit = time.Duration(rateLimitSeconds) * time.Second
}

func RegisterCommand(s *discordgo.Session, guildID string) {
	command := &discordgo.ApplicationCommand{
		Name:        "checknownew",
		Description: "Immediately check the status of all your accounts or a specific account",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "account_title",
				Description: "The title of the account to check (leave empty to check all accounts)",
				Required:    false,
			},
		},
	}

	_, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, command)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating checknownew command")
	}
}

func UnregisterCommand(s *discordgo.Session, guildID string) {
	commands, err := s.ApplicationCommands(s.State.User.ID, guildID)
	if err != nil {
		logger.Log.WithError(err).Error("Error getting application commands")
		return
	}

	for _, command := range commands {
		if command.Name == "checknownew" {
			err := s.ApplicationCommandDelete(s.State.User.ID, guildID, command.ID)
			if err != nil {
				logger.Log.WithError(err).Error("Error deleting checknownew command")
			}
			return
		}
	}
}

func CommandCheckNowNew(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to defer interaction response")
		return
	}

	userID := i.Member.User.ID
	if !checkRateLimit(userID) {
		sendFollowUpMessage(s, i, fmt.Sprintf("You're using this command too frequently. Please wait %v before trying again.", rateLimit))
		return
	}

	var accountTitle string
	if len(i.ApplicationCommandData().Options) > 0 {
		accountTitle = i.ApplicationCommandData().Options[0].StringValue()
	}

	var accounts []models.Account
	query := database.DB.Where("user_id = ? AND guild_id = ?", userID, i.GuildID)
	if accountTitle != "" {
		query = query.Where("title = ?", accountTitle)
	}
	result := query.Find(&accounts)

	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching accounts")
		sendFollowUpMessage(s, i, "Error fetching accounts. Please try again later.")
		return
	}

	if len(accounts) == 0 {
		sendFollowUpMessage(s, i, "No accounts found to check.")
		return
	}

	var embeds []*discordgo.MessageEmbed

	for _, account := range accounts {
		if account.IsExpiredCookie {
			embed := &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("%s - Expired Cookie", account.Title),
				Description: "The SSO cookie for this account has expired. Please update it using the /updateaccountnew command.",
				Color:       0xFF0000, // Red color for expired cookie
			}
			embeds = append(embeds, embed)
			continue
		}

		status, err := services.CheckAccount(account.SSOCookie)
		if err != nil {
			logger.Log.WithError(err).Errorf("Error checking account status for %s", account.Title)
			embed := &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("%s - Error", account.Title),
				Description: "An error occurred while checking this account's status.",
				Color:       0xFF0000, // Red color for error
			}
			embeds = append(embeds, embed)
			continue
		}

		account.LastStatus = status
		account.LastCheck = time.Now().Unix()
		if err := database.DB.Save(&account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Failed to update account %s after check", account.Title)
		}

		embed := &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("%s - %s", account.Title, status),
			Description: fmt.Sprintf("Current status: %s", status),
			Color:       services.GetColorForStatus(status, account.IsExpiredCookie),
			Timestamp:   time.Now().Format(time.RFC3339),
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Last Checked",
					Value:  time.Unix(account.LastCheck, 0).Format(time.RFC1123),
					Inline: true,
				},
			},
		}
		embeds = append(embeds, embed)
	}

	// Send embeds in batches of 10 (Discord's limit)
	for j := 0; j < len(embeds); j += 10 {
		end := j + 10
		if end > len(embeds) {
			end = len(embeds)
		}
		sendFollowUpMessage(s, i, "", embeds[j:end]...)
	}
}

func checkRateLimit(userID string) bool {
	rateLimiterLock.Lock()
	defer rateLimiterLock.Unlock()

	lastUse, exists := rateLimiter[userID]
	if !exists || time.Since(lastUse) >= rateLimit {
		rateLimiter[userID] = time.Now()
		return true
	}
	return false
}

func sendFollowUpMessage(s *discordgo.Session, i *discordgo.InteractionCreate, content string, embeds ...*discordgo.MessageEmbed) {
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: content,
		Embeds:  embeds,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send follow-up message")
	}
}
