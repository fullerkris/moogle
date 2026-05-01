package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IonelPopJara/search-engine/services/spider/internal/controllers"
	"github.com/IonelPopJara/search-engine/services/spider/internal/crawler"
	"github.com/IonelPopJara/search-engine/services/spider/internal/database"
	"github.com/IonelPopJara/search-engine/services/spider/internal/pages"
	"github.com/IonelPopJara/search-engine/services/spider/internal/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type spiderHealthState struct {
	startupComplete atomic.Bool
}

func writeProbeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("Could not encode probe payload: %v", err)
	}
}

func startProbeServer(probeAddr string, metricsEnabled bool, db *database.Database, health *spiderHealthState) {
	mux := http.NewServeMux()

	if metricsEnabled {
		mux.Handle("/metrics", promhttp.Handler())
	}

	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		writeProbeJSON(w, http.StatusOK, map[string]any{
			"status":    "up",
			"service":   "spider",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		dependencies := map[string]bool{
			"startup_complete": health != nil && health.startupComplete.Load(),
			"pipeline_redis":  false,
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err == nil {
			dependencies["pipeline_redis"] = true
		}

		isReady := dependencies["startup_complete"] && dependencies["pipeline_redis"]
		statusCode := http.StatusOK
		status := "ready"
		if !isReady {
			statusCode = http.StatusServiceUnavailable
			status = "not_ready"
		}

		writeProbeJSON(w, statusCode, map[string]any{
			"status":       status,
			"service":      "spider",
			"dependencies": dependencies,
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
		})
	})

	go func() {
		log.Printf("Spider probe server listening on %s", probeAddr)
		if err := http.ListenAndServe(probeAddr, mux); err != nil {
			log.Printf("Spider probe server stopped: %v", err)
		}
	}()
}

// getEnv retrieves the value of an environment variable or returns a fallback value if not set.
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return fallback
}

func getEnvInt(key string, fallback int) int {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("Invalid %s value %q. Falling back to %d", key, value, fallback)
		return fallback
	}

	if parsed <= 0 {
		log.Printf("Non-positive %s value %d. Falling back to %d", key, parsed, fallback)
		return fallback
	}

	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}

	normalized := strings.TrimSpace(strings.ToLower(value))
	switch normalized {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		log.Printf("Invalid %s value %q. Falling back to %t", key, value, fallback)
		return fallback
	}
}

func getEnvStringList(key string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.ToLower(strings.TrimSpace(part))
		if trimmed == "" {
			continue
		}
		items = append(items, trimmed)
	}

	return items
}

func main() {
	// Parse flags
	maxConcurrency := flag.Int("max-concurrency", 10, "Maximum number of concurrent workers")
	maxPages := flag.Int("max-pages", 100, "Maximum number of pages per batch")
	flag.Parse()

	// Retrieve environment variables
	pipelineRedisURL := strings.TrimSpace(os.Getenv("PIPELINE_REDIS_URL"))
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	redisPassword := getEnv("REDIS_PASSWORD", "")
	redisDB := getEnv("REDIS_DB", "0")
	allowRedisFallback := getEnvBool("ALLOW_REDIS_HOST_FALLBACK", false)
	startingURL := getEnv("STARTING_URL", "https://en.wikipedia.org/wiki/Kamen_Rider")
	httpTimeoutSeconds := getEnvInt("SPIDER_HTTP_TIMEOUT_SECONDS", utils.DefaultHTTPTimeoutSeconds)
	httpMaxBodyBytes := getEnvInt("SPIDER_HTTP_MAX_BODY_BYTES", utils.DefaultHTTPMaxBodyBytes)
	httpUserAgent := getEnv("SPIDER_HTTP_USER_AGENT", utils.DefaultHTTPUserAgent)
	crawlMode := strings.ToLower(getEnv("SPIDER_CRAWL_MODE", string(crawler.CrawlModeOpen)))
	allowlistDomains := getEnvStringList("SPIDER_ALLOWLIST_DOMAINS")
	blocklistDomains := getEnvStringList("SPIDER_BLOCKLIST_DOMAINS")
	domainMatchSubdomains := getEnvBool("SPIDER_DOMAIN_MATCH_SUBDOMAINS", true)
	robotsEnabled := getEnvBool("SPIDER_ROBOTS_ENABLED", true)
	robotsCacheTTLSeconds := getEnvInt("SPIDER_ROBOTS_CACHE_TTL_SECONDS", 3600)
	defaultCrawlDelayMs := getEnvInt("SPIDER_DEFAULT_CRAWL_DELAY_MS", 1000)
	maxConcurrentPerDomain := getEnvInt("SPIDER_MAX_CONCURRENT_PER_DOMAIN", 1)
	robotsAllowOnFetchFailure := getEnvBool("SPIDER_ROBOTS_ALLOW_ON_FETCH_FAILURE", true)
	robotsBypassDomains := getEnvStringList("SPIDER_ROBOTS_BYPASS_DOMAINS")
	robotsBypassSubdomains := getEnvBool("SPIDER_ROBOTS_BYPASS_SUBDOMAINS", true)
	robotsBypassDelayMs := getEnvInt("SPIDER_ROBOTS_BYPASS_DELAY_MS", defaultCrawlDelayMs)
	metricsEnabled := getEnvBool("SPIDER_METRICS_ENABLED", true)
	metricsAddr := getEnv("SPIDER_METRICS_ADDR", ":2113")
	budgetEnabled := getEnvBool("SPIDER_BUDGET_ENABLED", true)
	runMaxAttempts := getEnvInt("SPIDER_BUDGET_RUN_MAX_ATTEMPTS", 50000)
	runMaxSuccesses := getEnvInt("SPIDER_BUDGET_RUN_MAX_SUCCESSES", 30000)
	domainMaxAttempts := getEnvInt("SPIDER_BUDGET_DOMAIN_MAX_ATTEMPTS", 2000)
	bypassMaxRun := getEnvInt("SPIDER_BUDGET_BYPASS_MAX_RUN", 200)
	bypassMaxDomain := getEnvInt("SPIDER_BUDGET_BYPASS_MAX_DOMAIN", 50)

	fetchClient := &http.Client{
		Timeout: time.Duration(httpTimeoutSeconds) * time.Second,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	crawler.SetFetchConfig(crawler.FetchConfig{
		Client:       fetchClient,
		UserAgent:    httpUserAgent,
		MaxBodyBytes: int64(httpMaxBodyBytes),
	})

	policyManager := crawler.NewFetchPolicyManager(crawler.FetchPolicyConfig{
		Client:    fetchClient,
		UserAgent: httpUserAgent,
		Domain: crawler.DomainPolicyConfig{
			Mode:            crawler.CrawlMode(crawlMode),
			Allowlist:       allowlistDomains,
			Blocklist:       blocklistDomains,
			MatchSubdomains: domainMatchSubdomains,
		},
		Robots: crawler.RobotsConfig{
			Enabled:                robotsEnabled,
			CacheTTL:               time.Duration(robotsCacheTTLSeconds) * time.Second,
			DefaultCrawlDelay:      time.Duration(defaultCrawlDelayMs) * time.Millisecond,
			MaxConcurrentPerDomain: maxConcurrentPerDomain,
			AllowOnFetchFailure:    robotsAllowOnFetchFailure,
			BypassDomains:          robotsBypassDomains,
			BypassSubdomains:       robotsBypassSubdomains,
			BypassDelay:            time.Duration(robotsBypassDelayMs) * time.Millisecond,
		},
	})

	budgetManager := crawler.NewCrawlBudgetManager(crawler.BudgetConfig{
		Enabled:           budgetEnabled,
		RunMaxAttempts:    runMaxAttempts,
		RunMaxSuccesses:   runMaxSuccesses,
		DomainMaxAttempts: domainMaxAttempts,
		BypassMaxRun:      bypassMaxRun,
		BypassMaxDomain:   bypassMaxDomain,
	})

	var spiderMetrics *crawler.SpiderMetrics
	if metricsEnabled {
		spiderMetrics = crawler.NewSpiderMetrics(prometheus.DefaultRegisterer)
	}

	// Connect to Redis
	db := &database.Database{}
	health := &spiderHealthState{}
	var err error
	if pipelineRedisURL != "" {
		err = db.ConnectToRedisURL(pipelineRedisURL)
	} else {
		if !allowRedisFallback {
			log.Println("PIPELINE_REDIS_URL is required. Set ALLOW_REDIS_HOST_FALLBACK=true to use REDIS_HOST/REDIS_PORT fallback")
			return
		}
		log.Println("PIPELINE_REDIS_URL not set, falling back to REDIS_HOST/REDIS_PORT")
		err = db.ConnectToRedis(redisHost, redisPort, redisPassword, redisDB)
	}
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	startProbeServer(metricsAddr, metricsEnabled, db, health)

	// Add an entry to the message queue with score 0 (high priority)
	if err := db.PushURL(startingURL, 0); err != nil {
		log.Printf("Could not seed starting URL %q: %v", startingURL, err)
	} else {
		log.Printf("PUSH %v\n", startingURL)
	}
	health.startupComplete.Store(true)

	// Instantiate controllers
	pageController := controllers.NewPageController(db)
	linksController := controllers.NewLinksController(db)
	imageController := controllers.NewImageController(db)

	// Instantiate crawler
	crawler := &crawler.CrawlerConfig{
		Mu:             &sync.Mutex{},
		Wg:             &sync.WaitGroup{},
		Pages:          make(map[string]*pages.Page),
		Outlinks:       make(map[string]*pages.PageNode),
		Backlinks:      make(map[string]*pages.PageNode),
		Images:         make(map[string][]*pages.Image),
		MaxPages:       *maxPages,
		MaxConcurrency: *maxConcurrency,
		Policy:         policyManager,
		Budget:         budgetManager,
		Metrics:        spiderMetrics,
	}

	// Infinite loop to crawl the web in batches
	for {
		// Check how busy the indexer queue is
		log.Printf("Checking number of entries...\n")
		// If we have reached the maximum number of entries in the spider queue
		queueSize, err := db.GetIndexerQueueSize()
		if err != nil {
			log.Printf("Error getting indexer queue: %v\n", err)
			return
		}

		if queueSize >= utils.MaxIndexerQueueSize {
			resumeThreshold := int64(utils.MaxIndexerQueueSize - 500)
			log.Printf("Indexer queue is full (%d). Waiting for it to drain below %d...\n", queueSize, resumeThreshold)
			for queueSize >= resumeThreshold {
				time.Sleep(5 * time.Second)
				queueSize, err = db.GetIndexerQueueSize()
				if err != nil {
					log.Printf("Error getting indexer queue while waiting: %v\n", err)
					return
				}
			}
			log.Printf("Indexer queue drained to %d. Resuming crawl.\n", queueSize)
		}

		log.Printf("Spawning workers...\n")
		for range crawler.MaxConcurrency {
			crawler.Wg.Add(1)
			go crawler.Crawl(db)
		}

		crawler.Wg.Wait()

		// Write entries to the db
		pageController.SavePages(crawler)
		linksController.SaveLinks(crawler)
		imageController.SaveImages(crawler)

		// Clean visited pages by this runner
		crawler.Pages = make(map[string]*pages.Page)
		crawler.Outlinks = make(map[string]*pages.PageNode)
		crawler.Backlinks = make(map[string]*pages.PageNode)
		crawler.Images = make(map[string][]*pages.Image)
	}
}
