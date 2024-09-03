package accountage

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

func CommandAccountAge(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) error {
	userID := event.User().ID

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user accounts")
		return event.CreateMessage(discord.MessageCreate{
			Content: "Error fetching your accounts. Please try again.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	if len(accounts) == 0 {
		return event.CreateMessage(discord.MessageCreate{
			Content: "You don't have any monitored accounts.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	var components []discord.MessageComponent
	for _, account := range accounts {
		components = append(components, discord.ButtonComponent{
			Label:    account.Title,
			Style:    discord.ButtonStylePrimary,
			CustomID: fmt.Sprintf("account_age_%d", account.ID),
		})
	}

	return event.CreateMessage(discord.MessageCreate{
		Content: "Select an account to check its age:",
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
	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "account_age_"))
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
			Content: "Error: Account not found or you don't have permission to check its age.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	if account.LastStatus.Games == nil {
		account.LastStatus.Games = make(map[string]models.GameStatus)
	}

	if !services.VerifySSOCookie(account.SSOCookie) {
		account.IsExpiredCookie = true
		database.DB.Save(&account)

		return event.CreateMessage(discord.MessageCreate{
			Content: "Invalid SSOCookie. Account's cookie status updated.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	years, months, days, err := services.CheckAccountAge(account.SSOCookie)
	if err != nil {
		logger.Log.WithError(err).Errorf("Error checking account age for account %s", account.Title)
		return event.CreateMessage(discord.MessageCreate{
			Content: "There was an error checking the account age.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	embed := discord.NewEmbedBuilder().
		SetTitle(fmt.Sprintf("%s - Account Age", account.Title)).
		SetDescription(fmt.Sprintf("The account is %d years, %d months, and %d days old.", years, months, days)).
		SetColor(0x00ff00).
		SetTimestamp(time.Now()).
		AddField("Last Status", formatAccountStatus(account.LastStatus), false).
		AddField("Creation Date", time.Now().AddDate(-years, -months, -days).Format("January 2, 2006"), true).
		Build()

	return event.UpdateMessage(discord.MessageUpdate{
		Embeds:     []*discord.Embed{embed},
		Components: []discord.MessageComponent{},
	})
}

func formatAccountStatus(status models.AccountStatus) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Overall: %s\n", status.Overall))

	for game, gameStatus := range status.Games {
		sb.WriteString(fmt.Sprintf("%s: ", game))
		switch gameStatus.Status {
		case models.StatusGood:
			sb.WriteString("Good Standing")
		case models.StatusPermaban:
			sb.WriteString("Permanently Banned")
		case models.StatusShadowban:
			sb.WriteString("Under Review")
		case models.StatusTempban:
			duration := services.FormatBanDuration(gameStatus.DurationSeconds)
			sb.WriteString(fmt.Sprintf("Temporarily Banned (%s remaining)", duration))
		default:
			sb.WriteString("Unknown Status")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
