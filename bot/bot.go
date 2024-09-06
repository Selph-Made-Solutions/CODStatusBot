package bot

import (
	"CODStatusBot/command"
	"CODStatusBot/logger"
	"CODStatusBot/services"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
	"runtime/debug"
	"sync"
)

var (
	discord *discordgo.Session
	wg      sync.WaitGroup
)

func StartBot() (*discordgo.Session, error) {
	logger.Log.Info("Starting bot initialization")
	envToken := os.Getenv("DISCORD_TOKEN")
	if envToken == "" {
		return nil, fmt.Errorf("DISCORD_TOKEN environment variable not set")
	}

	var err error
	discord, err = discordgo.New("Bot " + envToken)
	if err != nil {
		return nil, fmt.Errorf("error creating Discord session: %w", err)
	}

	err = discord.Open()
	if err != nil {
		return nil, fmt.Errorf("error opening Discord session: %w", err)
	}

	err = discord.UpdateWatchStatus(0, "monitoring account status")
	if err != nil {
		logger.Log.WithError(err).Error("Failed to set presence status")
	}

	command.RegisterCommands(discord)
	logger.Log.Info("Registered global commands")

	discord.AddHandler(handleInteraction)

	// Start the account checking service
	go services.CheckAccounts(discord)

	return discord, nil
}

func handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("Recovered from panic in interaction handler: %v\nStack trace:\n%s", r, debug.Stack())
			respondToInteraction(s, i, "An internal error occurred. Please try again later.")
		}
	}()

	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		handleCommand(s, i)
	case discordgo.InteractionModalSubmit:
		handleModalSubmit(s, i)
	case discordgo.InteractionMessageComponent:
		handleMessageComponent(s, i)
	}
}

func handleCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	commandName := i.ApplicationCommandData().Name
	if handler, ok := command.Handlers[commandName]; ok {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Log.Errorf("Recovered from panic in command handler: %v\nStack trace:\n%s", r, debug.Stack())
					respondToInteraction(s, i, "An internal error occurred. Please try again later.")
				}
			}()
			handler(s, i)
		}()
	} else {
		logger.Log.Errorf("No handler found for command: %s", commandName)
		respondToInteraction(s, i, "Unknown command. Please try again.")
	}
}

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.ModalSubmitData().CustomID
	if handler, ok := command.Handlers[customID]; ok {
		handler(s, i)
	} else {
		logger.Log.WithField("customID", customID).Error("Unknown modal submission")
		respondToInteraction(s, i, "An error occurred processing your input. Please try again.")
	}
}

func handleMessageComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	for prefix, handler := range command.Handlers {
		if customID == prefix || (len(customID) > len(prefix) && customID[:len(prefix)] == prefix) {
			handler(s, i)
			return
		}
	}
	logger.Log.WithField("customID", customID).Error("Unknown message component interaction")
	respondToInteraction(s, i, "An error occurred processing your input. Please try again.")
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
