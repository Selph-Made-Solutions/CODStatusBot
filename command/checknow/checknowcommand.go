package checknow

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
	"strconv"
	"strings"
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

func CommandCheckNow(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var userID string
	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	} else {
		logger.Log.Error("Interaction doesn't have Member or User")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	userSettings, err := services.GetUserSettings(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Error fetching user settings")
		respondToInteraction(s, i, "Error fetching user settings. Please try again later.")
		return
	}

	if userSettings.CaptchaAPIKey == "" {
		if !checkRateLimit(userID) {
			respondToInteraction(s, i, fmt.Sprintf("You're using this command too frequently. Please wait %v before trying again, or set up your own API key for unlimited use.", rateLimit))
			return
		}
	}

	var accountTitle string
	if len(i.ApplicationCommandData().Options) > 0 {
		accountTitle = i.ApplicationCommandData().Options[0].StringValue()
	}

	var accounts []models.Account
	query := database.DB.Where("user_id = ?", userID)
	if accountTitle != "" {
		query = query.Where("title = ?", accountTitle)
	}
	result := query.Find(&accounts)

	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching accounts")
		respondToInteraction(s, i, "Error fetching accounts. Please try again later.")
		return
	}

	if len(accounts) == 0 {
		respondToInteraction(s, i, "You don't have any monitored accounts.")
		return
	}

	if len(accounts) == 1 || accountTitle != "" {
		// If only one account or a specific account was requested, check it immediately
		checkAccounts(s, i, accounts)
	} else {
		// If multiple accounts and no specific account was requested, show buttons
		showAccountButtons(s, i, accounts)
	}
}

func showAccountButtons(s *discordgo.Session, i *discordgo.InteractionCreate, accounts []models.Account) {
	var components []discordgo.MessageComponent
	for _, account := range accounts {
		components = append(components, discordgo.Button{
			Label:    account.Title,
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("check_now_%d", account.ID),
		})
	}

	components = append(components, discordgo.Button{
		Label:    "Check All",
		Style:    discordgo.SuccessButton,
		CustomID: "check_now_all",
	})

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Select an account to check, or 'Check All' to check all accounts:",
			Flags:   discordgo.MessageFlagsEphemeral,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: components,
				},
			},
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Error responding with account selection")
	}
}

func HandleAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	if customID == "check_now_all" {
		var accounts []models.Account
		result := database.DB.Where("user_id = ?", i.Member.User.ID).Find(&accounts)
		if result.Error != nil {
			logger.Log.WithError(result.Error).Error("Error fetching accounts")
			respondToInteraction(s, i, "Error fetching accounts. Please try again later.")
			return
		}
		checkAccounts(s, i, accounts)
		return
	}

	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "check_now_"))
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing account ID")
		respondToInteraction(s, i, "Error processing your selection. Please try again.")
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		respondToInteraction(s, i, "Error: Account not found or you don't have permission to check it.")
		return
	}

	checkAccounts(s, i, []models.Account{account})
}

func checkAccounts(s *discordgo.Session, i *discordgo.InteractionCreate, accounts []models.Account) {
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

	var embeds []*discordgo.MessageEmbed

	for _, account := range accounts {
		if account.IsExpiredCookie {
			embed := &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("%s - Expired Cookie", account.Title),
				Description: "The SSO cookie for this account has expired. Please update it using the /updateaccount command.",
				Color:       0xFF0000, // Red color for expired cookie
			}
			embeds = append(embeds, embed)
			continue
		}

		status, err := services.CheckAccount(account.SSOCookie, account.UserID)
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
		} else {
			logger.Log.Infof("Updated LastCheck for account %s to %v", account.Title, account.LastCheck)
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
		_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Embeds: embeds[j:end],
			Flags:  discordgo.MessageFlagsEphemeral,
		})
		if err != nil {
			logger.Log.WithError(err).Error("Failed to send follow-up message")
		}
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

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	var err error
	if i.Type == discordgo.InteractionMessageComponent {
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    content,
				Components: []discordgo.MessageComponent{},
				Flags:      discordgo.MessageFlagsEphemeral,
			},
		})
	} else {
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: content,
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}
