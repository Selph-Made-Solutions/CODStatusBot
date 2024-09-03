package bot

import (
	"CODStatusBot/command"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"context"
	"errors"
	"fmt"
	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"os"
	"time"
)

var client bot.Client
var commandQueue chan *events.ApplicationCommandInteractionCreate
var workerPool chan struct{}

const (
	maxQueueSize  = 1000
	maxWorkers    = 50
	workerTimeout = 30 * time.Second
)

func StartBot() error {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		return errors.New("DISCORD_TOKEN environment variable not set")
	}

	var err error
	client, err = disgo.New(token,
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuildMessages,
				gateway.IntentDirectMessages,
				gateway.IntentMessageContent,
				gateway.IntentGuilds,
			),
		),
		bot.WithCacheConfigOpts(
			cache.WithCaches(cache.FlagGuilds, cache.FlagMembers),
		),
	)
	if err != nil {
		return fmt.Errorf("error creating discord client: %w", err)
	}

	err = client.OpenGateway(context.TODO())
	if err != nil {
		return fmt.Errorf("error connecting to gateway: %w", err)
	}

	err = client.SetPresence(context.TODO(), gateway.WithWatchingActivity("the Status of your Accounts so you don't have to."))
	if err != nil {
		logger.Log.WithError(err).Error("Error setting presence")
	}

	command.RegisterCommands(client)
	logger.Log.Info("Registering global commands")

	// Initialize command queue and worker pool
	commandQueue = make(chan *events.ApplicationCommandInteractionCreate, maxQueueSize)
	workerPool = make(chan struct{}, maxWorkers)

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

	client.AddEventListeners(&events.ListenerAdapter{
		OnApplicationCommandInteraction: func(event *events.ApplicationCommandInteractionCreate) {
			select {
			case commandQueue <- event:
				logger.Log.Debugf("Command queued: %s", event.Data.CommandName())
			default:
				logger.Log.Warn("Command queue is full, dropping command")
				event.CreateMessage(discord.MessageCreate{
					Content: "The bot is currently experiencing high load. Please try again later.",
					Flags:   discord.MessageFlagEphemeral,
				})
			}
		},
	})

	go services.CheckAccounts(client)
	return nil
}

func worker() {
	for event := range commandQueue {
		workerPool <- struct{}{}
		processCommand(event)
		<-workerPool
	}
}

func processCommand(event *events.ApplicationCommandInteractionCreate) {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("Panic recovered in processCommand: %v", r)
		}
	}()

	var installType models.InstallationType
	if event.GuildID.IsValid() {
		installType = models.InstallTypeGuild
	} else {
		installType = models.InstallTypeUser
	}

	ctx, cancel := context.WithTimeout(context.Background(), workerTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)

		command.HandleCommand(client, event, installType)
	}()

	select {
	case <-ctx.Done():
		logger.Log.Warnf("Command processing timed out: %s", event.Data.CommandName())
		event.CreateMessage(discord.MessageCreate{
			Content: "The command processing timed out. Please try again later.",
			Flags:   discord.MessageFlagEphemeral,
		})
	case <-done:
		// Command processed successfully
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
