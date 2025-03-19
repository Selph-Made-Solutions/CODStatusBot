package verdansk

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bradselph/CODStatusBot/configuration"
	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bradselph/CODStatusBot/services"
	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
)

var X_APIKey string

type PlayerPreferences struct {
	Visible bool `json:"visible"`
}

type StatValue struct {
	OrderValue  *int   `json:"order_value"`
	StringValue string `json:"string_value"`
}

type ImageDownload struct {
	Name string
	URL  string
	Data []byte
	Err  error
}

var (
	tempZipFiles = struct {
		sync.RWMutex
		files map[string]time.Time
	}{
		files: make(map[string]time.Time),
	}
)

func CommandVerdansk(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cfg := configuration.Get()
	X_APIKey = cfg.Verdansk.APIKey

	log := logger.Log.WithFields(logrus.Fields{
		"command": "verdansk",
		"action":  "start",
	})

	log.Info("Verdansk command initiated")

	userID, err := services.GetUserID(i)
	if err != nil {
		log.WithError(err).Error("Failed to get user ID")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	log = log.WithField("userID", userID)

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		log.WithError(result.Error).Error("Error fetching user accounts")
		respondToInteraction(s, i, "Error fetching your accounts. Please try again.")
		return
	}

	log.WithField("accountCount", len(accounts)).Info("Found user accounts")

	components := []discordgo.MessageComponent{
		discordgo.Button{
			Label:    "Provide Activision ID",
			Style:    discordgo.PrimaryButton,
			CustomID: "verdansk_provide_id",
		},
	}

	if len(accounts) > 0 {
		components = append(components, discordgo.Button{
			Label:    "Select Account",
			Style:    discordgo.SuccessButton,
			CustomID: "verdansk_select_account",
		})
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "How would you like to check Verdansk Replay stats?",
			Flags:   discordgo.MessageFlagsEphemeral,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: components,
				},
			},
		},
	})
	if err != nil {
		log.WithError(err).Error("Error responding with method selection")
	} else {
		log.Info("Successfully displayed method selection options")
	}

	services.LogCommandExecution("verdansk", userID, i.GuildID, err == nil, time.Now().UnixMilli(), "")
}

func HandleMethodSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	userID, _ := services.GetUserID(i)
	log := logger.Log.WithFields(logrus.Fields{
		"command":  "verdansk",
		"action":   "method_selection",
		"userID":   userID,
		"customID": customID,
	})

	log.Info("Processing method selection")

	switch customID {
	case "verdansk_provide_id":
		log.Info("User chose to provide Activision ID")
		showActivisionIDModal(s, i)
	case "verdansk_select_account":
		log.Info("User chose to select an account")
		showAccountSelection(s, i)
	default:
		log.Warn("Invalid selection")
		respondToInteraction(s, i, "Invalid selection.")
	}
}

func showActivisionIDModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := services.GetUserID(i)
	log := logger.Log.WithFields(logrus.Fields{
		"command": "verdansk",
		"action":  "show_activision_id_modal",
		"userID":  userID,
	})

	log.Info("Showing Activision ID entry modal")

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "verdansk_activision_id_modal",
			Title:    "Enter Activision ID",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "activision_id",
							Label:       "Activision ID (e.g. Username#1234)",
							Style:       discordgo.TextInputShort,
							Placeholder: "Enter Activision ID",
							Required:    true,
							MinLength:   3,
							MaxLength:   32,
						},
					},
				},
			},
		},
	})
	if err != nil {
		log.WithError(err).Error("Error showing Activision ID modal")
	} else {
		log.Info("Successfully displayed Activision ID modal")
	}
}

func showAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, err := services.GetUserID(i)
	log := logger.Log.WithFields(logrus.Fields{
		"command": "verdansk",
		"action":  "show_account_selection",
		"userID":  userID,
	})

	log.Info("Preparing account selection")

	if err != nil {
		log.WithError(err).Error("Failed to get user ID")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		log.WithError(result.Error).Error("Error fetching user accounts")
		respondToInteraction(s, i, "Error fetching your accounts. Please try again.")
		return
	}

	if len(accounts) == 0 {
		log.Warn("No accounts found")
		respondToInteraction(s, i, "You don't have any monitored accounts.")
		return
	}

	log.WithField("accountCount", len(accounts)).Info("Found accounts for selection")

	var (
		components []discordgo.MessageComponent
		currentRow []discordgo.MessageComponent
	)

	var ogVerdanskAccounts int
	for _, account := range accounts {
		buttonStyle := discordgo.PrimaryButton
		if account.IsOGVerdansk {
			buttonStyle = discordgo.SuccessButton
			ogVerdanskAccounts++
		}

		currentRow = append(currentRow, discordgo.Button{
			Label:    account.Title,
			Style:    buttonStyle,
			CustomID: fmt.Sprintf("verdansk_account_%d", account.ID),
		})

		if len(currentRow) == 5 {
			components = append(components, discordgo.ActionsRow{Components: currentRow})
			currentRow = []discordgo.MessageComponent{}
		}
	}

	if len(currentRow) > 0 {
		components = append(components, discordgo.ActionsRow{Components: currentRow})
	}

	log.WithField("ogVerdanskAccounts", ogVerdanskAccounts).Info("Displaying account selection buttons")

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    "Select an account to check Verdansk Replay stats:\n(Green buttons indicate accounts with confirmed Verdansk stats)",
			Flags:      discordgo.MessageFlagsEphemeral,
			Components: components,
		},
	})
	if err != nil {
		log.WithError(err).Error("Error responding with account selection")
	} else {
		log.Info("Successfully displayed account selection options")
	}
}

func HandleAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	userID, _ := services.GetUserID(i)

	log := logger.Log.WithFields(logrus.Fields{
		"command":  "verdansk",
		"action":   "handle_account_selection",
		"userID":   userID,
		"customID": customID,
	})

	log.Info("Processing account selection")

	if !strings.HasPrefix(customID, "verdansk_account_") {
		log.Warn("Invalid custom ID format")
		return
	}

	accountID := strings.TrimPrefix(customID, "verdansk_account_")
	log = log.WithField("accountID", accountID)

	var account models.Account
	if err := database.DB.First(&account, accountID).Error; err != nil {
		log.WithError(err).Error("Error fetching account")
		respondToInteraction(s, i, "Error: Account not found or you don't have permission to check its Verdansk stats.")
		return
	}

	log.WithField("accountTitle", account.Title).Info("Account found, deferring response")

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.WithError(err).Error("Error sending deferred response")
		return
	}

	log.Info("Retrieving Activision ID from account")
	activisionID, err := getActivisionIDFromAccount(account)
	if err != nil {
		log.WithError(err).Error("Error getting Activision ID from account")
		sendFollowupMessage(s, i, "Error: Could not determine Activision ID from account. Please ensure your cookie is valid.")
		return
	}

	log.WithField("activisionID", activisionID).Info("Successfully retrieved Activision ID, processing stats")
	processVerdanskStats(s, i, activisionID, &account)
}

func HandleActivisionIDModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	userID, _ := services.GetUserID(i)

	log := logger.Log.WithFields(logrus.Fields{
		"command": "verdansk",
		"action":  "handle_activision_id_modal",
		"userID":  userID,
	})

	log.Info("Processing Activision ID modal submission")

	var activisionID string
	for _, comp := range data.Components {
		if row, ok := comp.(*discordgo.ActionsRow); ok {
			for _, rowComp := range row.Components {
				if textInput, ok := rowComp.(*discordgo.TextInput); ok && textInput.CustomID == "activision_id" {
					activisionID = strings.TrimSpace(textInput.Value)
				}
			}
		}
	}

	log = log.WithField("activisionID", activisionID)

	if activisionID == "" {
		log.Warn("Empty Activision ID provided")
		respondToInteraction(s, i, "Error: Activision ID is required.")
		return
	}

	log.Info("Deferring response")
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.WithError(err).Error("Error sending deferred response")
		return
	}

	log.Info("Processing Verdansk stats for provided ID")
	processVerdanskStats(s, i, activisionID, nil)
}

func getActivisionIDFromAccount(account models.Account) (string, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"function":     "getActivisionIDFromAccount",
		"accountID":    account.ID,
		"accountTitle": account.Title,
	})

	log.Info("Retrieving Activision ID from account")

	cfg := configuration.Get()

	if !services.VerifySSOCookie(account.SSOCookie) {
		log.Error("Invalid SSO cookie")
		return "", fmt.Errorf("invalid SSO cookie")
	}

	client := services.GetDefaultHTTPClient()

	req, err := http.NewRequest("GET", cfg.API.ProfileEndpoint, nil)
	if err != nil {
		log.WithError(err).Error("Error creating request")
		return "", fmt.Errorf("error creating request: %w", err)
	}

	headers := services.GenerateHeaders(account.SSOCookie)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	log.Debug("Sending profile request")
	resp, err := client.Do(req)
	if err != nil {
		log.WithError(err).Error("Error sending request")
		return "", fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.WithField("statusCode", resp.StatusCode).Error("API returned non-OK status")
		return "", fmt.Errorf("API returned status code %d", resp.StatusCode)
	}

	var profileData struct {
		Username string `json:"username"`
		Accounts []struct {
			Username string `json:"username"`
			Provider string `json:"provider"`
		} `json:"accounts"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&profileData); err != nil {
		log.WithError(err).Error("Error decoding response")
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	for _, acc := range profileData.Accounts {
		if acc.Provider == "uno" {
			log.WithField("activisionID", acc.Username).Info("Found Activision ID (UNO provider)")
			return acc.Username, nil
		}
	}

	if profileData.Username != "" {
		log.WithField("username", profileData.Username).Info("Using main username as Activision ID")
		return profileData.Username, nil
	}

	log.Error("No Activision ID found in profile")
	return "", fmt.Errorf("no Activision ID found")
}

func processVerdanskStats(s *discordgo.Session, i *discordgo.InteractionCreate, activisionID string, account *models.Account) {
	userID, _ := services.GetUserID(i)
	log := logger.Log.WithFields(logrus.Fields{
		"function":     "processVerdanskStats",
		"userID":       userID,
		"activisionID": activisionID,
	})

	log.Info("Processing Verdansk stats")

	cfg := configuration.Get()
	tempDir := cfg.Verdansk.TempDir
	if tempDir == "" {
		tempDir = "verdansk_temp"
	}

	cleanupTime := cfg.Verdansk.CleanupTime
	if cleanupTime == 0 {
		cleanupTime = 30 * time.Minute
	}

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.WithError(err).Error("Error creating temp directory")
		sendFollowupMessage(s, i, "Error: Failed to create temporary directory for stats.")
		return
	}

	encodedID := strings.Replace(activisionID, "#", "%23", -1)
	log.WithField("encodedID", encodedID).Debug("Encoded Activision ID")

	client := services.GetDefaultHTTPClient()

	log.Info("Fetching player preferences")
	preferences, err := fetchPlayerPreferences(client, encodedID)
	if err != nil {
		log.WithError(err).Error("Error fetching player preferences")
		sendFollowupMessage(s, i, fmt.Sprintf("Error: Failed to fetch preferences for %s. %v", activisionID, err))
		return
	}

	if !preferences.Visible {
		log.Info("Verdansk stats not available for this account")
		sendFollowupMessage(s, i, fmt.Sprintf("Verdansk stats for %s are not available. This could be because:\n- You haven't played enough in Verdansk (at least 5 deployments required)\n- Your Game Player Data settings need to be updated at https://profile.callofduty.com/cod/login", activisionID))
		return
	}

	if account != nil {
		if !account.IsOGVerdansk {
			log.Info("Setting OGVerdansk flag for account")
			account.IsOGVerdansk = true
			if err := database.DB.Save(account).Error; err != nil {
				log.WithError(err).Error("Failed to update OGVerdansk flag")
			}
		}
	}

	log.Info("Fetching player stats")
	stats, err := fetchPlayerStats(client, encodedID)
	if err != nil {
		log.WithError(err).Error("Error fetching player stats")
		sendFollowupMessage(s, i, fmt.Sprintf("Error: Failed to fetch stats for %s. %v", activisionID, err))
		return
	}

	if len(stats) == 0 {
		log.Warn("No stats found for player")
		sendFollowupMessage(s, i, fmt.Sprintf("No Verdansk stats found for %s.", activisionID))
		return
	}

	log.WithField("statCount", len(stats)).Info("Successfully retrieved stats")

	uniqueID := fmt.Sprintf("%s_%d", strings.ReplaceAll(activisionID, "#", "_"), time.Now().Unix())
	outputDir := filepath.Join(tempDir, uniqueID)
	zipFilename := filepath.Join(tempDir, uniqueID+".zip")

	log.WithFields(logrus.Fields{
		"outputDir":   outputDir,
		"zipFilename": zipFilename,
	}).Info("Downloading stat images")

	images, err := downloadImages(client, stats, outputDir, 3)
	if err != nil {
		log.WithError(err).Error("Error downloading stat images")
		sendFollowupMessage(s, i, fmt.Sprintf("Error: Failed to download stat images for %s. %v", activisionID, err))
		return
	}

	log.WithField("imageCount", len(images)).Info("Downloaded images successfully")

	log.Info("Creating zip file")
	if err := createZip(images, zipFilename); err != nil {
		log.WithError(err).Error("Error creating zip file")
		sendFollowupMessage(s, i, fmt.Sprintf("Error: Failed to create zip file for %s. %v", activisionID, err))
		return
	}

	tempZipFiles.Lock()
	tempZipFiles.files[zipFilename] = time.Now().Add(cleanupTime)
	tempZipFiles.Unlock()

	log.Info("Preparing embeds and files for Discord")
	var embeds []*discordgo.MessageEmbed
	for i, img := range images {
		if i >= 10 {
			break
		}

		formattedName := formatStatName(img.Name)
		embed := &discordgo.MessageEmbed{
			Title:       formattedName,
			Description: fmt.Sprintf("Verdansk Replay Stat %d/%d", i+1, len(images)),
			Image: &discordgo.MessageEmbedImage{
				URL: fmt.Sprintf("attachment://%s.jpg", img.Name),
			},
			Color: 0x00BFFF,
		}
		embeds = append(embeds, embed)
	}

	var files []*discordgo.File
	for i, img := range images {
		if i >= 10 {
			break
		}
		files = append(files, &discordgo.File{
			Name:   fmt.Sprintf("%s.jpg", img.Name),
			Reader: bytes.NewReader(img.Data),
		})
	}

	zipFile, err := os.Open(zipFilename)
	if err != nil {
		log.WithError(err).Error("Error opening zip file")
	} else {
		defer zipFile.Close()
		files = append(files, &discordgo.File{
			Name:   fmt.Sprintf("%s_verdansk_stats.zip", activisionID),
			Reader: zipFile,
		})
	}

	log.Info("Sending stats to Discord")
	_, err = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: fmt.Sprintf("Verdansk Replay Stats for %s\n\nThe images will expire after 30 minutes. Download the zip file for permanent access.", activisionID),
		Embeds:  embeds,
		Files:   files,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		log.WithError(err).Error("Error sending stat images")
		sendFollowupMessage(s, i, fmt.Sprintf("Error: Failed to send stat images for %s. %v", activisionID, err))
		return
	}

	log.Info("Successfully sent Verdansk stats to user")

	go func() {
		time.Sleep(cleanupTime)
		cleanupTempFiles(uniqueID)
	}()

	services.LogCommandExecution("verdansk_stats_retrieved", userID, i.GuildID, true,
		time.Now().UnixMilli(), fmt.Sprintf("Activision ID: %s, Images: %d", activisionID, len(images)))
}

func fetchPlayerPreferences(client *http.Client, encodedGamerTag string) (*PlayerPreferences, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"function":        "fetchPlayerPreferences",
		"encodedGamerTag": encodedGamerTag,
	})

	cfg := configuration.Get()
	preferencesEndpoint := cfg.Verdansk.PreferencesEndpoint
	apiKey := cfg.Verdansk.APIKey

	if preferencesEndpoint == "" {
		preferencesEndpoint = "https://pd.callofduty.com/api/x/v1/campaign/warzonewrapped/preferences/gamer/%s"
	}

	if apiKey == "" {
		apiKey = "a855a770-cf8a-4ae8-9f30-b787d676e608"
	}

	url := fmt.Sprintf(preferencesEndpoint, encodedGamerTag)
	log.WithField("url", url).Debug("Preferences URL")

	log.Info("Sending preflight request")
	if err := doPreflightRequest(client, url); err != nil {
		log.WithError(err).Error("Preflight request failed")
		return nil, fmt.Errorf("preflight request failed: %w", err)
	}

	time.Sleep(time.Duration(300+rand.Intn(500)) * time.Millisecond)

	log.Info("Creating preferences request")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.WithError(err).Error("Error creating request")
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("accept", "*/*")
	req.Header.Set("accept-encoding", "gzip, deflate, br")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("dnt", "1")
	req.Header.Set("origin", "https://www.callofduty.com")
	req.Header.Set("pragma", "no-cache")
	req.Header.Set("referer", "https://www.callofduty.com/")
	req.Header.Set("sec-ch-ua", "\"Chromium\";v=\"134\", \"Not:A-Brand\";v=\"24\", \"Google Chrome\";v=\"134\"")
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", "\"Windows\"")
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-site")
	req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36")
	req.Header.Set("x-api-key", X_APIKey)

	log.Debug("Sending preferences API request")
	resp, err := client.Do(req)
	if err != nil {
		log.WithError(err).Error("Error sending request")
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	log.WithField("statusCode", resp.StatusCode).Debug("Received API response")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.WithFields(logrus.Fields{
			"statusCode": resp.StatusCode,
			"body":       string(body),
		}).Error("API returned non-OK status")
		return nil, fmt.Errorf("API returned status code %d: %s", resp.StatusCode, string(body))
	}

	body, err := readResponseBody(resp)
	if err != nil {
		log.WithError(err).Error("Error reading response body")
		return nil, err
	}

	log.WithField("body", string(body)).Debug("Received preferences response")

	var result PlayerPreferences
	if err := json.Unmarshal(body, &result); err != nil {
		log.WithError(err).Error("Error decoding response")
		return nil, fmt.Errorf("error decoding response: %w, body: %s", err, string(body))
	}

	log.WithField("visible", result.Visible).Info("Successfully fetched player preferences")
	return &result, nil
}

func fetchPlayerStats(client *http.Client, encodedGamerTag string) (map[string]StatValue, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"function":        "fetchPlayerStats",
		"encodedGamerTag": encodedGamerTag,
	})

	cfg := configuration.Get()
	statsEndpoint := cfg.Verdansk.StatsEndpoint

	if statsEndpoint == "" {
		statsEndpoint = "https://pd.callofduty.com/api/x/v1/campaign/warzonewrapped/stats/gamer/%s"
	}

	url := fmt.Sprintf(statsEndpoint, encodedGamerTag)
	log.WithField("url", url).Debug("Stats URL")

	log.Info("Sending preflight request")
	if err := doPreflightRequest(client, url); err != nil {
		log.WithError(err).Error("Preflight request failed")
		return nil, fmt.Errorf("preflight request failed: %w", err)
	}

	time.Sleep(time.Duration(300+rand.Intn(500)) * time.Millisecond)

	log.Info("Creating stats request")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.WithError(err).Error("Error creating request")
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("accept", "*/*")
	req.Header.Set("accept-encoding", "gzip, deflate, br")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("dnt", "1")
	req.Header.Set("origin", "https://www.callofduty.com")
	req.Header.Set("pragma", "no-cache")
	req.Header.Set("referer", "https://www.callofduty.com/")
	req.Header.Set("sec-ch-ua", "\"Chromium\";v=\"134\", \"Not:A-Brand\";v=\"24\", \"Google Chrome\";v=\"134\"")
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", "\"Windows\"")
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-site")
	req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36")
	req.Header.Set("x-api-key", X_APIKey)

	log.Debug("Sending stats API request")
	resp, err := client.Do(req)
	if err != nil {
		log.WithError(err).Error("Error sending request")
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	log.WithField("statusCode", resp.StatusCode).Debug("Received API response")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.WithFields(logrus.Fields{
			"statusCode": resp.StatusCode,
			"body":       string(body),
		}).Error("API returned non-OK status")
		return nil, fmt.Errorf("API returned status code %d: %s", resp.StatusCode, string(body))
	}

	body, err := readResponseBody(resp)
	if err != nil {
		log.WithError(err).Error("Error reading response body")
		return nil, err
	}

	log.WithField("bodyLength", len(body)).Debug("Received stats response")

	var stats map[string]StatValue
	if err := json.Unmarshal(body, &stats); err != nil {
		log.WithError(err).Error("Error decoding response")
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	log.WithField("statCount", len(stats)).Info("Successfully fetched player stats")
	return stats, nil
}

func downloadImages(client *http.Client, stats map[string]StatValue, outputDir string, concurrency int) ([]ImageDownload, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"function":    "downloadImages",
		"outputDir":   outputDir,
		"concurrency": concurrency,
	})

	log.Info("Starting image downloads")

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.WithError(err).Error("Error creating directory")
		return nil, fmt.Errorf("error creating directory: %w", err)
	}

	var downloads []ImageDownload
	for statName, stat := range stats {
		if stat.StringValue != "" && strings.HasPrefix(stat.StringValue, "http") {
			downloads = append(downloads, ImageDownload{
				Name: statName,
				URL:  stat.StringValue,
			})
		}
	}

	if len(downloads) == 0 {
		log.Warn("No images found to download")
		return nil, fmt.Errorf("no images found to download")
	}

	log.WithField("downloadCount", len(downloads)).Info("Preparing to download images")

	results := make(chan ImageDownload, len(downloads))
	var wg sync.WaitGroup
	limiter := make(chan struct{}, concurrency)

	for _, download := range downloads {
		wg.Add(1)
		go func(dl ImageDownload) {
			defer wg.Done()

			dlLog := log.WithFields(logrus.Fields{
				"imageName": dl.Name,
				"imageURL":  dl.URL,
			})

			dlLog.Debug("Starting image download")

			limiter <- struct{}{}
			defer func() { <-limiter }()

			time.Sleep(time.Duration(200+rand.Intn(300)) * time.Millisecond)

			req, err := http.NewRequest("GET", dl.URL, nil)
			if err != nil {
				dlLog.WithError(err).Error("Error creating request")
				dl.Err = fmt.Errorf("error creating request: %w", err)
				results <- dl
				return
			}

			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36")
			req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
			req.Header.Set("Accept-Encoding", "gzip, deflate, br")
			req.Header.Set("Referer", "https://www.callofduty.com/")
			req.Header.Set("Origin", "https://www.callofduty.com")

			dlLog.Debug("Sending image request")
			resp, err := client.Do(req)
			if err != nil {
				dlLog.WithError(err).Error("Error downloading image")
				dl.Err = fmt.Errorf("error downloading: %w", err)
				results <- dl
				return
			}
			defer resp.Body.Close()

			dlLog.WithField("statusCode", resp.StatusCode).Debug("Received image response")
			if resp.StatusCode != http.StatusOK {
				dlLog.WithField("statusCode", resp.StatusCode).Error("Non-OK status code")
				dl.Err = fmt.Errorf("status code %d", resp.StatusCode)
				results <- dl
				return
			}

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				dlLog.WithError(err).Error("Error reading image data")
				dl.Err = fmt.Errorf("error reading data: %w", err)
				results <- dl
				return
			}

			filePath := filepath.Join(outputDir, dl.Name+".jpg")
			dlLog.WithField("filePath", filePath).Debug("Saving image to file")

			if err := os.WriteFile(filePath, data, 0644); err != nil {
				dlLog.WithError(err).Error("Error saving file")
				dl.Err = fmt.Errorf("error saving file: %w", err)
				results <- dl
				return
			}

			dl.Data = data
			dlLog.Info("Successfully downloaded image")
			results <- dl
		}(download)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var downloadedImages []ImageDownload
	var failedCount int

	for result := range results {
		if result.Err == nil {
			downloadedImages = append(downloadedImages, result)
		} else {
			failedCount++
			log.WithFields(logrus.Fields{
				"imageName": result.Name,
				"error":     result.Err,
			}).Error("Failed to download image")
		}
	}

	log.WithFields(logrus.Fields{
		"successCount": len(downloadedImages),
		"failedCount":  failedCount,
	}).Info("Download results")

	return downloadedImages, nil
}

func createZip(images []ImageDownload, zipName string) error {
	log := logger.Log.WithFields(logrus.Fields{
		"function": "createZip",
		"zipName":  zipName,
		"images":   len(images),
	})

	log.Info("Creating ZIP archive")

	zipFile, err := os.Create(zipName)
	if err != nil {
		log.WithError(err).Error("Error creating zip file")
		return fmt.Errorf("error creating zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, img := range images {
		imgLog := log.WithField("imageName", img.Name)
		imgLog.Debug("Adding image to ZIP")

		zipEntry, err := zipWriter.Create(img.Name + ".jpg")
		if err != nil {
			imgLog.WithError(err).Error("Error creating zip entry")
			return fmt.Errorf("error creating zip entry for %s: %w", img.Name, err)
		}

		if _, err := io.Copy(zipEntry, bytes.NewReader(img.Data)); err != nil {
			imgLog.WithError(err).Error("Error writing image to zip")
			return fmt.Errorf("error writing image to zip: %w", err)
		}
	}

	log.Info("Successfully created ZIP archive")
	return nil
}

func cleanupTempFiles(uniqueID string) {
	log := logger.Log.WithFields(logrus.Fields{
		"function": "cleanupTempFiles",
		"uniqueID": uniqueID,
	})

	log.Info("Cleaning up temporary files")

	cfg := configuration.Get()
	tempDir := cfg.Verdansk.TempDir
	if tempDir == "" {
		tempDir = "verdansk_temp"
	}

	tempZipFiles.Lock()
	defer tempZipFiles.Unlock()

	zipFilename := filepath.Join(tempDir, uniqueID+".zip")
	delete(tempZipFiles.files, zipFilename)

	outputDir := filepath.Join(tempDir, uniqueID)
	if err := os.RemoveAll(outputDir); err != nil {
		log.WithError(err).Errorf("Failed to remove temp directory %s", outputDir)
	} else {
		log.WithField("outputDir", outputDir).Info("Removed temp directory")
	}

	if err := os.Remove(zipFilename); err != nil {
		log.WithError(err).Errorf("Failed to remove temp zip file %s", zipFilename)
	} else {
		log.WithField("zipFilename", zipFilename).Info("Removed zip file")
	}
}

func InitCleanupRoutine() {
	log := logger.Log.WithField("function", "InitCleanupRoutine")
	log.Info("Starting cleanup routine for Verdansk temporary files")

	go func() {
		for {
			time.Sleep(10 * time.Minute)
			cleanupOldZipFiles()
		}
	}()
}

func cleanupOldZipFiles() {
	log := logger.Log.WithField("function", "cleanupOldZipFiles")
	log.Debug("Running scheduled cleanup of old zip files")

	cfg := configuration.Get()
	tempDir := cfg.Verdansk.TempDir
	if tempDir == "" {
		tempDir = "verdansk_temp"
	}

	tempZipFiles.Lock()
	defer tempZipFiles.Unlock()

	now := time.Now()
	deletedCount := 0

	for zipFile, expireTime := range tempZipFiles.files {
		if now.After(expireTime) {
			baseName := filepath.Base(zipFile)
			uniqueID := strings.TrimSuffix(baseName, ".zip")

			outputDir := filepath.Join(tempDir, uniqueID)
			if err := os.RemoveAll(outputDir); err != nil {
				log.WithError(err).Errorf("Failed to remove temp directory %s", outputDir)
			}

			if err := os.Remove(zipFile); err != nil {
				log.WithError(err).Errorf("Failed to remove temp zip file %s", zipFile)
			}

			delete(tempZipFiles.files, zipFile)
			deletedCount++
		}
	}

	if deletedCount > 0 {
		log.WithField("deletedCount", deletedCount).Info("Cleaned up expired zip files")
	}
}

func doPreflightRequest(client *http.Client, targetURL string) error {
	log := logger.Log.WithFields(logrus.Fields{
		"function":  "doPreflightRequest",
		"targetURL": targetURL,
	})

	log.Debug("Setting up preflight request")

	req, err := http.NewRequest("OPTIONS", targetURL, nil)
	if err != nil {
		log.WithError(err).Error("Error creating preflight request")
		return fmt.Errorf("error creating preflight request: %w", err)
	}

	req.Header.Set("accept", "*/*")
	req.Header.Set("accept-encoding", "gzip, deflate, br")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	req.Header.Set("access-control-request-headers", "x-api-key")
	req.Header.Set("access-control-request-method", "GET")
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("origin", "https://www.callofduty.com")
	req.Header.Set("pragma", "no-cache")
	req.Header.Set("referer", "https://www.callofduty.com/")
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-site")
	req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36")

	log.Debug("Sending preflight request")
	resp, err := client.Do(req)
	if err != nil {
		log.WithError(err).Error("Error sending preflight request")
		return fmt.Errorf("error sending preflight request: %w", err)
	}
	defer resp.Body.Close()

	log.WithField("statusCode", resp.StatusCode).Debug("Received preflight response")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.WithFields(logrus.Fields{
			"statusCode": resp.StatusCode,
			"body":       string(body),
		}).Error("Preflight request failed")
		return fmt.Errorf("preflight request failed with status code %d: %s", resp.StatusCode, string(body))
	}

	log.Debug("Preflight request successful")
	return nil
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	log := logger.Log.WithFields(logrus.Fields{
		"function":    "readResponseBody",
		"contentType": resp.Header.Get("Content-Type"),
		"encoding":    resp.Header.Get("Content-Encoding"),
	})

	log.Debug("Reading response body")

	var reader io.ReadCloser
	var err error

	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		log.Debug("Using gzip reader for compressed response")
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			log.WithError(err).Error("Error creating gzip reader")
			return nil, fmt.Errorf("error creating gzip reader: %w", err)
		}
		defer reader.Close()
	default:
		log.Debug("Using standard reader for response")
		reader = resp.Body
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		log.WithError(err).Error("Error reading response body")
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	log.WithField("bodyLength", len(body)).Debug("Successfully read response body")
	return body, nil
}

func formatStatName(name string) string {
	words := strings.Split(name, "_")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[0:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}

func sendFollowupMessage(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: message,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error sending followup message")
	}
}
