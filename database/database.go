package database

import (
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
	DB *gorm.DB
)

func Connect() error {
	logger.Log.Info("Connecting to database...")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")
	dbVar := os.Getenv("DB_VAR")

	// Log the presence of each environment variable
	logger.Log.Infof("DB_USER set: %v", dbUser != "")
	logger.Log.Infof("DB_PASSWORD set: %v", dbPassword != "")
	logger.Log.Infof("DB_HOST set: %v", dbHost != "")
	logger.Log.Infof("DB_PORT set: %v", dbPort != "")
	logger.Log.Infof("DB_NAME set: %v", dbName != "")
	logger.Log.Infof("DB_VAR set: %v", dbVar != "")

	if dbUser == "" || dbPassword == "" || dbHost == "" || dbPort == "" || dbName == "" || dbVar == "" {
		return fmt.Errorf("one or more environment variables for database not set or missing")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s%s", dbUser, dbPassword, dbHost, dbPort, dbName, dbVar)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	DB = db

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	// Set connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	err = DB.AutoMigrate(&models.Account{}, &models.Ban{}, &models.UserSettings{})
	if err != nil {
		return fmt.Errorf("failed to auto-migrate database models: %w", err)
	}

	go monitorDatabaseHealth()

	return nil
}

func monitorDatabaseHealth() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		sqlDB, err := DB.DB()
		if err != nil {
			logger.Log.WithError(err).Error("Failed to get database instance for health check")
			continue
		}

		err = sqlDB.Ping()
		if err != nil {
			logger.Log.WithError(err).Error("Database health check failed")
		} else {
			logger.Log.Info("Database health check passed")
		}

		stats := sqlDB.Stats()
		logger.Log.Infof("DB Stats - Open connections: %d, In use: %d, Idle: %d", stats.OpenConnections, stats.InUse, stats.Idle)
	}
}

func GetDB() *gorm.DB {
	return DB
}
