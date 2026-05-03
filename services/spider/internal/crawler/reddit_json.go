package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	htmlpkg "html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

const redditJSONContentType = "application/json"

type redditListingResponse struct {
	Kind string            `json:"kind"`
	Data redditListingData `json:"data"`
}

type redditListingData struct {
	Children []redditThing `json:"children"`
}

type redditThing struct {
	Kind string         `json:"kind"`
	Data redditPostData `json:"data"`
}

type redditPostData struct {
	Title      string                           `json:"title"`
	SelfText   string                           `json:"selftext"`
	Body       string                           `json:"body"`
	URL        string                           `json:"url"`
	Permalink  string                           `json:"permalink"`
	PostHint   string                           `json:"post_hint"`
	Thumbnail  string                           `json:"thumbnail"`
	Preview    redditPreview                    `json:"preview"`
	IsGallery  bool                             `json:"is_gallery"`
	Gallery    redditGalleryData                `json:"gallery_data"`
	MediaItems map[string]redditMediaMetaRecord `json:"media_metadata"`
	Subreddit  string                           `json:"subreddit"`
	Name       string                           `json:"name"`
	Replies    json.RawMessage                  `json:"replies"`
}

type redditPreview struct {
	Images []redditPreviewImage `json:"images"`
}

type redditPreviewImage struct {
	Source redditPreviewSource `json:"source"`
}

type redditPreviewSource struct {
	URL string `json:"url"`
}

type redditGalleryData struct {
	Items []redditGalleryItem `json:"items"`
}

type redditGalleryItem struct {
	MediaID string `json:"media_id"`
}

type redditMediaMetaRecord struct {
	Status   string                `json:"status"`
	Type     string                `json:"e"`
	Source   redditPreviewSource   `json:"s"`
	Previews []redditPreviewSource `json:"p"`
}

func enrichRedditPage(rawCurrentURL string, html string, outgoingLinks []string, imagesMap map[string]map[string]string) ([]string, map[string]map[string]string, string, error) {
	if !isRedditURL(rawCurrentURL) {
		return outgoingLinks, imagesMap, html, nil
	}

	redditLinks, redditImages, syntheticHTML, err := fetchRedditJSONSnapshot(rawCurrentURL)
	if err != nil {
		return outgoingLinks, imagesMap, html, err
	}

	mergedLinks := mergeLinks(outgoingLinks, redditLinks)
	mergedImages := mergeImages(imagesMap, redditImages)

	if shouldReplaceHTMLWithRedditJSON(html, outgoingLinks) && syntheticHTML != "" {
		html = syntheticHTML
	}

	return mergedLinks, mergedImages, html, nil
}

func isRedditURL(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsedURL.Hostname())
	return host == "reddit.com" || strings.HasSuffix(host, ".reddit.com")
}

func shouldReplaceHTMLWithRedditJSON(html string, outgoingLinks []string) bool {
	if len(outgoingLinks) == 0 {
		return true
	}

	return strings.Contains(strings.ToLower(html), "please wait for verification")
}

func fetchRedditJSONSnapshot(rawCurrentURL string) ([]string, map[string]map[string]string, string, error) {
	jsonURL, err := buildRedditJSONURL(rawCurrentURL)
	if err != nil {
		return nil, nil, "", err
	}

	body, err := getStructuredPageData(jsonURL, redditJSONContentType)
	if err != nil {
		return nil, nil, "", err
	}

	return parseRedditJSONSnapshot(body, rawCurrentURL)
}

func getPageDataWithRedditFallback(rawURL string) (string, int, string, bool, error) {
	html, statusCode, contentType, err := getPageData(rawURL)
	if err == nil || !isRedditURL(rawURL) {
		return html, statusCode, contentType, false, err
	}

	_, _, syntheticHTML, redditErr := fetchRedditJSONSnapshot(rawURL)
	if redditErr != nil || syntheticHTML == "" {
		return html, statusCode, contentType, false, err
	}

	return syntheticHTML, http.StatusOK, "text/html", true, nil
}

func buildRedditJSONURL(rawCurrentURL string) (string, error) {
	parsedURL, err := url.Parse(rawCurrentURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse reddit URL: %w", err)
	}

	if strings.HasSuffix(parsedURL.Path, ".json") {
		return parsedURL.String(), nil
	}

	if strings.HasSuffix(parsedURL.Path, "/") {
		parsedURL.Path += ".json"
	} else {
		parsedURL.Path += ".json"
	}

	return parsedURL.String(), nil
}

func getStructuredPageData(rawURL string, expectedMediaType string) ([]byte, error) {
	if fetchConfig.Client == nil {
		return nil, fmt.Errorf("HTTP client is not configured")
	}

	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	if fetchConfig.UserAgent != "" {
		request.Header.Set("User-Agent", fetchConfig.UserAgent)
	}

	res, err := fetchConfig.Client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("request timeout for %s: %w", rawURL, err)
		}

		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode > 399 {
		return nil, fmt.Errorf("HTTP error for %s: %d %s", rawURL, res.StatusCode, http.StatusText(res.StatusCode))
	}

	contentTypeHeader := res.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentTypeHeader)
	if err != nil {
		return nil, fmt.Errorf("invalid content type header %q: %w", contentTypeHeader, err)
	}

	if !strings.EqualFold(mediaType, expectedMediaType) {
		return nil, fmt.Errorf("invalid content type: %s", contentTypeHeader)
	}

	limitedReader := io.LimitReader(res.Body, fetchConfig.MaxBodyBytes+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if int64(len(body)) > fetchConfig.MaxBodyBytes {
		return nil, fmt.Errorf("response body exceeds max size (%d bytes)", fetchConfig.MaxBodyBytes)
	}

	return body, nil
}

func parseRedditJSONSnapshot(body []byte, rawCurrentURL string) ([]string, map[string]map[string]string, string, error) {
	var listing redditListingResponse
	if err := json.Unmarshal(body, &listing); err == nil && listing.Kind == "Listing" {
		links, images, html := buildRedditSnapshot(listing.Data.Children, rawCurrentURL, nil)
		return links, images, html, nil
	}

	var listingArray []redditListingResponse
	if err := json.Unmarshal(body, &listingArray); err == nil && len(listingArray) > 0 {
		commentParagraphs := []string{}
		if len(listingArray) > 1 {
			commentParagraphs = extractRedditCommentParagraphs(listingArray[1].Data.Children, 200)
		}

		links, images, html := buildRedditSnapshot(listingArray[0].Data.Children, rawCurrentURL, commentParagraphs)
		return links, images, html, nil
	}

	return nil, nil, "", fmt.Errorf("unsupported reddit json response")
}

func buildRedditSnapshot(children []redditThing, rawCurrentURL string, commentParagraphs []string) ([]string, map[string]map[string]string, string) {
	linksSet := make(map[string]struct{})
	imagesMap := make(map[string]map[string]string)
	paragraphs := make([]string, 0, len(children)*2)
	titleParts := make([]string, 0, len(children))
	descriptionParts := make([]string, 0, len(children))

	for _, child := range children {
		post := child.Data

		if post.Title != "" {
			titleParts = append(titleParts, post.Title)
			paragraphs = append(paragraphs, post.Title)
			descriptionParts = append(descriptionParts, post.Title)
		}

		if post.SelfText != "" {
			paragraphs = append(paragraphs, post.SelfText)
			descriptionParts = append(descriptionParts, post.SelfText)
		}

		if post.Body != "" {
			paragraphs = append(paragraphs, post.Body)
		}

		if permalinkURL := resolveRedditPermalink(post.Permalink); permalinkURL != "" {
			linksSet[permalinkURL] = struct{}{}
		}

		if postURL := sanitizeRedditURL(post.URL); postURL != "" {
			linksSet[postURL] = struct{}{}
		}

		if thumbnailURL := sanitizeRedditURL(post.Thumbnail); thumbnailURL != "" {
			imagesMap[thumbnailURL] = map[string]string{"src": thumbnailURL, "alt": post.Title}
		}

		for _, previewImage := range post.Preview.Images {
			previewURL := sanitizeRedditURL(previewImage.Source.URL)
			if previewURL == "" {
				continue
			}

			imagesMap[previewURL] = map[string]string{"src": previewURL, "alt": post.Title}
		}

		if post.IsGallery {
			for _, item := range post.Gallery.Items {
				mediaItem, exists := post.MediaItems[item.MediaID]
				if !exists {
					continue
				}

				mediaURL := sanitizeRedditURL(mediaItem.Source.URL)
				if mediaURL == "" {
					continue
				}

				imagesMap[mediaURL] = map[string]string{"src": mediaURL, "alt": post.Title}
			}
		}
	}

	paragraphs = append(paragraphs, commentParagraphs...)

	links := make([]string, 0, len(linksSet))
	for link := range linksSet {
		links = append(links, link)
	}
	sort.Strings(links)

	return links, imagesMap, buildSyntheticRedditHTML(rawCurrentURL, titleParts, descriptionParts, paragraphs)
}

func extractRedditCommentParagraphs(children []redditThing, remaining int) []string {
	if remaining <= 0 {
		return nil
	}

	paragraphs := make([]string, 0, remaining)
	for _, child := range children {
		if remaining <= 0 {
			break
		}

		if strings.EqualFold(child.Kind, "t1") {
			body := strings.TrimSpace(child.Data.Body)
			if body != "" {
				paragraphs = append(paragraphs, body)
				remaining--
			}
		}

		if remaining <= 0 {
			break
		}

		nested, consumed := extractNestedRedditReplies(child.Data.Replies, remaining)
		paragraphs = append(paragraphs, nested...)
		remaining -= consumed
	}

	return paragraphs
}

func extractNestedRedditReplies(rawReplies json.RawMessage, remaining int) ([]string, int) {
	trimmed := strings.TrimSpace(string(rawReplies))
	if remaining <= 0 || trimmed == "" || trimmed == `""` || trimmed == "null" {
		return nil, 0
	}

	var listing redditListingResponse
	if err := json.Unmarshal(rawReplies, &listing); err != nil {
		return nil, 0
	}

	paragraphs := extractRedditCommentParagraphs(listing.Data.Children, remaining)
	return paragraphs, len(paragraphs)
}

func resolveRedditPermalink(permalink string) string {
	if permalink == "" {
		return ""
	}

	baseURL, err := url.Parse("https://www.reddit.com")
	if err != nil {
		return ""
	}

	permalinkURL, err := url.Parse(permalink)
	if err != nil {
		return ""
	}

	return baseURL.ResolveReference(permalinkURL).String()
}

func sanitizeRedditURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" || trimmed == "self" || trimmed == "default" || trimmed == "nsfw" || trimmed == "spoiler" {
		return ""
	}

	trimmed = htmlpkg.UnescapeString(trimmed)
	if strings.HasPrefix(trimmed, "/") {
		return resolveRedditPermalink(trimmed)
	}

	return trimmed
}

func buildSyntheticRedditHTML(rawCurrentURL string, titleParts []string, descriptionParts []string, paragraphs []string) string {
	title := "Reddit JSON"
	if len(titleParts) > 0 {
		title = titleParts[0]
	}

	description := strings.Join(descriptionParts, " ")
	description = strings.TrimSpace(description)
	if len(description) > 280 {
		description = description[:280]
	}

	var builder strings.Builder
	builder.WriteString("<!DOCTYPE html><html><head>")
	builder.WriteString("<meta charset=\"UTF-8\" />")
	builder.WriteString("<title>")
	builder.WriteString(htmlpkg.EscapeString(title))
	builder.WriteString("</title>")
	builder.WriteString("<meta property=\"og:title\" content=\"")
	builder.WriteString(htmlpkg.EscapeString(title))
	builder.WriteString("\" />")
	builder.WriteString("<meta property=\"og:url\" content=\"")
	builder.WriteString(htmlpkg.EscapeString(rawCurrentURL))
	builder.WriteString("\" />")
	if description != "" {
		builder.WriteString("<meta name=\"description\" content=\"")
		builder.WriteString(htmlpkg.EscapeString(description))
		builder.WriteString("\" />")
		builder.WriteString("<meta property=\"og:description\" content=\"")
		builder.WriteString(htmlpkg.EscapeString(description))
		builder.WriteString("\" />")
	}
	builder.WriteString("</head><body>")
	for _, paragraph := range paragraphs {
		trimmed := strings.TrimSpace(paragraph)
		if trimmed == "" {
			continue
		}

		builder.WriteString("<p>")
		builder.WriteString(htmlpkg.EscapeString(trimmed))
		builder.WriteString("</p>")
	}
	builder.WriteString("</body></html>")

	return builder.String()
}

func mergeLinks(existing []string, extra []string) []string {
	if len(extra) == 0 {
		return existing
	}

	linksSet := make(map[string]struct{}, len(existing)+len(extra))
	merged := make([]string, 0, len(existing)+len(extra))

	for _, link := range existing {
		if _, exists := linksSet[link]; exists {
			continue
		}
		linksSet[link] = struct{}{}
		merged = append(merged, link)
	}

	for _, link := range extra {
		if _, exists := linksSet[link]; exists {
			continue
		}
		linksSet[link] = struct{}{}
		merged = append(merged, link)
	}

	return merged
}

func mergeImages(existing map[string]map[string]string, extra map[string]map[string]string) map[string]map[string]string {
	if len(extra) == 0 {
		return existing
	}

	if existing == nil {
		existing = make(map[string]map[string]string)
	}

	for imageURL, attrs := range extra {
		if _, exists := existing[imageURL]; exists {
			continue
		}
		existing[imageURL] = attrs
	}

	return existing
}
