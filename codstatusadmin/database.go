package codstatusadmin

import (
	"fmt"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func initDatabase() error {
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")
	dbVar := os.Getenv("DB_VAR")

	if dbUser == "" || dbPassword == "" || dbHost == "" || dbPort == "" || dbName == "" || dbVar == "" {
		return fmt.Errorf("one or more environment variables for database not set or missing")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s%s", dbUser, dbPassword, dbHost, dbPort, dbName, dbVar)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	DB = db
	return nil
}
