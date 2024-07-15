package addaccountnew

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"

	"github.com/bwmarrin/discordgo"
)

func RegisterCommand(s *discordgo.Session, guildID string) {
	command := &discordgo.ApplicationCommand{
		Name:        "addaccountnew",
		Description: "Add a new account to monitor using a modal",
	}

	_, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, command)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating addaccountnew command")
	}
}

func UnregisterCommand(s *discordgo.Session, guildID string) {
	commands, err := s.ApplicationCommands(s.State.User.ID, guildID)
	if err != nil {
		logger.Log.WithError(err).Error("Error getting application commands")
		return
	}

	for _, cmd := range commands {
		if cmd.Name == "addaccountnew" {
			err := s.ApplicationCommandDelete(s.State.User.ID, guildID, cmd.ID)
			if err != nil {
				logger.Log.WithError(err).Error("Error deleting addaccountnew command")
			}
			return
		}
	}
}

func CommandAddAccountNew(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "add_account_modal",
			Title:    "Add New Account",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "account_title",
							Label:       "Account Title",
							Style:       discordgo.TextInputShort,
							Placeholder: "Enter a title for this account",
							Required:    true,
							MinLength:   1,
							MaxLength:   100,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "sso_cookie",
							Label:       "SSO Cookie",
							Style:       discordgo.TextInputParagraph,
							Placeholder: "Enter the SSO cookie for this account",
							Required:    true,
							MinLength:   1,
							MaxLength:   4000,
						},
					},
				},
			},
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Error responding with modal")
	}
}

func HandleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	title := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	ssoCookie := data.Components[1].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

	// Verify SSO Cookie
	if !services.VerifySSOCookie(ssoCookie) {
		respondToInteraction(s, i, "Invalid SSO cookie. Please try again with a valid cookie.")
		return
	}

	// Create new account
	account := models.Account{
		UserID:           i.Member.User.ID,
		Title:            title,
		SSOCookie:        ssoCookie,
		GuildID:          i.GuildID,
		ChannelID:        i.ChannelID,
		NotificationType: "channel",
	}

	// Save to database
	result := database.DB.Create(&account)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error creating account")
		respondToInteraction(s, i, "Error creating account. Please try again.")
		return
	}

	respondToInteraction(s, i, "Account added successfully!")
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}
