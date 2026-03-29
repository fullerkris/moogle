package crawler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/IonelPopJara/search-engine/services/spider/internal/utils"
)

type FetchConfig struct {
	Client       *http.Client
	UserAgent    string
	MaxBodyBytes int64
}

var fetchConfig = FetchConfig{
	Client: &http.Client{
		Timeout: time.Duration(utils.DefaultHTTPTimeoutSeconds) * time.Second,
	},
	UserAgent:    utils.DefaultHTTPUserAgent,
	MaxBodyBytes: utils.DefaultHTTPMaxBodyBytes,
}

func SetFetchConfig(config FetchConfig) {
	if config.Client != nil {
		fetchConfig.Client = config.Client
	}

	if config.UserAgent != "" {
		fetchConfig.UserAgent = config.UserAgent
	}

	if config.MaxBodyBytes > 0 {
		fetchConfig.MaxBodyBytes = config.MaxBodyBytes
	}
}

func ResetFetchConfigToDefault() {
	fetchConfig = FetchConfig{
		Client: &http.Client{
			Timeout: time.Duration(utils.DefaultHTTPTimeoutSeconds) * time.Second,
		},
		UserAgent:    utils.DefaultHTTPUserAgent,
		MaxBodyBytes: utils.DefaultHTTPMaxBodyBytes,
	}
}

// Return HTML, Status Code, Content-Type, and Error Code
func getPageData(rawURL string) (string, int, string, error) {
	if fetchConfig.Client == nil {
		return "", 0, "", fmt.Errorf("HTTP client is not configured")
	}

	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, "", fmt.Errorf("failed to build request: %w", err)
	}

	if fetchConfig.UserAgent != "" {
		request.Header.Set("User-Agent", fetchConfig.UserAgent)
	}

	res, err := fetchConfig.Client.Do(request)

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", 0, "", fmt.Errorf("request timeout for %s: %w", rawURL, err)
		}

		return "", 0, "", fmt.Errorf("failed to fetch URL: %w", err)
	}

	defer res.Body.Close() // Close the body to prevent memory leaks or something I don't remember

	if res.StatusCode > 399 {
		return "", res.StatusCode, "", fmt.Errorf("HTTP error for %s: %d %s", rawURL, res.StatusCode, http.StatusText(res.StatusCode))
	}

	contentTypeHeader := res.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentTypeHeader)
	if err != nil {
		return "", res.StatusCode, contentTypeHeader, fmt.Errorf("invalid content type header %q: %w", contentTypeHeader, err)
	}

	if !strings.EqualFold(mediaType, "text/html") {
		return "", res.StatusCode, contentTypeHeader, fmt.Errorf("invalid content type: %s", contentTypeHeader)
	}

	limitedReader := io.LimitReader(res.Body, fetchConfig.MaxBodyBytes+1)
	body, err := io.ReadAll(limitedReader)

	if err != nil {
		return "", res.StatusCode, mediaType, fmt.Errorf("failed to read response body: %w", err)
	}

	if int64(len(body)) > fetchConfig.MaxBodyBytes {
		return "", res.StatusCode, mediaType, fmt.Errorf("response body exceeds max size (%d bytes)", fetchConfig.MaxBodyBytes)
	}

	return string(body), res.StatusCode, mediaType, nil
}
