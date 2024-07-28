package services

import (
	"CODStatusBot/logger"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	TwoCaptchaAPIURL    = "https://2captcha.com/in.php"
	TwoCaptchaResultURL = "https://2captcha.com/res.php"
	TwoCaptchaTimeout   = 120 * time.Second
)

var (
	twoCaptchaAPIKey string
)

func SolveTwoCaptchaReCaptchaV2() (string, error) {
	return SolveTwoCaptchaReCaptchaV2WithKey(twoCaptchaAPIKey)
}

func SolveTwoCaptchaReCaptchaV2WithKey(apiKey string) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("2captcha API key is empty")
	}

	logger.Log.Info("Starting to solve ReCaptcha V2 using 2captcha")

	requestID, err := createTwoCaptchaTaskWithKey(apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to create 2captcha task: %v", err)
	}

	solution, err := getTwoCaptchaTaskResultWithKey(requestID, apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to get 2captcha task result: %v", err)
	}

	logger.Log.Info("Successfully solved ReCaptcha V2")
	return solution, nil
}

func createTwoCaptchaTaskWithKey(apiKey string) (string, error) {
	params := url.Values{}
	params.Add("key", apiKey)
	params.Add("method", "userrecaptcha")
	params.Add("googlekey", siteKey)
	params.Add("pageurl", pageURL)
	params.Add("json", "1") // Request JSON response

	resp, err := http.PostForm(TwoCaptchaAPIURL, params)
	if err != nil {
		return "", fmt.Errorf("failed to send 2captcha task creation request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read 2captcha response body: %v", err)
	}

	logger.Log.Infof("2captcha API response: %s", string(body))

	var result struct {
		Status  int    `json:"status"`
		Request string `json:"request"`
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", fmt.Errorf("failed to parse 2captcha task creation response: %v", err)
	}

	if result.Status != 1 {
		return "", fmt.Errorf("2captcha task creation failed with status: %d", result.Status)
	}

	return result.Request, nil
}

func getTwoCaptchaTaskResultWithKey(requestID string, apiKey string) (string, error) {
	params := url.Values{}
	params.Add("key", apiKey)
	params.Add("action", "get")
	params.Add("id", requestID)
	params.Add("json", "1") // Request JSON response

	startTime := time.Now()

	for {
		resp, err := http.Get(TwoCaptchaResultURL + "?" + params.Encode())
		if err != nil {
			return "", fmt.Errorf("failed to send 2captcha task result request: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read 2captcha task result response: %v", err)
		}

		logger.Log.Infof("2captcha result API response: %s", string(body))

		var result struct {
			Status  int    `json:"status"`
			Request string `json:"request"`
		}

		err = json.Unmarshal(body, &result)
		if err != nil {
			return "", fmt.Errorf("failed to parse 2captcha task result response: %v", err)
		}

		if result.Status == 1 {
			return result.Request, nil
		}

		if time.Since(startTime) > TwoCaptchaTimeout {
			return "", fmt.Errorf("2captcha task timed out")
		}

		time.Sleep(5 * time.Second)
	}
}

func LoadTwoCaptchaEnvironmentVariables() error {
	twoCaptchaAPIKey = os.Getenv("TWOCAPTCHA_API_KEY")

	if twoCaptchaAPIKey == "" {
		return fmt.Errorf("TWOCAPTCHA_API_KEY is not set in the environment")
	}
	return nil
}
