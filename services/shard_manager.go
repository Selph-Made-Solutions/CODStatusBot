package services

import (
	"fmt"
	"sync"
	"time"

	"github.com/bradselph/CODStatusBot/configuration"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bwmarrin/discordgo"
)

// ShardManager handles the creation and management of Discord gateway shards
type ShardManager struct {
	sync.Mutex
	Sessions       []*discordgo.Session
	MaxConcurrency int
	TotalShards    int
	StartedShards  int
}

// NewShardManager creates a new shard manager
func NewShardManager() (*ShardManager, error) {
	cfg := configuration.Get()

	// Get gateway info to determine shard count
	gatewayInfo, err := GetGatewayBotInfo(cfg.Discord.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway bot info: %w", err)
	}

	logger.Log.Infof("Creating shard manager with %d shards and max concurrency %d",
		gatewayInfo.Shards, gatewayInfo.SessionStartLimit.MaxConcurrency)

	return &ShardManager{
		Sessions:       make([]*discordgo.Session, gatewayInfo.Shards),
		MaxConcurrency: gatewayInfo.SessionStartLimit.MaxConcurrency,
		TotalShards:    gatewayInfo.Shards,
		StartedShards:  0,
	}, nil
}

// GetGatewayBotInfo retrieves the gateway bot info from Discord
func GetGatewayBotInfo(token string) (*discordgo.GatewayBotResponse, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	return s.GatewayBot()
}

// StartShards initializes and connects all shards
func (sm *ShardManager) StartShards(token string, handlers map[string]func(*discordgo.Session, *discordgo.InteractionCreate)) error {
	sm.Lock()
	defer sm.Unlock()

	// Track which rate limit buckets we've opened
	buckets := make(map[int]bool)

	for i := 0; i < sm.TotalShards; i++ {
		// Calculate the rate limit bucket for this shard
		bucket := i % sm.MaxConcurrency

		// If this is a new bucket, we need to wait for all shards in the previous bucket to start
		if i >= sm.MaxConcurrency && !buckets[bucket] {
			logger.Log.Infof("Waiting for previous bucket to complete before starting shard %d (bucket %d)", i, bucket)
			time.Sleep(5 * time.Second) // Discord rate limiting between buckets
		}

		// Mark this bucket as opened
		buckets[bucket] = true

		// Create and start the shard
		logger.Log.Infof("Starting shard %d of %d", i, sm.TotalShards)
		session, err := discordgo.New("Bot " + token)
		if err != nil {
			return fmt.Errorf("error creating session for shard %d: %w", i, err)
		}

		// Set the shard ID
		session.ShardID = i
		session.ShardCount = sm.TotalShards

		// Register handlers
		registerHandlers(session, handlers)

		// Open connection to Discord
		err = session.Open()
		if err != nil {
			return fmt.Errorf("error opening session for shard %d: %w", i, err)
		}

		sm.Sessions[i] = session
		sm.StartedShards++

		logger.Log.Infof("Shard %d started successfully", i)

		// Wait between shards in the same bucket to avoid hitting rate limits
		if sm.MaxConcurrency > 1 {
			time.Sleep(1 * time.Second)
		}
	}

	logger.Log.Infof("All %d shards started successfully", sm.TotalShards)
	return nil
}

// Close closes all shard connections
func (sm *ShardManager) Close() {
	sm.Lock()
	defer sm.Unlock()

	for i, s := range sm.Sessions {
		if s != nil {
			logger.Log.Infof("Closing shard %d", i)
			s.Close()
		}
	}
}

// GetSession returns the session for a specific guild
func (sm *ShardManager) GetSession(guildID string) *discordgo.Session {
	if len(sm.Sessions) == 1 {
		return sm.Sessions[0]
	}

	// Calculate which shard this guild belongs to
	shardID := getShardIDForGuild(guildID, sm.TotalShards)

	if shardID >= 0 && shardID < len(sm.Sessions) {
		return sm.Sessions[shardID]
	}

	// Default to first shard (for DMs and other non-guild interactions)
	return sm.Sessions[0]
}

// registerHandlers registers command handlers for a session
func registerHandlers(s *discordgo.Session, handlers map[string]func(*discordgo.Session, *discordgo.InteractionCreate)) {
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if h, ok := handlers[i.ApplicationCommandData().Name]; ok {
				h(s, i)
			}
		case discordgo.InteractionModalSubmit:
			if h, ok := handlers[i.ModalSubmitData().CustomID]; ok {
				h(s, i)
			}
		case discordgo.InteractionMessageComponent:
			if h, ok := handlers[i.MessageComponentData().CustomID]; ok {
				h(s, i)
			}
		}
	})
}

// getShardIDForGuild calculates the shard ID for a guild using Discord's sharding formula
func getShardIDForGuild(guildID string, shardCount int) int {
	// If no guild ID (e.g., for DMs), return shard 0
	if guildID == "" {
		return 0
	}

	// Parse guild ID to uint64
	var id uint64
	fmt.Sscanf(guildID, "%d", &id)

	// Use Discord's sharding formula: (guild_id >> 22) % num_shards
	return int((id >> 22) % uint64(shardCount))
}
