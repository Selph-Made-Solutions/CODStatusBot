package admin

import (
	services "CODStatusBot"
	"CODStatusBot/logger"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"time"

	"CODStatusBot/database"
	"CODStatusBot/models"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

type Stats struct {
	TotalAccounts      int
	ActiveAccounts     int
	BannedAccounts     int
	TotalUsers         int
	ChecksLastHour     int
	ChecksLast24Hours  int
	MarkedAccountCount int
}

var store = sessions.NewCookieStore([]byte(os.Getenv("SESSION_KEY")))

func StartAdminPanel() {
	r := mux.NewRouter()

	r.HandleFunc("/admin/login", loginHandler).Methods("GET", "POST")
	r.HandleFunc("/admin/logout", logoutHandler).Methods("POST")
	r.HandleFunc("/admin/stats", authMiddleware(statsHandler)).Methods("GET")
	r.HandleFunc("/admin", authMiddleware(dashboardHandler)).Methods("GET")
	r.HandleFunc("/admin/marked-accounts", authMiddleware(markedAccountsHandler)).Methods("GET")

	// Serve static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Admin panel starting on port %s\n", port)
	err := http.ListenAndServe(":"+port, r)
	if err != nil {
		fmt.Printf("Failed to start admin panel: %v\n", err)
	}
}
func GetErrorDisabledAccounts() ([]models.Account, error) {
	var accounts []models.Account
	result := database.DB.Where("is_error_disabled = ?", true).Find(&accounts)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching error-disabled accounts")
		return nil, result.Error
	}
	return accounts, nil
}

func markedAccountsHandler(w http.ResponseWriter, r *http.Request) {
	markedAccounts, err := GetErrorDisabledAccounts()
	if err != nil {
		http.Error(w, "Error fetching marked accounts", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(markedAccounts)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// Serve login page
		tmpl, err := template.ParseFiles("templates/login.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, nil)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == os.Getenv("ADMIN_USERNAME") && password == os.Getenv("ADMIN_PASSWORD") {
		session, _ := store.Get(r, "admin-session")
		session.Values["authenticated"] = true
		session.Save(r, w)
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	} else {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
	}
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "admin-session")
	session.Values["authenticated"] = false
	session.Save(r, w)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Logout successful")
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "admin-session")
		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := getStats()
	if err != nil {
		fmt.Printf("Failed to get stats: %v\n", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := getStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl, err := template.ParseFiles("templates/dashboard.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, stats)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func getStats() (Stats, error) {
	var stats Stats
	var err error

	stats.TotalAccounts, err = getTotalAccounts()
	if err != nil {
		return stats, err
	}

	stats.ActiveAccounts, err = getActiveAccounts()
	if err != nil {
		return stats, err
	}

	stats.BannedAccounts, err = getBannedAccounts()
	if err != nil {
		return stats, err
	}

	stats.TotalUsers, err = getTotalUsers()
	if err != nil {
		return stats, err
	}

	stats.ChecksLastHour, err = getChecksInTimeRange(time.Hour)
	if err != nil {
		return stats, err
	}

	stats.ChecksLast24Hours, err = getChecksInTimeRange(24 * time.Hour)
	if err != nil {
		return stats, err
	}

	markedAccounts, err := services.GetErrorDisabledAccounts()
	if err != nil {
		return stats, err
	}

	stats.MarkedAccountCount = len(markedAccounts)
	return stats, nil
}

func getTotalAccounts() (int, error) {
	var count int64
	err := database.DB.Model(&models.Account{}).Count(&count).Error
	return int(count), err
}

func getActiveAccounts() (int, error) {
	var count int64
	err := database.DB.Model(&models.Account{}).Where("is_permabanned = ? AND is_expired_cookie = ?", false, false).Count(&count).Error
	return int(count), err
}

func getBannedAccounts() (int, error) {
	var count int64
	err := database.DB.Model(&models.Account{}).Where("is_permabanned = ?", true).Count(&count).Error
	return int(count), err
}

func getTotalUsers() (int, error) {
	var count int64
	err := database.DB.Model(&models.UserSettings{}).Count(&count).Error
	return int(count), err
}

func getChecksInTimeRange(duration time.Duration) (int, error) {
	var count int64
	timeThreshold := time.Now().Add(-duration).Unix()
	err := database.DB.Model(&models.Account{}).Where("last_check > ?", timeThreshold).Count(&count).Error
	return int(count), err
}
