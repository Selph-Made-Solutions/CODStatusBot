package accountlogs

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

func CommandAccountLogs(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) error {
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
			CustomID: fmt.Sprintf("account_logs_%d", account.ID),
		})
	}

	components = append(components, discord.ButtonComponent{
		Label:    "View All Logs",
		Style:    discord.ButtonStyleSuccess,
		CustomID: "account_logs_all",
	})

	return event.CreateMessage(discord.MessageCreate{
		Content: "Select an account to view its logs, or 'View All Logs' to see logs for all accounts:",
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

	if customID == "account_logs_all" {
		return handleAllAccountLogs(client, event)
	}

	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "account_logs_"))
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
			Content: "Error: Account not found or you don't have permission to view its logs.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	embed := createAccountLogEmbed(account)

	return event.UpdateMessage(discord.MessageUpdate{
		Embeds:     []*discord.Embed{embed},
		Components: []discord.MessageComponent{},
	})
}

func handleAllAccountLogs(client bot.Client, event *events.ComponentInteractionCreate) error {
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

	var embeds []*discord.Embed
	for _, account := range accounts {
		embed := createAccountLogEmbed(account)
		embeds = append(embeds, embed)
	}

	// Send embeds in batches of 10 (Discord's limit)
	for j := 0; j < len(embeds); j += 10 {
		end := j + 10
		if end > len(embeds) {
			end = len(embeds)
		}

		var err error
		if j == 0 {
			err = event.UpdateMessage(discord.MessageUpdate{
				Content:    "",
				Embeds:     embeds[j:end],
				Components: []discord.MessageComponent{},
			})
		} else {
			_, err = event.CreateFollowupMessage(discord.MessageCreate{
				Embeds: embeds[j:end],
				Flags:  discord.MessageFlagEphemeral,
			})
		}

		if err != nil {
			logger.Log.WithError(err).Error("Error sending account logs")
		}
	}

	return nil
}

func createAccountLogEmbed(account models.Account) *discord.Embed {
	var logs []models.Ban
	database.DB.Where("account_id = ?", account.ID).Order("created_at desc").Limit(10).Find(&logs)

	embedBuilder := discord.NewEmbedBuilder().
		SetTitle(fmt.Sprintf("%s - Account Logs", account.Title)).
		SetDescription("The last 10 status changes for this account").
		SetColor(services.GetColorForStatus(account.LastStatus.Overall))

	currentStatusField := discord.EmbedField{
		Name:   "Current Status",
		Value:  formatAccountStatus(account.LastStatus),
		Inline: false,
	}
	embedBuilder.AddField(currentStatusField)

	for i, log := range logs {
		logEntry := discord.EmbedField{
			Name:   fmt.Sprintf("Status Change %d", i+1),
			Value:  fmt.Sprintf("Status: %s\nTime: %s", log.Status, log.CreatedAt.Format(time.RFC1123)),
			Inline: false,
		}
		embedBuilder.AddField(logEntry)
	}

	if len(logs) == 0 {
		embedBuilder.SetDescription("No status changes logged for this account yet.")
	}

	return embedBuilder.Build()
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
