package database

import (
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"errors"
	"fmt"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
	DB  *gorm.DB
	dsn string
)

func Databaselogin() error {
	logger.Log.Info("Connecting to database...")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")
	dbVar := os.Getenv("DB_VAR")

	if dbUser == "" || dbPassword == "" || dbHost == "" || dbPort == "" || dbName == "" || dbVar == "" {
		err := errors.New("one or more environment variables for database not set or missing")
		logger.Log.WithError(err).WithField("Bot Startup ", "database variables ").Error()
		return err
	}

	dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s%s", dbUser, dbPassword, dbHost, dbPort, dbName, dbVar)
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		logger.Log.WithError(err).WithField("Bot Startup ", "Mysql Config ").Error()
		return err
	}

	sqlDB, err := DB.DB()
	if err != nil {
		logger.Log.WithError(err).WithField("Bot Startup ", "Get underlying SQL DB ").Error()
		return err
	}

	// SetMaxIdleConns sets the maximum number of connections in the idle connection pool.
	sqlDB.SetMaxIdleConns(10)

	// SetMaxOpenConns sets the maximum number of open connections to the database.
	sqlDB.SetMaxOpenConns(100)

	// SetConnMaxLifetime sets the maximum amount of time a connection may be reused.
	sqlDB.SetConnMaxLifetime(time.Hour)

	err = DB.AutoMigrate(&models.Account{}, &models.Ban{}, &models.UserSettings{})
	if err != nil {
		logger.Log.WithError(err).WithField("Bot Startup ", "Database Models Problem ").Error()
		return err
	}

	return nil
}

func GetDB() *gorm.DB {
	return DB
}
