package database

import (
	"fmt"
	"os"

	"CODStatusBot/errorhandler"
	"CODStatusBot/models"

	"CODStatusBot/logger"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Databaselogin() error {
	logger.Log.Info("Connecting to database...")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")
	dbVar := os.Getenv("DB_VAR")

	if dbUser == "" || dbPassword == "" || dbHost == "" || dbPort == "" || dbName == "" || dbVar == "" {
		return errorhandler.NewValidationError(fmt.Errorf("one or more environment variables for database not set or missing"), "database configuration")
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s%s", dbUser, dbPassword, dbHost, dbPort, dbName, dbVar)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return errorhandler.NewDatabaseError(err, "opening database connection")
	}

	DB = db

	err = DB.AutoMigrate(&models.Account{}, &models.Ban{}, &models.UserSettings{})
	if err != nil {
		return errorhandler.NewDatabaseError(err, "auto-migrating database models")
	}
	return nil
}
