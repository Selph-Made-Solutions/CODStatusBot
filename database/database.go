package database

import (
	"errors"
	"fmt"

	"github.com/bradselph/CODStatusBot/configuration"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Databaselogin() error {
	logger.Log.Info("Connecting to database...")

	cfg := configuration.Get()
	dbConfig := cfg.Database

	if dbConfig.User == "" || dbConfig.Password == "" || dbConfig.Host == "" ||
		dbConfig.Port == "" || dbConfig.Name == "" || dbConfig.Var == "" {
		err := errors.New("one or more database configuration values not set")
		logger.Log.WithError(err).WithField("Bot Startup ", "database configuration ").Error()
		return err
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s%s",
		dbConfig.User,
		dbConfig.Password,
		dbConfig.Host,
		dbConfig.Port,
		dbConfig.Name,
		dbConfig.Var)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		logger.Log.WithError(err).WithField("Bot Startup ", "MySQL Config ").Error()
		return err
	}

	DB = db

	// Configure connection pool
	sqlDB, err := DB.DB()
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get database instance")
		return err
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	err = DB.AutoMigrate(&models.Account{}, &models.Ban{}, &models.UserSettings{}, &models.SuppressedNotification{})
	if err != nil {
		logger.Log.WithError(err).WithField("Bot Startup ", "Database Models Problem ").Error()
		return err
	}
	return nil
}

func CloseConnection() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		if err := sqlDB.Close(); err != nil {
			return err
		}
		logger.Log.Info("Database connection closed successfully")
	}
	return nil
}
