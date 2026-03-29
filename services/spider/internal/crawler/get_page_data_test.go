package crawler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetPageDataSuccess(t *testing.T) {
	const userAgent = "MoogleSpider-TestAgent"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != userAgent {
			t.Fatalf("expected user-agent %q, got %q", userAgent, got)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Hello</body></html>"))
	}))
	defer server.Close()

	SetFetchConfig(FetchConfig{
		Client:       server.Client(),
		UserAgent:    userAgent,
		MaxBodyBytes: 1024,
	})
	t.Cleanup(ResetFetchConfigToDefault)

	html, statusCode, contentType, err := getPageData(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if statusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, statusCode)
	}

	if contentType != "text/html" {
		t.Fatalf("expected content-type text/html, got %q", contentType)
	}

	if !strings.Contains(html, "Hello") {
		t.Fatalf("expected response body to contain Hello, got %q", html)
	}
}

func TestGetPageDataStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	SetFetchConfig(FetchConfig{Client: server.Client(), MaxBodyBytes: 1024})
	t.Cleanup(ResetFetchConfigToDefault)

	_, statusCode, _, err := getPageData(server.URL)
	if err == nil {
		t.Fatalf("expected error for 404 response")
	}

	if statusCode != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, statusCode)
	}
}

func TestGetPageDataContentTypeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	SetFetchConfig(FetchConfig{Client: server.Client(), MaxBodyBytes: 1024})
	t.Cleanup(ResetFetchConfigToDefault)

	_, statusCode, contentType, err := getPageData(server.URL)
	if err == nil {
		t.Fatalf("expected content-type validation error")
	}

	if statusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, statusCode)
	}

	if contentType != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", contentType)
	}
}

func TestGetPageDataBodyTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>0123456789</html>"))
	}))
	defer server.Close()

	SetFetchConfig(FetchConfig{Client: server.Client(), MaxBodyBytes: 5})
	t.Cleanup(ResetFetchConfigToDefault)

	_, _, _, err := getPageData(server.URL)
	if err == nil {
		t.Fatalf("expected body size limit error")
	}
}

func TestGetPageDataTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>slow response</html>"))
	}))
	defer server.Close()

	SetFetchConfig(FetchConfig{
		Client: &http.Client{Timeout: 10 * time.Millisecond},
	})
	t.Cleanup(ResetFetchConfigToDefault)

	_, _, _, err := getPageData(server.URL)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}
