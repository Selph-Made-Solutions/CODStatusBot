package accountlogs

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"strconv"
	"time"
)

func RegisterCommand(s *discordgo.Session, guildID string) {
	command := &discordgo.ApplicationCommand{
		Name:        "accountlogs",
		Description: "View the logs for an account",
	}

	_, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, command)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating accountlogs command")
	}
}

func UnregisterCommand(s *discordgo.Session, guildID string) {
	commands, err := s.ApplicationCommands(s.State.User.ID, guildID)
	if err != nil {
		logger.Log.WithError(err).Error("Error getting application commands")
		return
	}

	for _, command := range commands {
		if command.Name == "accountlogs" {
			err := s.ApplicationCommandDelete(s.State.User.ID, guildID, command.ID)
			if err != nil {
				logger.Log.WithError(err).Error("Error deleting accountlogs command")
			}
			return
		}
	}
}

func CommandAccountLogs(s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	options := make([]discordgo.SelectMenuOption, len(accounts))
	for index, account := range accounts {
		options[index] = discordgo.SelectMenuOption{
			Label:       account.Title,
			Value:       strconv.Itoa(int(account.ID)),
			Description: fmt.Sprintf("Last status: %s, Guild: %s", account.LastStatus, account.GuildID),
		}
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Select an account to view its logs:",
			Flags:   discordgo.MessageFlagsEphemeral,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{
							CustomID:    "account_logs_select",
							Placeholder: "Choose an account",
							Options:     options,
						},
					},
				},
			},
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Error responding with account selection")
	}
}

func HandleAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if len(data.Values) == 0 {
		respondToInteraction(s, i, "No account selected. Please try again.")
		return
	}

	accountID, err := strconv.Atoi(data.Values[0])
	if err != nil {
		logger.Log.WithError(err).Error("Error converting account ID")
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

	var logs []models.Ban
	database.DB.Where("account_id = ?", account.ID).Order("created_at desc").Limit(10).Find(&logs)

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Account Logs", account.Title),
		Description: "The last 10 status changes for this account",
		Color:       0x00ff00,
		Fields:      make([]*discordgo.MessageEmbedField, len(logs)),
	}

	for i, log := range logs {
		embed.Fields[i] = &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("Status Change %d", i+1),
			Value:  fmt.Sprintf("Status: %s\nTime: %s", log.Status, log.CreatedAt.Format(time.RFC1123)),
			Inline: false,
		}
	}

	if len(logs) == 0 {
		embed.Description = "No status changes logged for this account yet."
	}

	respondToInteraction(s, i, "", embed)
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, content string, embeds ...*discordgo.MessageEmbed) {
	var err error
	if i.Type == discordgo.InteractionMessageComponent {
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content: content,
				Embeds:  embeds,
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	} else {
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: content,
				Embeds:  embeds,
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}
