package services

import (
	"net/http"
	"sync"
	"time"

	"github.com/bradselph/CODStatusBot/logger"
)

var (
	defaultClient      *http.Client
	longTimeoutClient  *http.Client
	clientMutex        sync.RWMutex
	clientsInitialized bool
	defaultUserAgents  = []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:123.0) Gecko/20100101 Firefox/123.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36 Edg/133.0.2623.71",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
	}
)

func InitHTTPClients() {
	clientMutex.Lock()
	defer clientMutex.Unlock()

	if clientsInitialized {
		return
	}

	proxyManager := GetProxyManager()

	if proxyManager.ProxyEnabled && len(proxyManager.Proxies) > 0 {
		logger.Log.Infof("Using proxies for HTTP clients, %d proxies available", len(proxyManager.Proxies))
		defaultClient = proxyManager.GetClient()

		longTimeoutClient = &http.Client{
			Timeout:   60 * time.Second,
			Transport: defaultClient.Transport,
		}
	} else {
		logger.Log.Info("Using direct HTTP clients without proxies")

		transport := &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
			DisableKeepAlives:   false,
			ForceAttemptHTTP2:   true,
			MaxConnsPerHost:     0,
			TLSHandshakeTimeout: 10 * time.Second,
		}

		var userAgents []string
		if len(proxyManager.UserAgents) > 0 {
			userAgents = proxyManager.UserAgents
		} else {
			userAgents = defaultUserAgents
		}

		defaultClient = &http.Client{
			Timeout:   30 * time.Second,
			Transport: NewHeaderTransport(transport, userAgents),
		}

		longTimeoutClient = &http.Client{
			Timeout:   60 * time.Second,
			Transport: NewHeaderTransport(transport, userAgents),
		}
	}

	clientsInitialized = true
}

func GetDefaultHTTPClient() *http.Client {
	clientMutex.RLock()

	if !clientsInitialized {
		clientMutex.RUnlock()
		InitHTTPClients()
		clientMutex.RLock()
	}

	proxyManager := GetProxyManager()
	if proxyManager.ProxyEnabled {
		clientMutex.RUnlock()
		return proxyManager.GetClient()
	}

	defer clientMutex.RUnlock()
	return defaultClient
}

func GetLongTimeoutHTTPClient() *http.Client {
	clientMutex.RLock()

	if !clientsInitialized {
		clientMutex.RUnlock()
		InitHTTPClients()
		clientMutex.RLock()
	}

	proxyManager := GetProxyManager()
	if proxyManager.ProxyEnabled {
		clientMutex.RUnlock()

		client := proxyManager.GetClient()
		return &http.Client{
			Timeout:   60 * time.Second,
			Transport: client.Transport,
		}
	}

	defer clientMutex.RUnlock()
	return longTimeoutClient
}
