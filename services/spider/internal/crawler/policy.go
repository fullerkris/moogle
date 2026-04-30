package crawler

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxRobotsBodyBytes = 1 * 1024 * 1024

type CrawlMode string

const (
	CrawlModeOpen      CrawlMode = "open"
	CrawlModeAllowlist CrawlMode = "allowlist"
)

type DomainPolicyConfig struct {
	Mode            CrawlMode
	Allowlist       []string
	Blocklist       []string
	MatchSubdomains bool
}

type RobotsConfig struct {
	Enabled                bool
	CacheTTL               time.Duration
	DefaultCrawlDelay      time.Duration
	MaxConcurrentPerDomain int
	AllowOnFetchFailure    bool
	BypassDomains          []string
	BypassSubdomains       bool
	BypassDelay            time.Duration
}

type FetchPolicyConfig struct {
	Domain    DomainPolicyConfig
	Robots    RobotsConfig
	Client    *http.Client
	UserAgent string
}

type FetchDecision struct {
	Allowed              bool
	Reason               string
	Host                 string
	Delay                time.Duration
	MatchedBypassDomain  string
	BypassSubdomainMatch bool
}

type FetchPolicyManager struct {
	domainConfig DomainPolicyConfig
	robotsConfig RobotsConfig
	client       *http.Client
	userAgent    string

	mu           sync.Mutex
	robotsByHost map[string]robotsCacheEntry
	hostState    map[string]hostPolitenessState
}

type robotsCacheEntry struct {
	fetchedAt time.Time
	expiresAt time.Time
	rules     robotsRules
	fetchErr  error
}

type hostPolitenessState struct {
	nextAllowedAt time.Time
	inFlight      int
}

type robotsRules struct {
	groups []robotsGroup
}

type robotsGroup struct {
	userAgents []string
	rules      []robotsRule
	crawlDelay time.Duration
}

type robotsRule struct {
	allow bool
	path  string
}

func NewFetchPolicyManager(config FetchPolicyConfig) *FetchPolicyManager {
	manager := &FetchPolicyManager{
		domainConfig: config.Domain,
		robotsConfig: config.Robots,
		client:       config.Client,
		userAgent:    config.UserAgent,
		robotsByHost: make(map[string]robotsCacheEntry),
		hostState:    make(map[string]hostPolitenessState),
	}

	manager.applyDefaults()

	return manager
}

func (m *FetchPolicyManager) applyDefaults() {
	if m.client == nil {
		m.client = &http.Client{Timeout: 10 * time.Second}
	}

	if strings.TrimSpace(m.userAgent) == "" {
		m.userAgent = "MoogleSpider/1.0"
	}

	if m.domainConfig.Mode == "" {
		m.domainConfig.Mode = CrawlModeOpen
	}

	if m.robotsConfig.CacheTTL <= 0 {
		m.robotsConfig.CacheTTL = 1 * time.Hour
	}

	if m.robotsConfig.DefaultCrawlDelay <= 0 {
		m.robotsConfig.DefaultCrawlDelay = 1 * time.Second
	}

	if m.robotsConfig.MaxConcurrentPerDomain <= 0 {
		m.robotsConfig.MaxConcurrentPerDomain = 1
	}

	if m.robotsConfig.BypassDelay <= 0 {
		m.robotsConfig.BypassDelay = m.robotsConfig.DefaultCrawlDelay
	}
}

func (m *FetchPolicyManager) Evaluate(rawURL string) (FetchDecision, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return FetchDecision{}, fmt.Errorf("failed to parse URL %q: %w", rawURL, err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return FetchDecision{Allowed: false, Reason: "invalid_scheme"}, nil
	}

	host := strings.ToLower(strings.TrimSpace(parsedURL.Hostname()))
	if host == "" {
		return FetchDecision{Allowed: false, Reason: "missing_host"}, nil
	}

	if !m.isHostAllowedByDomainPolicy(host) {
		return FetchDecision{Allowed: false, Reason: "domain_policy_denied", Host: host}, nil
	}

	bypassMatch, matchedDomain, subdomainMatch := matchDomain(host, m.robotsConfig.BypassDomains, m.robotsConfig.BypassSubdomains)
	if bypassMatch {
		return FetchDecision{
			Allowed:              true,
			Reason:               "robots_bypass",
			Host:                 host,
			Delay:                m.robotsConfig.BypassDelay,
			MatchedBypassDomain:  matchedDomain,
			BypassSubdomainMatch: subdomainMatch,
		}, nil
	}

	if !m.robotsConfig.Enabled {
		return FetchDecision{
			Allowed: true,
			Reason:  "robots_disabled",
			Host:    host,
			Delay:   m.robotsConfig.DefaultCrawlDelay,
		}, nil
	}

	rules, fetchErr := m.getRobotsRules(parsedURL, host)
	if fetchErr != nil {
		if !m.robotsConfig.AllowOnFetchFailure {
			return FetchDecision{Allowed: false, Reason: "robots_fetch_failed", Host: host}, nil
		}

		return FetchDecision{
			Allowed: true,
			Reason:  "robots_fetch_failed_allow",
			Host:    host,
			Delay:   m.robotsConfig.DefaultCrawlDelay,
		}, nil
	}

	path := parsedURL.EscapedPath()
	if path == "" {
		path = "/"
	}

	if !rules.isAllowed(m.userAgent, path) {
		return FetchDecision{Allowed: false, Reason: "robots_disallow", Host: host}, nil
	}

	delay := rules.crawlDelay(m.userAgent)
	if delay <= 0 {
		delay = m.robotsConfig.DefaultCrawlDelay
	}

	return FetchDecision{Allowed: true, Reason: "allowed", Host: host, Delay: delay}, nil
}

func (m *FetchPolicyManager) Acquire(host string, delay time.Duration) func() {
	if host == "" {
		return func() {}
	}

	if delay <= 0 {
		delay = m.robotsConfig.DefaultCrawlDelay
	}

	maxConcurrent := m.robotsConfig.MaxConcurrentPerDomain
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	for {
		m.mu.Lock()

		state := m.hostState[host]
		now := time.Now()

		if state.inFlight < maxConcurrent && (state.nextAllowedAt.IsZero() || !now.Before(state.nextAllowedAt)) {
			state.inFlight++
			state.nextAllowedAt = now.Add(delay)
			m.hostState[host] = state
			m.mu.Unlock()

			return func() {
				m.mu.Lock()
				defer m.mu.Unlock()

				current := m.hostState[host]
				if current.inFlight > 0 {
					current.inFlight--
				}
				m.hostState[host] = current
			}
		}

		waitFor := 50 * time.Millisecond
		if now.Before(state.nextAllowedAt) {
			waitFor = state.nextAllowedAt.Sub(now)
			if waitFor > 1*time.Second {
				waitFor = 1 * time.Second
			}
		}

		m.mu.Unlock()
		time.Sleep(waitFor)
	}
}

func (m *FetchPolicyManager) isHostAllowedByDomainPolicy(host string) bool {
	if matchesAnyDomain(host, m.domainConfig.Blocklist, m.domainConfig.MatchSubdomains) {
		return false
	}

	if m.domainConfig.Mode != CrawlModeAllowlist {
		return true
	}

	if len(m.domainConfig.Allowlist) == 0 {
		return false
	}

	return matchesAnyDomain(host, m.domainConfig.Allowlist, m.domainConfig.MatchSubdomains)
}

func matchesAnyDomain(host string, domains []string, includeSubdomains bool) bool {
	matched, _, _ := matchDomain(host, domains, includeSubdomains)
	return matched
}

func matchDomain(host string, domains []string, includeSubdomains bool) (bool, string, bool) {
	normalizedHost := strings.ToLower(strings.TrimSpace(host))
	if normalizedHost == "" {
		return false, "", false
	}

	for _, domain := range domains {
		normalizedDomain := strings.ToLower(strings.TrimSpace(domain))
		if normalizedDomain == "" {
			continue
		}

		if normalizedHost == normalizedDomain {
			return true, normalizedDomain, false
		}

		if includeSubdomains && strings.HasSuffix(normalizedHost, "."+normalizedDomain) {
			return true, normalizedDomain, true
		}
	}

	return false, "", false
}

func (m *FetchPolicyManager) getRobotsRules(parsedURL *url.URL, host string) (robotsRules, error) {
	now := time.Now()

	m.mu.Lock()
	entry, exists := m.robotsByHost[host]
	if exists && now.Before(entry.expiresAt) {
		m.mu.Unlock()
		return entry.rules, entry.fetchErr
	}
	m.mu.Unlock()

	robotsURL := &url.URL{
		Scheme: parsedURL.Scheme,
		Host:   parsedURL.Host,
		Path:   "/robots.txt",
	}

	request, err := http.NewRequest(http.MethodGet, robotsURL.String(), nil)
	if err != nil {
		return robotsRules{}, fmt.Errorf("failed to build robots request: %w", err)
	}
	request.Header.Set("User-Agent", m.userAgent)

	response, err := m.client.Do(request)
	if err != nil {
		cache := robotsCacheEntry{
			fetchedAt: now,
			expiresAt: now.Add(m.robotsConfig.CacheTTL),
			fetchErr:  err,
		}

		m.mu.Lock()
		m.robotsByHost[host] = cache
		m.mu.Unlock()

		return robotsRules{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		statusErr := fmt.Errorf("robots fetch returned status %d", response.StatusCode)
		cache := robotsCacheEntry{
			fetchedAt: now,
			expiresAt: now.Add(m.robotsConfig.CacheTTL),
			fetchErr:  statusErr,
		}

		m.mu.Lock()
		m.robotsByHost[host] = cache
		m.mu.Unlock()

		return robotsRules{}, statusErr
	}

	limitedReader := io.LimitReader(response.Body, maxRobotsBodyBytes)
	body, readErr := io.ReadAll(limitedReader)
	if readErr != nil {
		cache := robotsCacheEntry{
			fetchedAt: now,
			expiresAt: now.Add(m.robotsConfig.CacheTTL),
			fetchErr:  readErr,
		}

		m.mu.Lock()
		m.robotsByHost[host] = cache
		m.mu.Unlock()

		return robotsRules{}, readErr
	}

	parsedRules := parseRobotsTxt(string(body))
	cache := robotsCacheEntry{
		fetchedAt: now,
		expiresAt: now.Add(m.robotsConfig.CacheTTL),
		rules:     parsedRules,
	}

	m.mu.Lock()
	m.robotsByHost[host] = cache
	m.mu.Unlock()

	return parsedRules, nil
}

func parseRobotsTxt(content string) robotsRules {
	var groups []robotsGroup
	var current robotsGroup
	var hasCurrent bool
	var sawRules bool

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if hash := strings.Index(line, "#"); hash >= 0 {
			line = strings.TrimSpace(line[:hash])
			if line == "" {
				continue
			}
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		directive := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch directive {
		case "user-agent":
			if !hasCurrent || sawRules {
				if hasCurrent {
					groups = append(groups, current)
				}
				current = robotsGroup{}
				hasCurrent = true
				sawRules = false
			}
			current.userAgents = append(current.userAgents, strings.ToLower(value))
		case "allow", "disallow":
			if !hasCurrent {
				continue
			}
			sawRules = true
			current.rules = append(current.rules, robotsRule{allow: directive == "allow", path: value})
		case "crawl-delay":
			if !hasCurrent {
				continue
			}
			sawRules = true
			if parsedDelay, err := strconv.ParseFloat(value, 64); err == nil && parsedDelay >= 0 {
				current.crawlDelay = time.Duration(parsedDelay * float64(time.Second))
			}
		}
	}

	if hasCurrent {
		groups = append(groups, current)
	}

	return robotsRules{groups: groups}
}

func (r robotsRules) isAllowed(userAgent string, path string) bool {
	selectedGroups := r.matchingGroups(userAgent)
	if len(selectedGroups) == 0 {
		return true
	}

	bestMatchLength := -1
	bestMatchAllow := true

	for _, group := range selectedGroups {
		for _, rule := range group.rules {
			if rule.path == "" {
				continue
			}

			if strings.HasPrefix(path, rule.path) {
				pathLen := len(rule.path)
				if pathLen > bestMatchLength {
					bestMatchLength = pathLen
					bestMatchAllow = rule.allow
				} else if pathLen == bestMatchLength && rule.allow {
					bestMatchAllow = true
				}
			}
		}
	}

	if bestMatchLength == -1 {
		return true
	}

	return bestMatchAllow
}

func (r robotsRules) crawlDelay(userAgent string) time.Duration {
	selectedGroups := r.matchingGroups(userAgent)
	for _, group := range selectedGroups {
		if group.crawlDelay > 0 {
			return group.crawlDelay
		}
	}

	return 0
}

func (r robotsRules) matchingGroups(userAgent string) []robotsGroup {
	if len(r.groups) == 0 {
		return nil
	}

	normalizedAgent := strings.ToLower(userAgent)
	var exact []robotsGroup
	var wildcard []robotsGroup

	for _, group := range r.groups {
		matchedExact := false
		matchedWildcard := false

		for _, token := range group.userAgents {
			normalizedToken := strings.ToLower(strings.TrimSpace(token))
			if normalizedToken == "" {
				continue
			}

			if normalizedToken == "*" {
				matchedWildcard = true
				continue
			}

			if strings.Contains(normalizedAgent, normalizedToken) {
				matchedExact = true
			}
		}

		if matchedExact {
			exact = append(exact, group)
			continue
		}

		if matchedWildcard {
			wildcard = append(wildcard, group)
		}
	}

	if len(exact) > 0 {
		return exact
	}

	return wildcard
}
