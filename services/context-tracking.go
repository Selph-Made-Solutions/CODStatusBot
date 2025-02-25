package services

import (
	"time"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
)

func TrackUserInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	userID, err := GetUserID(i)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get user ID for context tracking")
		return err
	}

	context := GetInstallContext(i)

	var userSettings models.UserSettings
	result := database.DB.Where("user_id = ?", userID).FirstOrCreate(&userSettings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting/creating user settings for context tracking")
		return result.Error
	}

	var guildID string
	if context == ServerContext {
		guildID = i.GuildID
	}

	userSettings.UpdateInteractionContext(guildID)

	if userSettings.InstallationType == "server" {
		logger.Log.Infof("User %s interacting in %s context (installed in server: %s)",
			userID, string(context), userSettings.InstallationGuildID)
	} else {
		logger.Log.Infof("User %s interacting in %s context (direct installation)",
			userID, string(context))
	}

	if err := database.DB.Save(&userSettings).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to save user settings after context tracking update")
		return err
	}

	return nil
}

func GetUserInstallationStats() (serverCount int64, directCount int64, err error) {
	if err = database.DB.Model(&models.UserSettings{}).
		Where("installation_type = ?", "server").
		Count(&serverCount).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to count server installations")
		return
	}

	if err = database.DB.Model(&models.UserSettings{}).
		Where("installation_type = ?", "direct").
		Count(&directCount).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to count direct installations")
		return
	}

	return
}

func LogInstallationStats(s *discordgo.Session) {
	guildCount := len(s.State.Guilds)

	serverUsers, directUsers, err := GetUserInstallationStats()
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get installation statistics")
		return
	}

	var totalUsers int64
	if err := database.DB.Model(&models.UserSettings{}).Count(&totalUsers).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to count total users")
		return
	}

	var activeUsers int64
	oneWeekAgo := time.Now().Add(-7 * 24 * time.Hour)
	if err := database.DB.Model(&models.UserSettings{}).
		Where("last_guild_interaction > ? OR last_direct_interaction > ?", oneWeekAgo, oneWeekAgo).
		Count(&activeUsers).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to count active users")
		return
	}

	logger.Log.Infof("Installation Stats: Bot is in %d servers | %d total users (%d active in last 7 days)",
		guildCount, totalUsers, activeUsers)
	logger.Log.Infof("Usage Context: %d users installed in servers | %d users use direct installation",
		serverUsers, directUsers)
}
