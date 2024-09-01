package checknow

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"strconv"
	"strings"
	"time"
)

func CommandCheckNow(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
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

	userSettings, err := services.GetUserSettings(userID, installType)
	if err != nil {
		logger.Log.WithError(err).Error("Error fetching user settings")
		respondToInteraction(s, i, "Error fetching user settings. Please try again later.")
		return
	}

	logger.Log.Infof("User %s initiated a check. API Key set: %v", userID, userSettings.CaptchaAPIKey != "")

	if userSettings.CaptchaAPIKey == "" {
		if !services.CheckDefaultKeyRateLimit(userID) {
			respondToInteraction(s, i, fmt.Sprintf("You're using the default API key too frequently. Please wait before trying again, or set up your own API key for unlimited use."))
			return
		}
	}

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)

	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching accounts")
		respondToInteraction(s, i, "Error fetching accounts. Please try again later.")
		return
	}

	if len(accounts) == 0 {
		respondToInteraction(s, i, "You don't have any monitored accounts.")
		return
	}

	showAccountButtons(s, i, accounts)
}

// Update other functions to include installType where necessary
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

// Update HandleAccountSelection to pass installType to checkAccounts
func HandleAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
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
		logger.Log.Infof("Checking account %s for user %s", account.Title, account.UserID)

		if account.IsExpiredCookie {
			embed := &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("%s - Expired Cookie", account.Title),
				Description: "The SSO cookie for this account has expired. Please update it using the /updateaccount command.",
				Color:       0xFF0000, // Red color for expired cookie
			}
			embeds = append(embeds, embed)
			continue
		}

		status, err := services.CheckAccount(account.SSOCookie, account.UserID, models.InstallTypeUser)
		if err != nil {
			logger.Log.WithError(err).Errorf("Error checking account status for %s: %v", account.Title, err)

			errorDescription := "An error occurred while checking this account's status. "
			if strings.Contains(err.Error(), "captcha") {
				errorDescription += "There may be an issue with the captcha service. Please try again in a few minutes or contact support if the problem persists."
			} else {
				errorDescription += "Please try again later. If the problem continues, consider updating your account's SSO cookie."
			}

			embed := &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("%s - Error", account.Title),
				Description: errorDescription,
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

		embed := createStatusEmbed(account.Title, status)
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

func createStatusEmbed(accountTitle string, status models.AccountStatus) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Account Status", accountTitle),
		Description: fmt.Sprintf("Overall Status: %s", status.Overall),
		Color:       services.GetColorForStatus(status.Overall),
		Timestamp:   time.Now().Format(time.RFC3339),
		Fields:      []*discordgo.MessageEmbedField{},
	}

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
			duration := services.FormatBanDuration(gameStatus.DurationSeconds)
			statusDesc = fmt.Sprintf("Temporarily Banned (%s remaining)", duration)
		default:
			statusDesc = "Unknown Status"
		}

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   game,
			Value:  statusDesc,
			Inline: true,
		})
	}

	return embed
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}
