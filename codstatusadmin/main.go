package codstatusadmin

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func Init() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
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
