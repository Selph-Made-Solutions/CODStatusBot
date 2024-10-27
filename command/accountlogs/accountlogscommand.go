package accountlogs

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/Discordgo"
	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
)

func CommandAccountLogs(s *Discordgo.Session, i *Discordgo.InteractionCreate) {
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
	var components []Discordgo.MessageComponent
	var currentRow []Discordgo.MessageComponent

	for _, account := range accounts {
		currentRow = append(currentRow, Discordgo.Button{
			Label:    account.Title,
			Style:    Discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("account_logs_%d", account.ID),
		})

		if len(currentRow) == 5 {
			components = append(components, Discordgo.ActionsRow{Components: currentRow})
			currentRow = []Discordgo.MessageComponent{}
		}
	}

	// Create View All Logs button
	if len(currentRow) < 5 {
		currentRow = append(currentRow, Discordgo.Button{
			Label:    "View All Logs",
			Style:    Discordgo.SuccessButton,
			CustomID: "account_logs_all",
		})
	} else {
		components = append(components, Discordgo.ActionsRow{Components: currentRow})
		currentRow = []Discordgo.MessageComponent{
			Discordgo.Button{
				Label:    "View All Logs",
				Style:    Discordgo.SuccessButton,
				CustomID: "account_logs_all",
			},
		}
	}

	// Add the last row
	components = append(components, Discordgo.ActionsRow{Components: currentRow})

	err := s.InteractionRespond(i.Interaction, &Discordgo.InteractionResponse{
		Type: Discordgo.InteractionResponseChannelMessageWithSource,
		Data: &Discordgo.InteractionResponseData{
			Content:    "Select an account to view its logs, or 'View All Logs' to see logs for all accounts:",
			Flags:      Discordgo.MessageFlagsEphemeral,
			Components: components,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding with account selection")
	}
}

func HandleAccountSelection(s *Discordgo.Session, i *Discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	if customID == "account_logs_all" {
		handleAllAccountLogs(s, i)
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

	err = s.InteractionRespond(i.Interaction, &Discordgo.InteractionResponse{
		Type: Discordgo.InteractionResponseUpdateMessage,
		Data: &Discordgo.InteractionResponseData{
			Embeds:     []*Discordgo.MessageEmbed{embed},
			Components: []Discordgo.MessageComponent{},
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction with account logs")
		respondToInteraction(s, i, "Error displaying account logs. Please try again.")
	}
}

func handleAllAccountLogs(s *Discordgo.Session, i *Discordgo.InteractionCreate) {
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

	var embeds []*Discordgo.MessageEmbed
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
			err = s.InteractionRespond(i.Interaction, &Discordgo.InteractionResponse{
				Type: Discordgo.InteractionResponseUpdateMessage,
				Data: &Discordgo.InteractionResponseData{
					Content:    "",
					Embeds:     embeds[j:end],
					Components: []Discordgo.MessageComponent{},
				},
			})
		} else {
			_, err = s.FollowupMessageCreate(i.Interaction, true, &Discordgo.WebhookParams{
				Embeds: embeds[j:end],
				Flags:  Discordgo.MessageFlagsEphemeral,
			})
		}

		if err != nil {
			logger.Log.WithError(err).Error("Error sending account logs")
		}
	}
}

func createAccountLogEmbed(account models.Account) *Discordgo.MessageEmbed {
	var logs []models.Ban
	database.DB.Where("account_id = ?", account.ID).Order("created_at desc").Limit(10).Find(&logs)

	embed := &Discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Account Logs", account.Title),
		Description: "The last 10 status changes for this account",
		Color:       0x00ff00,
		Fields:      make([]*Discordgo.MessageEmbedField, len(logs)),
	}

	for i, log := range logs {
		embed.Fields[i] = &Discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("Status Change %d", i+1),
			Value:  fmt.Sprintf("Status: %s\nTime: %s", log.Status, log.CreatedAt.Format(time.RFC1123)),
			Inline: false,
		}
	}

	if len(logs) == 0 {
		embed.Description = "No status changes logged for this account yet."
	}

	return embed
}

func respondToInteraction(s *Discordgo.Session, i *Discordgo.InteractionCreate, content string) {
	err := s.InteractionRespond(i.Interaction, &Discordgo.InteractionResponse{
		Type: Discordgo.InteractionResponseChannelMessageWithSource,
		Data: &Discordgo.InteractionResponseData{
			Content: content,
			Flags:   Discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}
