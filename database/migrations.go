package database

import (
	"time"

	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
)

func CleanupInvalidTimestamps() {
	logger.Log.Info("Running timestamp cleanup migration")

	result := DB.Model(&models.Ban{}).
		Where("timestamp <= '1970-01-01' OR timestamp IS NULL").
		Update("timestamp", time.Now())
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Failed to clean up invalid Ban timestamps")
	} else {
		logger.Log.Infof("Fixed %d invalid Ban timestamps", result.RowsAffected)
	}

	result = DB.Model(&models.Account{}).
		Where("created <= 0 OR created IS NULL").
		Update("created", time.Now().Unix())
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Failed to clean up invalid Account created timestamps")
	} else {
		logger.Log.Infof("Fixed %d invalid Account created timestamps", result.RowsAffected)
	}

	result = DB.Model(&models.Account{}).
		Where("last_check <= 0 OR last_check IS NULL").
		Update("last_check", time.Now().Unix())
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Failed to clean up invalid Account last_check timestamps")
	} else {
		logger.Log.Infof("Fixed %d invalid Account last_check timestamps", result.RowsAffected)
	}
}
