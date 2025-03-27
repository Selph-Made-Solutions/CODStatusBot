package services

import (
	"time"

	"github.com/bradselph/CODStatusBot/configuration"
	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
)

func CleanupInactiveUsers() {
	cfg := configuration.Get()
	inactivePeriod := time.Now().Add(-cfg.Users.InactiveUserPeriod)

	var inactiveUsers []models.UserSettings
	if err := database.DB.Where("last_guild_interaction < ? AND last_direct_interaction < ?",
		inactivePeriod, inactivePeriod).Find(&inactiveUsers).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to fetch inactive users")
		return
	}

	logger.Log.Infof("Found %d inactive users to archive", len(inactiveUsers))

	for _, user := range inactiveUsers {
		user.IsUnreachable = true
		database.DB.Save(&user)
	}
}
