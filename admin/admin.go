package admin

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

var (
	store = sessions.NewCookieStore([]byte(os.Getenv("SESSION_KEY")))
)

type Stats struct {
	TotalAccounts     int `json:"total_accounts"`
	ActiveAccounts    int `json:"active_accounts"`
	BannedAccounts    int `json:"banned_accounts"`
	TotalUsers        int `json:"total_users"`
	ChecksLastHour    int `json:"checks_last_hour"`
	ChecksLast24Hours int `json:"checks_last_24_hours"`
}

func StartAdminPanel() {
	r := mux.NewRouter()

	r.HandleFunc("/login", loginHandler).Methods("POST")
	r.HandleFunc("/logout", logoutHandler).Methods("POST")
	r.HandleFunc("/stats", authMiddleware(statsHandler)).Methods("GET")

	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "8080"
	}

	logger.Log.Infof("Admin panel starting on port %s", port)
	err := http.ListenAndServe(":"+port, r)
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to start admin panel")
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == os.Getenv("ADMIN_USERNAME") && password == os.Getenv("ADMIN_PASSWORD") {
		session, _ := store.Get(r, "admin-session")
		session.Values["authenticated"] = true
		session.Save(r, w)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Login successful")
	} else {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "Invalid credentials")
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
		logger.Log.WithError(err).Error("Failed to get stats")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
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
