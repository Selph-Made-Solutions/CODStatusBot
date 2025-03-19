package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	VerdanskPreferencesEndpoint = "https://pd.callofduty.com/api/x/v1/campaign/warzonewrapped/preferences/gamer/%s"
	VerdanskStatsEndpoint       = "https://pd.callofduty.com/api/x/v1/campaign/warzonewrapped/stats/gamer/%s"
	X_APIKey                    = "a855a770-cf8a-4ae8-9f30-b787d676e608"
)

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

func main() {
	var (
		outputDir    = flag.String("output", "verdansk_stats", "Directory to save downloaded images")
		downloadFlag = flag.Bool("download", false, "Download stat images")
		zipFlag      = flag.Bool("zip", false, "Create a zip file of all downloaded images")
		zipName      = flag.String("zipname", "verdansk_stats.zip", "Name of the zip file")
		concurrency  = flag.Int("concurrency", 3, "Number of concurrent downloads")
		timeout      = flag.Int("timeout", 30, "HTTP request timeout in seconds")
		verbose      = flag.Bool("verbose", false, "Show verbose output")
		debug        = flag.Bool("debug", false, "Show debug output for requests and responses")
	)

	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: verdansk [options] <ActivisionID>")
		fmt.Println("Example: verdansk --download --zip YourName#1234")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	activisionID := args[0]
	fmt.Printf("Fetching Verdansk Replay stats for: %s\n", activisionID)

	jar, err := cookiejar.New(nil)
	if err != nil {
		fmt.Printf("Error creating cookie jar: %v\n", err)
		os.Exit(1)
	}

	client := &http.Client{
		Jar:     jar,
		Timeout: time.Duration(*timeout) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	encodedID := strings.Replace(activisionID, "#", "%23", -1)

	err = doPreflightRequest(client, fmt.Sprintf(VerdanskPreferencesEndpoint, encodedID), *verbose, *debug)
	if err != nil {
		fmt.Printf("Error with preflight request: %v\n", err)
		os.Exit(1)
	}

	time.Sleep(time.Duration(500+rand.Intn(500)) * time.Millisecond)

	preferences, err := fetchPlayerPreferences(client, encodedID, *verbose, *debug)
	if err != nil {
		fmt.Printf("Error fetching player preferences: %v\n", err)
		os.Exit(1)
	}

	if !preferences.Visible {
		fmt.Println("Your Verdansk stats are not available. This could be because:")
		fmt.Println("- You haven't played enough in Verdansk (at least 5 deployments required)")
		fmt.Println("- You need to update your Game Player Data settings at https://profile.callofduty.com/cod/login")
		os.Exit(1)
	}

	time.Sleep(time.Duration(300+rand.Intn(300)) * time.Millisecond)

	err = doPreflightRequest(client, fmt.Sprintf(VerdanskStatsEndpoint, encodedID), *verbose, *debug)
	if err != nil {
		fmt.Printf("Error with stats preflight request: %v\n", err)
		os.Exit(1)
	}

	time.Sleep(time.Duration(300+rand.Intn(500)) * time.Millisecond)

	stats, err := fetchPlayerStats(client, encodedID, *verbose, *debug)
	if err != nil {
		fmt.Printf("Error fetching player stats: %v\n", err)
		os.Exit(1)
	}

	displayStats(stats)

	if *downloadFlag || *zipFlag {
		images, err := downloadImages(client, stats, *outputDir, *concurrency, *verbose)
		if err != nil {
			fmt.Printf("Error downloading images: %v\n", err)
			os.Exit(1)
		}

		if *zipFlag && len(images) > 0 {
			err = createZip(images, *zipName)
			if err != nil {
				fmt.Printf("Error creating zip file: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Successfully created zip file: %s\n", *zipName)
		}
	}
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	var reader io.ReadCloser
	var err error

	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error creating gzip reader: %w", err)
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	return body, nil
}

func doPreflightRequest(client *http.Client, targetURL string, verbose, debug bool) error {
	req, err := http.NewRequest("OPTIONS", targetURL, nil)
	if err != nil {
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

	if verbose {
		fmt.Printf("Sending preflight request to: %s\n", targetURL)
	}

	if debug {
		reqDump, _ := httputil.DumpRequestOut(req, true)
		fmt.Printf("DEBUG - Preflight Request:\n%s\n", reqDump)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending preflight request: %w", err)
	}
	defer resp.Body.Close()

	if debug {
		respDump, _ := httputil.DumpResponse(resp, true)
		fmt.Printf("DEBUG - Preflight Response:\n%s\n", respDump)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("preflight request failed with status code %d: %s", resp.StatusCode, string(body))
	}

	if verbose {
		fmt.Println("Preflight request successful")
	}

	return nil
}

func fetchPlayerPreferences(client *http.Client, encodedGamerTag string, verbose, debug bool) (*PlayerPreferences, error) {
	url := fmt.Sprintf(VerdanskPreferencesEndpoint, encodedGamerTag)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
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

	if verbose {
		fmt.Printf("Fetching player preferences from: %s\n", url)
	}

	if debug {
		reqDump, _ := httputil.DumpRequestOut(req, true)
		fmt.Printf("DEBUG - Preferences Request:\n%s\n", reqDump)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if debug {
		respDump, _ := httputil.DumpResponse(resp, true)
		fmt.Printf("DEBUG - Preferences Response:\n%s\n", respDump)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status code %d: %s", resp.StatusCode, string(body))
	}

	if verbose {
		fmt.Println("Received cookies:")
		for _, cookie := range resp.Cookies() {
			fmt.Printf("  %s: %s\n", cookie.Name, cookie.Value)
		}
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if debug {
		fmt.Printf("DEBUG - Decoded body: %s\n", string(body))
	}

	var result PlayerPreferences
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w, body: %s", err, string(body))
	}

	return &result, nil
}

func fetchPlayerStats(client *http.Client, encodedGamerTag string, verbose, debug bool) (map[string]StatValue, error) {
	url := fmt.Sprintf(VerdanskStatsEndpoint, encodedGamerTag)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
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

	if verbose {
		fmt.Printf("Fetching player stats from: %s\n", url)
	}

	if debug {
		reqDump, _ := httputil.DumpRequestOut(req, true)
		fmt.Printf("DEBUG - Stats Request:\n%s\n", reqDump)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if debug {
		respDump, _ := httputil.DumpResponse(resp, true)
		fmt.Printf("DEBUG - Stats Response:\n%s\n", respDump)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status code %d: %s", resp.StatusCode, string(body))
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if debug {
		fmt.Printf("DEBUG - Response body (first 200 bytes): %s\n", string(body[:min(200, len(body))]))
	}

	var stats map[string]StatValue
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, fmt.Errorf("error decoding response: %w, body starts with: %s", err, string(body[:min(100, len(body))]))
	}

	return stats, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func displayStats(stats map[string]StatValue) {
	fmt.Println("\nVerdansk Replay Stats:")
	fmt.Println("=====================")

	imageStats := make(map[int][]string)
	validStatCount := 0

	for statName, stat := range stats {
		if stat.StringValue != "" && strings.HasPrefix(stat.StringValue, "http") {
			validStatCount++
			orderValue := 999
			if stat.OrderValue != nil {
				orderValue = *stat.OrderValue
			}
			imageStats[orderValue] = append(imageStats[orderValue], statName)
		}
	}

	count := 1
	for i := 1; i < 999; i++ {
		if statNames, ok := imageStats[i]; ok {
			for _, name := range statNames {
				fmt.Printf("%d. %s\n", count, formatStatName(name))
				fmt.Printf("   Image URL: %s\n\n", stats[name].StringValue)
				count++
			}
		}
	}

	if statNames, ok := imageStats[999]; ok {
		for _, name := range statNames {
			fmt.Printf("%d. %s\n", count, formatStatName(name))
			fmt.Printf("   Image URL: %s\n\n", stats[name].StringValue)
			count++
		}
	}

	if validStatCount == 0 {
		fmt.Println("No stats images found.")
	} else {
		fmt.Printf("Found %d stat images.\n", validStatCount)
		fmt.Println("\nUse --download to save these images to disk")
		fmt.Println("Use --zip to create a zip file of all images")
	}
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

func downloadImages(client *http.Client, stats map[string]StatValue, outputDir string, concurrency int, verbose bool) ([]ImageDownload, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
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
		return nil, fmt.Errorf("no images found to download")
	}

	fmt.Printf("\nDownloading %d images to %s:\n", len(downloads), outputDir)

	results := make(chan ImageDownload, len(downloads))

	var wg sync.WaitGroup
	limiter := make(chan struct{}, concurrency)

	for _, download := range downloads {
		wg.Add(1)
		go func(dl ImageDownload) {
			defer wg.Done()

			limiter <- struct{}{}
			defer func() { <-limiter }()

			time.Sleep(time.Duration(300+rand.Intn(700)) * time.Millisecond)

			fmt.Printf("Downloading %s...\n", formatStatName(dl.Name))

			req, err := http.NewRequest("GET", dl.URL, nil)
			if err != nil {
				dl.Err = fmt.Errorf("error creating request: %w", err)
				results <- dl
				return
			}

			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36")
			req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
			req.Header.Set("Accept-Encoding", "gzip, deflate, br")
			req.Header.Set("Referer", "https://www.callofduty.com/")
			req.Header.Set("Origin", "https://www.callofduty.com")

			resp, err := client.Do(req)
			if err != nil {
				dl.Err = fmt.Errorf("error downloading: %w", err)
				results <- dl
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				dl.Err = fmt.Errorf("status code %d", resp.StatusCode)
				results <- dl
				return
			}

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				dl.Err = fmt.Errorf("error reading data: %w", err)
				results <- dl
				return
			}
			filePath := filepath.Join(outputDir, dl.Name+".jpg")
			if err := os.WriteFile(filePath, data, 0644); err != nil {
				dl.Err = fmt.Errorf("error saving file: %w", err)
				results <- dl
				return
			}

			dl.Data = data
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
		if result.Err != nil {
			failedCount++
			fmt.Printf("❌ Failed %s: %v\n", formatStatName(result.Name), result.Err)
		} else {
			downloadedImages = append(downloadedImages, result)
			fmt.Printf("✅ Downloaded %s\n", formatStatName(result.Name))
		}
	}

	fmt.Printf("\nDownloaded %d/%d images successfully to %s\n", len(downloadedImages), len(downloads), outputDir)
	if failedCount > 0 {
		fmt.Printf("%d downloads failed\n", failedCount)
	}

	return downloadedImages, nil
}

func createZip(images []ImageDownload, zipName string) error {
	fmt.Printf("Creating zip file %s...\n", zipName)

	zipFile, err := os.Create(zipName)
	if err != nil {
		return fmt.Errorf("error creating zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, img := range images {
		zipEntry, err := zipWriter.Create(img.Name + ".jpg")
		if err != nil {
			return fmt.Errorf("error creating zip entry for %s: %w", img.Name, err)
		}

		if _, err := io.Copy(zipEntry, bytes.NewReader(img.Data)); err != nil {
			return fmt.Errorf("error writing image to zip: %w", err)
		}
	}

	return nil
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
