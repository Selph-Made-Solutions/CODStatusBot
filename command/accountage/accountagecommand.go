package accountage

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"CODStatusBot/database"
	"CODStatusBot/errorhandler"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"

	"github.com/bwmarrin/discordgo"
)

func CommandAccountAge(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, err := getUserID(i)
	if err != nil {
		handleInteractionError(s, i, errorhandler.NewValidationError(err, "user ID"))
		return
	}

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		handleInteractionError(s, i, errorhandler.NewDatabaseError(result.Error, "fetching user accounts"))
		return
	}

	if len(accounts) == 0 {
		respondToInteraction(s, i, "You don't have any monitored accounts.")
		return
	}

	var (
		components []discordgo.MessageComponent
		currentRow []discordgo.MessageComponent
	)

	for _, account := range accounts {
		currentRow = append(currentRow, discordgo.Button{
			Label:    account.Title,
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("account_age_%d", account.ID),
		})

		if len(currentRow) == 5 {
			components = append(components, discordgo.ActionsRow{Components: currentRow})
			currentRow = []discordgo.MessageComponent{}
		}
	}

	if len(currentRow) > 0 {
		components = append(components, discordgo.ActionsRow{Components: currentRow})
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    "Select an account to check its age:",
			Flags:      discordgo.MessageFlagsEphemeral,
			Components: components,
		},
	})
	if err != nil {
		handleInteractionError(s, i, errorhandler.NewAPIError(err, "Discord"))
	}
}

func HandleAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "account_age_"))
	if err != nil {
		handleInteractionError(s, i, errorhandler.NewValidationError(err, "account ID"))
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		handleInteractionError(s, i, errorhandler.NewDatabaseError(result.Error, "fetching account"))
		return
	}

	if !services.VerifySSOCookie(account.SSOCookie) {
		account.IsExpiredCookie = true
		database.DB.Save(&account)
		respondToInteraction(s, i, "The SSO cookie for this account has expired. Please update it using the /updateaccount command.")
		return
	}

	years, months, days, createdEpoch, err := services.CheckAccountAge(account.SSOCookie)
	if err != nil {
		handleInteractionError(s, i, errorhandler.NewAPIError(err, "Activision"))
		return
	}

	// Update the account's Created field
	account.Created = createdEpoch
	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Errorf("Error saving account creation timestamp for account %s", account.Title)
	}

	creationDate := time.Unix(createdEpoch, 0).UTC().Format("January 2, 2006")

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Account Age", account.Title),
		Description: fmt.Sprintf("The account is %d years, %d months, and %d days old.", years, months, days),
		Color:       0x00ff00,
		Timestamp:   time.Now().Format(time.RFC3339),
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Last Status",
				Value:  string(account.LastStatus),
				Inline: true,
			},
			{
				Name:   "Creation Date",
				Value:  creationDate,
				Inline: true,
			},
		},
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{},
		},
	})
	if err != nil {
		handleInteractionError(s, i, errorhandler.NewAPIError(err, "Discord"))
	}
}

func handleInteractionError(s *discordgo.Session, i *discordgo.InteractionCreate, err error) {
	userMsg, _ := errorhandler.HandleError(err)
	respondToInteraction(s, i, userMsg)
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

func getUserID(i *discordgo.InteractionCreate) (string, error) {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID, nil
	}
	if i.User != nil {
		return i.User.ID, nil
	}
	return "", fmt.Errorf("unable to determine user ID")
}
