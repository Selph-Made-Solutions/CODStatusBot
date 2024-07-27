package services

import (
	"CODStatusBot/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	TwoCaptchaAPIURL    = "https://2captcha.com/in.php"
	TwoCaptchaResultURL = "https://2captcha.com/res.php"
	TwoCaptchaTimeout   = 30 * time.Second
)

var (
	twoCaptchaAPIKey string
	twoCaptchaAppID  string
)

type twoCaptchaResponse struct {
	Status    int    `json:"status"`
	RequestID string `json:"request"`
}

func SolveTwoCaptchaReCaptchaV2() (string, error) {
	logger.Log.Info("Starting to solve ReCaptcha V2 using 2captcha")

	requestID, err := createTwoCaptchaTask()
	if err != nil {
		return "", err
	}

	solution, err := getTwoCaptchaTaskResult(requestID)
	if err != nil {
		return "", err
	}

	logger.Log.Info("Successfully solved ReCaptcha V2")
	return solution, nil
}

func SolveTwoCaptchaReCaptchaV2WithKey(apiKey string) (string, error) {
	logger.Log.Info("Starting to solve ReCaptcha V2 using 2captcha with custom API key")

	requestID, err := createTwoCaptchaTaskWithKey(apiKey)
	if err != nil {
		return "", err
	}

	solution, err := getTwoCaptchaTaskResultWithKey(requestID, apiKey)
	if err != nil {
		return "", err
	}

	logger.Log.Info("Successfully solved ReCaptcha V2 with custom API key")
	return solution, nil
}

func createTwoCaptchaTaskWithKey(apiKey string) (string, error) {
	params := url.Values{}
	params.Add("key", apiKey)
	if twoCaptchaAppID != "" {
		//params.Add("appid", twoCaptchaAppID)
	}
	params.Add("method", "userrecaptcha")
	params.Add("googlekey", siteKey)
	params.Add("pageurl", pageURL)

	resp, err := http.PostForm(TwoCaptchaAPIURL, params)
	if err != nil {
		return "", fmt.Errorf("failed to send 2captcha task creation request: %v", err)
	}
	defer resp.Body.Close()

	var result twoCaptchaResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", fmt.Errorf("failed to parse 2captcha task creation response: %v", err)
	}

	if result.Status != 1 {
		return "", fmt.Errorf("2captcha task creation failed with status: %d", result.Status)
	}

	return result.RequestID, nil
}

func LoadTwoCaptchaEnvironmentVariables() error {
	twoCaptchaAPIKey = os.Getenv("TWOCAPTCHA_API_KEY")
	twoCaptchaAppID = os.Getenv("TWOCAPAPPID")

	if twoCaptchaAPIKey == "" {
		return fmt.Errorf("TWOCAPTCHA_API_KEY is not set in the environment")
	}
	return nil
}

func createTwoCaptchaTask() (string, error) {
	return createTwoCaptchaTaskWithKey(twoCaptchaAPIKey)
}

func getTwoCaptchaTaskResult(requestID string) (string, error) {
	return getTwoCaptchaTaskResultWithKey(requestID, twoCaptchaAPIKey)
}

func getTwoCaptchaTaskResultWithKey(requestID string, apiKey string) (string, error) {
	params := url.Values{}
	params.Add("key", apiKey)
	params.Add("action", "get")
	params.Add("id", requestID)

	startTime := time.Now()

	for {
		resp, err := http.PostForm(TwoCaptchaResultURL, params)
		if err != nil {
			return "", fmt.Errorf("failed to send 2captcha task result request: %v", err)
		}
		defer resp.Body.Close()

		var result struct {
			Status  int    `json:"status"`
			Request string `json:"request"`
		}
		err = json.NewDecoder(resp.Body).Decode(&result)
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
