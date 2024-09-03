package checknow

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"strconv"
	"strings"
	"time"
)

func CommandCheckNow(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) error {
	userID := event.User().ID

	userSettings, err := services.GetUserSettings(userID.String(), installType)
	if err != nil {
		logger.Log.WithError(err).Error("Error fetching user settings")
		return event.CreateMessage(discord.MessageCreate{
			Content: "Error fetching user settings. Please try again later.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	logger.Log.Infof("User %s initiated a check. API Key set: %v", userID, userSettings.CaptchaAPIKey != "")

	if userSettings.CaptchaAPIKey == "" {
		if !services.CheckDefaultKeyRateLimit(userID.String()) {
			return event.CreateMessage(discord.MessageCreate{
				Content: "You're using the default API key too frequently. Please wait before trying again, or set up your own API key for unlimited use.",
				Flags:   discord.MessageFlagEphemeral,
			})
		}
	}

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)

	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching accounts")
		return event.CreateMessage(discord.MessageCreate{
			Content: "Error fetching accounts. Please try again later.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	if len(accounts) == 0 {
		return event.CreateMessage(discord.MessageCreate{
			Content: "You don't have any monitored accounts.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	return showAccountButtons(event, accounts)
}

func showAccountButtons(event *events.ApplicationCommandInteractionCreate, accounts []models.Account) error {
	var components []discord.MessageComponent
	for _, account := range accounts {
		components = append(components, discord.ButtonComponent{
			Label:    account.Title,
			Style:    discord.ButtonStylePrimary,
			CustomID: fmt.Sprintf("check_now_%d", account.ID),
		})
	}

	components = append(components, discord.ButtonComponent{
		Label:    "Check All",
		Style:    discord.ButtonStyleSuccess,
		CustomID: "check_now_all",
	})

	return event.CreateMessage(discord.MessageCreate{
		Content: "Select an account to check, or 'Check All' to check all accounts:",
		Flags:   discord.MessageFlagEphemeral,
		Components: []discord.MessageComponent{
			discord.ActionRowComponent{
				Components: components,
			},
		},
	})
}

func HandleAccountSelection(client bot.Client, event *events.ComponentInteractionCreate) error {
	customID := event.Data.CustomID()

	if customID == "check_now_all" {
		var accounts []models.Account
		result := database.DB.Where("user_id = ?", event.User().ID).Find(&accounts)
		if result.Error != nil {
			logger.Log.WithError(result.Error).Error("Error fetching accounts")
			return event.CreateMessage(discord.MessageCreate{
				Content: "Error fetching accounts. Please try again later.",
				Flags:   discord.MessageFlagEphemeral,
			})
		}
		return checkAccounts(client, event, accounts)
	}

	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "check_now_"))
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing account ID")
		return event.CreateMessage(discord.MessageCreate{
			Content: "Error processing your selection. Please try again.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		return event.CreateMessage(discord.MessageCreate{
			Content: "Error: Account not found or you don't have permission to check it.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	return checkAccounts(client, event, []models.Account{account})
}

func checkAccounts(client bot.Client, event *events.ComponentInteractionCreate, accounts []models.Account) error {
	err := event.DeferResponse()
	if err != nil {
		logger.Log.WithError(err).Error("Failed to defer interaction response")
		return err
	}

	var embeds []*discord.Embed

	for _, account := range accounts {
		logger.Log.Infof("Checking account %s for user %s", account.Title, account.UserID)

		if account.IsExpiredCookie {
			embed := discord.NewEmbedBuilder().
				SetTitle(fmt.Sprintf("%s - Expired Cookie", account.Title)).
				SetDescription("The SSO cookie for this account has expired. Please update it using the /updateaccount command.").
				SetColor(0xFF0000).
				Build()
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

			embed := discord.NewEmbedBuilder().
				SetTitle(fmt.Sprintf("%s - Error", account.Title)).
				SetDescription(errorDescription).
				SetColor(0xFF0000).
				Build()
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
		_, err := event.CreateFollowupMessage(discord.MessageCreate{
			Embeds: embeds[j:end],
			Flags:  discord.MessageFlagEphemeral,
		})
		if err != nil {
			logger.Log.WithError(err).Error("Failed to send follow-up message")
		}
	}

	return nil
}

func createStatusEmbed(accountTitle string, status models.AccountStatus) *discord.Embed {
	embedBuilder := discord.NewEmbedBuilder().
		SetTitle(fmt.Sprintf("%s - Account Status", accountTitle)).
		SetDescription(fmt.Sprintf("Overall Status: %s", status.Overall)).
		SetColor(services.GetColorForStatus(status.Overall)).
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
			duration := services.FormatBanDuration(gameStatus.DurationSeconds)
			statusDesc = fmt.Sprintf("Temporarily Banned (%s remaining)", duration)
		default:
			statusDesc = "Unknown Status"
		}

		embedBuilder.AddField(game, statusDesc, true)
	}

	return embedBuilder.Build()
}
