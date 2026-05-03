package crawler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBuildRedditJSONURL(t *testing.T) {
	tests := []struct {
		name     string
		inputURL string
		expected string
	}{
		{
			name:     "subreddit root keeps trailing slash",
			inputURL: "https://old.reddit.com/r/programming/",
			expected: "https://old.reddit.com/r/programming/.json",
		},
		{
			name:     "listing path appends json",
			inputURL: "https://www.reddit.com/r/linux/hot",
			expected: "https://www.reddit.com/r/linux/hot.json",
		},
		{
			name:     "preserves query string",
			inputURL: "https://www.reddit.com/r/linux/hot?limit=10",
			expected: "https://www.reddit.com/r/linux/hot.json?limit=10",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := buildRedditJSONURL(tc.inputURL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if actual != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, actual)
			}
		})
	}
}

func TestParseRedditJSONSnapshotListing(t *testing.T) {
	body := []byte(`{
		"kind":"Listing",
		"data":{
			"children":[
				{
					"kind":"t3",
					"data":{
						"title":"Programming post",
						"selftext":"Some body text",
						"url":"https://old.reddit.com/r/programming/comments/abc123/programming_post/",
						"permalink":"/r/programming/comments/abc123/programming_post/",
						"preview":{
							"images":[{"source":{"url":"https://preview.redd.it/abc123.png?auto=webp&amp;s=xyz"}}]
						}
					}
				}
			]
		}
	}`)

	links, images, html, err := parseRedditJSONSnapshot(body, "https://old.reddit.com/r/programming/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(links) == 0 {
		t.Fatalf("expected extracted links")
	}

	if _, exists := images["https://preview.redd.it/abc123.png?auto=webp&s=xyz"]; !exists {
		t.Fatalf("expected preview image to be extracted, got %v", images)
	}

	if !strings.Contains(html, "Programming post") {
		t.Fatalf("expected synthetic html to include post title")
	}

	if !strings.Contains(html, "Some body text") {
		t.Fatalf("expected synthetic html to include selftext")
	}
}

func TestParseRedditJSONSnapshotPostArray(t *testing.T) {
	body := []byte(`[
		{
			"kind":"Listing",
			"data":{
				"children":[
					{
						"kind":"t3",
						"data":{
							"title":"Gallery post",
							"selftext":"",
							"url":"https://www.reddit.com/gallery/123",
							"permalink":"/r/pics/comments/123/gallery_post/",
							"is_gallery":true,
							"gallery_data":{"items":[{"media_id":"media1"}]},
							"media_metadata":{"media1":{"status":"valid","e":"Image","s":{"url":"https://i.redd.it/gallery1.jpeg"}}}
						}
					}
				]
			}
		},
		{
			"kind":"Listing",
			"data":{
				"children":[
					{
						"kind":"t1",
						"data":{
							"body":"Top level comment",
							"replies":{
								"kind":"Listing",
								"data":{
									"children":[
										{
											"kind":"t1",
											"data":{"body":"Nested reply"}
										}
									]
								}
							}
						}
					}
				]
			}
		}
	]`)

	links, images, html, err := parseRedditJSONSnapshot(body, "https://www.reddit.com/r/pics/comments/123/gallery_post/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(links) == 0 {
		t.Fatalf("expected extracted links for post array")
	}

	if _, exists := images["https://i.redd.it/gallery1.jpeg"]; !exists {
		t.Fatalf("expected gallery image to be extracted, got %v", images)
	}

	if !strings.Contains(html, "Gallery post") {
		t.Fatalf("expected synthetic html to include gallery post title")
	}

	if !strings.Contains(html, "Top level comment") || !strings.Contains(html, "Nested reply") {
		t.Fatalf("expected synthetic html to include comment text")
	}
}

func TestShouldReplaceHTMLWithRedditJSON(t *testing.T) {
	if !shouldReplaceHTMLWithRedditJSON("<html><title>Reddit - Please wait for verification</title></html>", []string{"https://example.com"}) {
		t.Fatalf("expected verification page to be replaceable")
	}

	if !shouldReplaceHTMLWithRedditJSON("<html></html>", nil) {
		t.Fatalf("expected empty outlinks to trigger replacement")
	}

	if shouldReplaceHTMLWithRedditJSON("<html><title>OK</title></html>", []string{"https://example.com"}) {
		t.Fatalf("did not expect replacement for normal html with links")
	}
}

func TestGetPageDataWithRedditFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/r/linux/hot":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("forbidden"))
		case "/r/linux/hot.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"kind":"Listing",
				"data":{
					"children":[
						{
							"kind":"t3",
							"data":{
								"title":"Linux hot post",
								"selftext":"real reddit json content",
								"url":"https://www.reddit.com/r/linux/comments/abc/linux_hot_post/",
								"permalink":"/r/linux/comments/abc/linux_hot_post/"
							}
						}
					]
				}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("unexpected server URL parse error: %v", err)
	}

	SetFetchConfig(FetchConfig{
		Client: &http.Client{
			Timeout: 2 * time.Second,
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				cloned := req.Clone(req.Context())
				cloned.URL.Scheme = serverURL.Scheme
				cloned.URL.Host = serverURL.Host
				cloned.Host = serverURL.Host
				return http.DefaultTransport.RoundTrip(cloned)
			}),
		},
	})
	defer ResetFetchConfigToDefault()

	rawURL := "https://www.reddit.com/r/linux/hot"
	html, statusCode, contentType, usedFallback, err := getPageDataWithRedditFallback(rawURL)
	if err != nil {
		t.Fatalf("expected fallback success, got error: %v", err)
	}

	if !usedFallback {
		t.Fatalf("expected reddit json fallback to be used")
	}

	if statusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", statusCode)
	}

	if contentType != "text/html" {
		t.Fatalf("expected text/html content type, got %s", contentType)
	}

	if !strings.Contains(html, "Linux hot post") || !strings.Contains(html, "real reddit json content") {
		t.Fatalf("expected synthetic html from reddit json fallback, got %s", html)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
