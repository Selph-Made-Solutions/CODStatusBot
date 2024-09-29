package codstatusadmin

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Load(".env.dashboard"); err != nil {
		log.Fatal("Error loading .env.dashboard file")
	}

	if err := initDatabase(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	go StartStatsCaching()

	http.HandleFunc("/admin", DashboardHandler)
	http.HandleFunc("/admin/stats", StatsHandler)

	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Admin dashboard starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func StartStatsCaching() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		UpdateCachedStats()
		<-ticker.C
	}
}

var DB *gorm.DB

func initDatabase() error {
	dbUser := os.Getenv("DASHBOARD_DB_USER")
	dbPassword := os.Getenv("DASHBOARD_DB_PASSWORD")
	dbHost := os.Getenv("DASHBOARD_DB_HOST")
	dbPort := os.Getenv("DASHBOARD_DB_PORT")
	dbName := os.Getenv("DASHBOARD_DB_NAME")
	dbVar := os.Getenv("DASHBOARD_DB_VAR")

	if dbUser == "" || dbPassword == "" || dbHost == "" || dbPort == "" || dbName == "" || dbVar == "" {
		return fmt.Errorf("one or more environment variables for dashboard database not set or missing")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s%s", dbUser, dbPassword, dbHost, dbPort, dbName, dbVar)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to dashboard database: %w", err)
	}

	DB = db
	return nil
}
