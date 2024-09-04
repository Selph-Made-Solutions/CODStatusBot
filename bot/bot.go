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
	discord  *discordgo.Session
	cmdQueue = make(chan *discordgo.InteractionCreate, 100)
	wg       sync.WaitGroup
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

	// Start command processing workers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go commandProcessor()
	}

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
		cmdQueue <- i
	case discordgo.InteractionModalSubmit:
		handleModalSubmit(s, i)
	case discordgo.InteractionMessageComponent:
		handleMessageComponent(s, i)
	}
}

func commandProcessor() {
	defer wg.Done()

	for i := range cmdQueue {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Log.Errorf("Recovered from panic in command processor: %v\nStack trace:\n%s", r, debug.Stack())
					respondToInteraction(discord, i, "An internal error occurred. Please try again later.")
				}
			}()

			command.HandleCommand(discord, i)
		}()
	}
}

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.ModalSubmitData().CustomID
	switch customID {
	case "set_captcha_service_modal":
		command.Handlers["setcaptchaservice"](s, i)
	case "add_account_modal":
		command.Handlers["addaccount"](s, i)
	default:
		if command.Handlers[customID] != nil {
			command.Handlers[customID](s, i)
		} else {
			logger.Log.WithField("customID", customID).Error("Unknown modal submission")
		}
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

func init() {
	err := services.InitializeServices()
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to initialize services")
	}
}
