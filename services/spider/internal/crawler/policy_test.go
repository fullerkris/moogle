package crawler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPolicyDomainModeAllowlist(t *testing.T) {
	manager := NewFetchPolicyManager(FetchPolicyConfig{
		Domain: DomainPolicyConfig{
			Mode:            CrawlModeAllowlist,
			Allowlist:       []string{"example.com"},
			Blocklist:       []string{"blocked.example.com"},
			MatchSubdomains: true,
		},
		Robots: RobotsConfig{Enabled: false},
	})

	allowedDecision, err := manager.Evaluate("https://docs.example.com/path")
	if err != nil {
		t.Fatalf("unexpected error for allowlisted host: %v", err)
	}
	if !allowedDecision.Allowed {
		t.Fatalf("expected allowlisted host to be allowed, got reason=%s", allowedDecision.Reason)
	}

	blockedDecision, err := manager.Evaluate("https://blocked.example.com/path")
	if err != nil {
		t.Fatalf("unexpected error for blocked host: %v", err)
	}
	if blockedDecision.Allowed {
		t.Fatalf("expected blocked host to be denied")
	}

	deniedDecision, err := manager.Evaluate("https://other.net/path")
	if err != nil {
		t.Fatalf("unexpected error for non-allowlisted host: %v", err)
	}
	if deniedDecision.Allowed {
		t.Fatalf("expected non-allowlisted host to be denied")
	}
}

func TestPolicyRespectsRobotsDisallow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /private\nCrawl-delay: 7\n"))
			return
		}

		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	manager := NewFetchPolicyManager(FetchPolicyConfig{
		Client: server.Client(),
		Domain: DomainPolicyConfig{Mode: CrawlModeOpen},
		Robots: RobotsConfig{
			Enabled:                true,
			CacheTTL:               10 * time.Second,
			DefaultCrawlDelay:      1 * time.Second,
			MaxConcurrentPerDomain: 1,
			AllowOnFetchFailure:    true,
		},
		UserAgent: "MoogleSpider/1.0",
	})

	privateDecision, err := manager.Evaluate(server.URL + "/private/area")
	if err != nil {
		t.Fatalf("unexpected error evaluating private URL: %v", err)
	}
	if privateDecision.Allowed {
		t.Fatalf("expected robots-disallowed path to be denied")
	}

	publicDecision, err := manager.Evaluate(server.URL + "/public/area")
	if err != nil {
		t.Fatalf("unexpected error evaluating public URL: %v", err)
	}
	if !publicDecision.Allowed {
		t.Fatalf("expected public path to be allowed, got reason=%s", publicDecision.Reason)
	}
	if publicDecision.Delay != 7*time.Second {
		t.Fatalf("expected crawl-delay of 7s, got %s", publicDecision.Delay)
	}
}

func TestPolicyBypassSkipsRobotsRules(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /\nCrawl-delay: 60\n"))
			return
		}

		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	hostname := ""
	request, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}
	hostname = request.URL.Hostname()

	manager := NewFetchPolicyManager(FetchPolicyConfig{
		Client: server.Client(),
		Domain: DomainPolicyConfig{Mode: CrawlModeOpen},
		Robots: RobotsConfig{
			Enabled:                true,
			CacheTTL:               10 * time.Second,
			DefaultCrawlDelay:      1 * time.Second,
			MaxConcurrentPerDomain: 1,
			AllowOnFetchFailure:    true,
			BypassDomains:          []string{hostname},
			BypassSubdomains:       true,
			BypassDelay:            3 * time.Second,
		},
		UserAgent: "MoogleSpider/1.0",
	})

	decision, evalErr := manager.Evaluate(server.URL + "/private/area")
	if evalErr != nil {
		t.Fatalf("unexpected error evaluating bypassed URL: %v", evalErr)
	}
	if !decision.Allowed {
		t.Fatalf("expected bypassed host to be allowed, got reason=%s", decision.Reason)
	}
	if decision.Delay != 3*time.Second {
		t.Fatalf("expected bypass delay of 3s, got %s", decision.Delay)
	}
}

func TestPolicyRobotsFetchFailureCanDeny(t *testing.T) {
	host := "127.0.0.1:1"
	manager := NewFetchPolicyManager(FetchPolicyConfig{
		Client: &http.Client{Timeout: 200 * time.Millisecond},
		Domain: DomainPolicyConfig{Mode: CrawlModeOpen},
		Robots: RobotsConfig{
			Enabled:                true,
			CacheTTL:               5 * time.Second,
			DefaultCrawlDelay:      1 * time.Second,
			MaxConcurrentPerDomain: 1,
			AllowOnFetchFailure:    false,
		},
		UserAgent: "MoogleSpider/1.0",
	})

	decision, err := manager.Evaluate(fmt.Sprintf("http://%s/path", host))
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected robots fetch failure policy to deny request")
	}
}

func TestPolicyBlocklistOverridesBypass(t *testing.T) {
	manager := NewFetchPolicyManager(FetchPolicyConfig{
		Domain: DomainPolicyConfig{
			Mode:            CrawlModeOpen,
			Blocklist:       []string{"example.com"},
			MatchSubdomains: true,
		},
		Robots: RobotsConfig{
			Enabled:                true,
			DefaultCrawlDelay:      1 * time.Second,
			MaxConcurrentPerDomain: 1,
			AllowOnFetchFailure:    true,
			BypassDomains:          []string{"example.com"},
			BypassSubdomains:       true,
			BypassDelay:            2 * time.Second,
		},
	})

	decision, err := manager.Evaluate("https://www.example.com/private")
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}

	if decision.Allowed {
		t.Fatalf("expected blocklist to override bypass")
	}

	if decision.Reason != "domain_policy_denied" {
		t.Fatalf("expected domain_policy_denied, got %s", decision.Reason)
	}
}

func TestPolicyBypassDecisionIncludesMatchMetadata(t *testing.T) {
	manager := NewFetchPolicyManager(FetchPolicyConfig{
		Domain: DomainPolicyConfig{Mode: CrawlModeOpen},
		Robots: RobotsConfig{
			Enabled:                true,
			DefaultCrawlDelay:      1 * time.Second,
			MaxConcurrentPerDomain: 1,
			AllowOnFetchFailure:    true,
			BypassDomains:          []string{"reddit.com"},
			BypassSubdomains:       true,
			BypassDelay:            1500 * time.Millisecond,
		},
	})

	decision, err := manager.Evaluate("https://www.reddit.com/r/programming")
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}

	if !decision.Allowed || decision.Reason != "robots_bypass" {
		t.Fatalf("expected bypass decision, got allowed=%t reason=%s", decision.Allowed, decision.Reason)
	}

	if decision.MatchedBypassDomain != "reddit.com" {
		t.Fatalf("expected matched bypass domain reddit.com, got %s", decision.MatchedBypassDomain)
	}

	if !decision.BypassSubdomainMatch {
		t.Fatalf("expected subdomain bypass match metadata")
	}

	if decision.Delay != 1500*time.Millisecond {
		t.Fatalf("expected bypass delay metadata to be propagated, got %s", decision.Delay)
	}
}

func TestPolicyAllowlistSubdomainToggle(t *testing.T) {
	manager := NewFetchPolicyManager(FetchPolicyConfig{
		Domain: DomainPolicyConfig{
			Mode:            CrawlModeAllowlist,
			Allowlist:       []string{"example.com"},
			MatchSubdomains: false,
		},
		Robots: RobotsConfig{Enabled: false},
	})

	exact, err := manager.Evaluate("https://example.com/docs")
	if err != nil {
		t.Fatalf("unexpected evaluation error for exact domain: %v", err)
	}
	if !exact.Allowed {
		t.Fatalf("expected exact allowlisted domain to be allowed")
	}

	subdomain, err := manager.Evaluate("https://docs.example.com/path")
	if err != nil {
		t.Fatalf("unexpected evaluation error for subdomain: %v", err)
	}
	if subdomain.Allowed {
		t.Fatalf("expected subdomain to be denied when MatchSubdomains=false")
	}
}
