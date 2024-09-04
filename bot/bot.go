package bot

import (
	"CODStatusBot/command"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	discord      *discordgo.Session
	commandQueue chan *discordgo.InteractionCreate
	workerPool   chan struct{}
	queueMutex   sync.Mutex
)

const (
	maxQueueSize  = 1000
	maxWorkers    = 50
	workerTimeout = 30 * time.Second
)

func StartBot() (*discordgo.Session, error) {
	envToken := os.Getenv("DISCORD_TOKEN")
	if envToken == "" {
		return nil, errors.New("DISCORD_TOKEN environment variable not set")
	}

	var err error
	discord, err = discordgo.New("Bot " + envToken)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create Discord session")
		return nil, err
	}

	err = discord.Open()
	if err != nil {
		logger.Log.WithError(err).Error("Failed to open Discord connection")
		return nil, err
	}

	err = discord.UpdateWatchStatus(0, "the Status of your Accounts so you don't have to.")
	if err != nil {
		logger.Log.WithError(err).Error("Failed to set presence status")
		return nil, err
	}

	// Initialize command queue and worker pool
	initializeCommandQueue()
	initializeWorkerPool()
	discord.AddHandler(handleInteraction)
	go handleCommands()
	go services.CheckAccounts(discord)

	return discord, nil
}

func handleCommands() {
	for {
		select {
		case i := <-commandQueue:
			go processCommand(i)
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}
func initializeCommandQueue() {
	logger.Log.Info("Initializing command queue")
	commandQueue = make(chan *discordgo.InteractionCreate, maxQueueSize)
}

func initializeWorkerPool() {
	logger.Log.Info("Initializing worker pool")
	workerPool = make(chan struct{}, maxWorkers)
	// Start workers
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}
}

func handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	queueMutex.Lock()
	defer queueMutex.Unlock()

	select {
	case commandQueue <- i:
		logger.Log.Debugf("Command queued: %s", i.ApplicationCommandData().Name)
	default:
		logger.Log.Warn("Command queue is full, dropping command")
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "The bot is currently experiencing high load. Please try again later.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
}

func worker() {
	for i := range commandQueue {
		processCommand(i)
	}
}

func processCommand(i *discordgo.InteractionCreate) {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("Panic recovered in processCommand: %v", r)
			sendErrorResponse(i, "An unexpected error occurred. Please try again later.")
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), workerTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handleCommand(i)
	}()

	select {
	case <-ctx.Done():
		logger.Log.Warnf("Command processing timed out: %s", i.ApplicationCommandData().Name)
		sendErrorResponse(i, "The command processing timed out. Please try again later.")
	case <-done:
		// Command processed successfully
	}
}

func handleCommand(i *discordgo.InteractionCreate) {
	var installType models.InstallationType
	if i.GuildID != "" {
		installType = models.InstallTypeGuild
	} else {
		installType = models.InstallTypeUser
	}

	userID := getUserID(i)
	if userID == "" {
		logger.Log.Error("Failed to get user ID from interaction")
		sendErrorResponse(i, "An error occurred while processing your request.")
		return
	}

	err := checkAndSendAnnouncement(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Error checking and sending announcement")
	}

	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		command.HandleCommand(discord, i, installType)
	case discordgo.InteractionModalSubmit:
		handleModalSubmit(i, installType)
	case discordgo.InteractionMessageComponent:
		handleMessageComponent(i, installType)
	}
}

func checkAndSendAnnouncement(userID string) error {
	var userSettings models.UserSettings
	err := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&userSettings).Error
	if err != nil {
		return err
	}

	if !userSettings.HasSeenAnnouncement {
		err = services.SendGlobalAnnouncement(discord, userID)
		if err != nil {
			return err
		}

		userSettings.HasSeenAnnouncement = true
		return database.DB.Save(&userSettings).Error
	}

	return nil
}

func handleModalSubmit(i *discordgo.InteractionCreate, installType models.InstallationType) {
	customID := i.ModalSubmitData().CustomID
	switch {
	case customID == "set_captcha_service_modal":
		command.Handlers["set_captcha_service_modal"](discord, i, installType)
	case customID == "add_account_modal":
		command.Handlers["add_account_modal"](discord, i, installType)
	case strings.HasPrefix(customID, "update_account_modal_"):
		command.Handlers["update_account_modal"](discord, i, installType)
	case customID == "set_check_interval_modal":
		command.Handlers["set_check_interval_modal"](discord, i, installType)
	default:
		logger.Log.WithField("customID", customID).Error("Unknown modal submission")
	}
}

func handleMessageComponent(i *discordgo.InteractionCreate, installType models.InstallationType) {
	customID := i.MessageComponentData().CustomID
	switch {
	case strings.HasPrefix(customID, "account_age_"):
		logger.Log.Info("Handling account age selection")
		command.Handlers["account_age"](discord, i, installType)
	case strings.HasPrefix(customID, "account_logs_"):
		logger.Log.Info("Handling account logs selection")
		command.Handlers["account_logs"](discord, i, installType)
	case customID == "account_logs_select":
		logger.Log.Info("Handling selected account log")
		command.Handlers["account_logs"](discord, i, installType)
	case strings.HasPrefix(customID, "update_account_"):
		logger.Log.Info("Handling update account selection")
		command.Handlers["update_account"](discord, i, installType)
	case customID == "update_account_select":
		logger.Log.Info("Handling update account selection")
		command.Handlers["update_account"](discord, i, installType)
	case strings.HasPrefix(customID, "remove_account_"):
		logger.Log.Info("Handling remove account selection")
		command.Handlers["remove_account"](discord, i, installType)
	case customID == "cancel_remove" || strings.HasPrefix(customID, "confirm_remove_"):
		logger.Log.Info("Handling remove account confirmation")
		command.Handlers["remove_account"](discord, i, installType)
	case strings.HasPrefix(customID, "check_now_"):
		logger.Log.Info("Handling check now selection")
		command.Handlers["check_now"](discord, i, installType)
	case strings.HasPrefix(customID, "toggle_check_"):
		logger.Log.Info("Handling toggle check selection")
		command.Handlers["toggle_check"](discord, i, installType)
	default:
		logger.Log.WithField("customID", customID).Error("Unknown message component interaction")
	}
}

func getUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func sendErrorResponse(i *discordgo.InteractionCreate, message string) {
	err := discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send error response")
	}
}

func init() {
	// Initialize the database connection
	err := database.Databaselogin()
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to initialize database connection")
	}

	// Create or update the UserSettings table
	err = database.DB.AutoMigrate(&models.UserSettings{})
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to create or update UserSettings table")
	}
}
