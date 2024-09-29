package codstatusadmin

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	if err := initDatabase(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	go startStatsCaching()

	http.HandleFunc("/admin", dashboardHandler)
	http.HandleFunc("/admin/stats", statsHandler)

	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Admin dashboard starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func startStatsCaching() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		updateCachedStats()
		<-ticker.C
	}
}
