package accountlogs

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

func CommandAccountLogs(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
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

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user accounts")
		respondToInteraction(s, i, "Error fetching your accounts. Please try again.")
		return
	}

	if len(accounts) == 0 {
		respondToInteraction(s, i, "You don't have any monitored accounts.")
		return
	}

	// Create buttons for each account
	var components []discordgo.MessageComponent
	for _, account := range accounts {
		components = append(components, discordgo.Button{
			Label:    account.Title,
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("account_logs_%d", account.ID),
		})
	}

	// Add "View All Logs" button
	components = append(components, discordgo.Button{
		Label:    "View All Logs",
		Style:    discordgo.SuccessButton,
		CustomID: "account_logs_all",
	})

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Select an account to view its logs, or 'View All Logs' to see logs for all accounts:",
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

func HandleAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
	customID := i.MessageComponentData().CustomID

	if customID == "account_logs_all" {
		handleAllAccountLogs(s, i, installType)
		return
	}

	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "account_logs_"))
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing account ID")
		respondToInteraction(s, i, "Error processing your selection. Please try again.")
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		respondToInteraction(s, i, "Error: Account not found or you don't have permission to view its logs.")
		return
	}

	embed := createAccountLogEmbed(account)

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{},
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction with account logs")
		respondToInteraction(s, i, "Error displaying account logs. Please try again.")
	}
}

func handleAllAccountLogs(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
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

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user accounts")
		respondToInteraction(s, i, "Error fetching your accounts. Please try again.")
		return
	}

	var embeds []*discordgo.MessageEmbed
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
			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{
					Content:    "",
					Embeds:     embeds[j:end],
					Components: []discordgo.MessageComponent{},
				},
			})
		} else {
			_, err = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Embeds: embeds[j:end],
				Flags:  discordgo.MessageFlagsEphemeral,
			})
		}

		if err != nil {
			logger.Log.WithError(err).Error("Error sending account logs")
		}
	}
}

func createAccountLogEmbed(account models.Account) *discordgo.MessageEmbed {
	var logs []models.Ban
	database.DB.Where("account_id = ?", account.ID).Order("created_at desc").Limit(10).Find(&logs)

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Account Logs", account.Title),
		Description: "The last 10 status changes for this account",
		Color:       services.GetColorForStatus(account.LastStatus.Overall),
		Fields:      make([]*discordgo.MessageEmbedField, 0),
	}

	// Add current status field
	currentStatusField := &discordgo.MessageEmbedField{
		Name:   "Current Status",
		Value:  formatAccountStatus(account.LastStatus),
		Inline: false,
	}
	embed.Fields = append(embed.Fields, currentStatusField)

	// Add log entries
	for i, log := range logs {
		logEntry := &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("Status Change %d", i+1),
			Value:  fmt.Sprintf("Status: %s\nTime: %s", log.Status, log.CreatedAt.Format(time.RFC1123)),
			Inline: false,
		}
		embed.Fields = append(embed.Fields, logEntry)
	}

	if len(logs) == 0 {
		embed.Description = "No status changes logged for this account yet."
	}

	return embed
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
