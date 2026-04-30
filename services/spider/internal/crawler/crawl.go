package crawler

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/IonelPopJara/search-engine/services/spider/internal/database"
	"github.com/IonelPopJara/search-engine/services/spider/internal/pages"
	"github.com/IonelPopJara/search-engine/services/spider/internal/utils"
)

// BFS crawling
func (crawcfg *CrawlerConfig) Crawl(db *database.Database) {
	// Starting a new webcrawler instance
	defer crawcfg.Wg.Done()

	// BFS loop
	for {
		log.Printf("Crawling...\n")
		// Check if we have reached the maximum number of pages
		if crawcfg.maxPagesReached() {
			log.Printf("Maximum number of pages reached\n")
			return
		}

		// Get the next URL from the queue
		log.Printf("Waiting for message queue...\n")
		rawCurrentURL, depthLevel, normalizedCurrentURL, err := db.PopURL()
		if err != nil {
			log.Printf("No more URLs in the queue: %v\n", err)
			return
		}

		log.Printf("Popped URL: %v | Depth Level: %v | Normalized URL: %v\n", rawCurrentURL, depthLevel, normalizedCurrentURL)

		// time.Sleep(1 * time.Second)

		// Check if the URL has been visited
		visited, err := db.HasURLBeenVisited(normalizedCurrentURL)
		if err != nil {
			log.Printf("Error: [%v] - skipping...\n", err)
			continue
		}

		if visited {
			log.Printf("Skipping %v - already visited\n", normalizedCurrentURL)
			continue
		}

		html, statusCode, contentType, err := crawcfg.fetchPage(rawCurrentURL, normalizedCurrentURL)
		if err != nil {
			log.Printf("Error fetching %v data: %v\n", rawCurrentURL, err)
			continue
		}

		err = crawcfg.processFetchedPage(db, normalizedCurrentURL, rawCurrentURL, depthLevel, html, statusCode, contentType)
		if err != nil {
			log.Printf("Error processing %v: %v\n", rawCurrentURL, err)
			continue
		}
	}
}

func (crawcfg *CrawlerConfig) fetchPage(rawCurrentURL string, normalizedCurrentURL string) (string, int, string, error) {
	log.Printf("Crawling from %v (%v)...\n", normalizedCurrentURL, rawCurrentURL)

	if crawcfg.Policy == nil {
		return getPageData(rawCurrentURL)
	}

	decision, err := crawcfg.Policy.Evaluate(rawCurrentURL)
	if err != nil {
		crawcfg.observeFetch("policy_error", 0, 0)
		return "", 0, "", fmt.Errorf("policy evaluation failed: %w", err)
	}

	crawcfg.observePolicyDecision(decision.Allowed, decision.Reason)

	if !decision.Allowed {
		log.Printf("Policy denied url=%s host=%s reason=%s", rawCurrentURL, decision.Host, decision.Reason)
		crawcfg.observeFetch("policy_denied", 0, 0)
		return "", 0, "", fmt.Errorf("policy denied (%s)", decision.Reason)
	}

	bypassed := decision.Reason == "robots_bypass"
	if bypassed {
		log.Printf(
			"event=robots_bypass_applied host=%s matched_domain=%s subdomain_match=%t delay_ms=%d url=%s",
			decision.Host,
			decision.MatchedBypassDomain,
			decision.BypassSubdomainMatch,
			decision.Delay.Milliseconds(),
			rawCurrentURL,
		)
		crawcfg.observeBypass(true, "policy")
	}

	if crawcfg.Budget != nil {
		budgetDecision := crawcfg.Budget.AllowFetch(decision.Host, bypassed)
		if !budgetDecision.Allowed {
			if bypassed {
				crawcfg.observeBypass(false, budgetDecision.Reason)
			}
			crawcfg.observeBudgetDenied(budgetDecision.Reason)
			crawcfg.observePolicyDecision(false, budgetDecision.Reason)
			crawcfg.observeFetch("budget_denied", 0, 0)
			return "", 0, "", fmt.Errorf("budget denied (%s)", budgetDecision.Reason)
		}
	}

	release := crawcfg.Policy.Acquire(decision.Host, decision.Delay)
	startedAt := time.Now()
	html, statusCode, contentType, err := getPageData(rawCurrentURL)
	release()
	crawcfg.observeFetch(classifyFetchResult(statusCode, err), statusCode, time.Since(startedAt))

	if crawcfg.Budget != nil {
		crawcfg.Budget.AccountFetch(err == nil)
		crawcfg.observeBudgetRemaining("run_attempts", crawcfg.Budget.RemainingRunAttempts())
		crawcfg.observeBudgetRemaining("run_successes", crawcfg.Budget.RemainingRunSuccesses())
	}

	return html, statusCode, contentType, err
}

func (crawcfg *CrawlerConfig) processFetchedPage(
	db *database.Database,
	normalizedCurrentURL string,
	rawCurrentURL string,
	depthLevel float64,
	html string,
	statusCode int,
	contentType string,
) error {
	outgoingLinks, imagesMap, err := getURLsFromHTML(html, rawCurrentURL)
	if err != nil {
		return fmt.Errorf("error getting links from HTML: %w", err)
	}

	crawcfg.AddImages(normalizedCurrentURL, imagesMap)
	crawcfg.UpdateLinks(normalizedCurrentURL, outgoingLinks)

	pg := pages.CreatePage(normalizedCurrentURL, html, contentType, statusCode)

	err = crawcfg.addPage(pg)
	if err != nil {
		return fmt.Errorf("error adding page visit: %w", err)
	}

	err = db.VisitPage(normalizedCurrentURL)
	if err != nil {
		return fmt.Errorf("error marking page visited: %w", err)
	}

	log.Printf("Adding links from %v (%v)...\n", normalizedCurrentURL, rawCurrentURL)
	for _, rawCurrentLink := range outgoingLinks {
		if !utils.IsValidURL(rawCurrentLink) {
			continue
		}

		scoreDetails := computeFrontierScore(depthLevel, rawCurrentLink)
		score := scoreDetails.Score

		score = math.Max(utils.MinScore, math.Min(score, utils.MaxScore))

		log.Printf("Frontier score url=%s depth=%.0f penalty=%.3f tie=%.4f score=%.4f", rawCurrentLink, scoreDetails.Depth, scoreDetails.Penalty, scoreDetails.TieBreaker, score)

		enqueueResult, err := db.PushURLWithResult(rawCurrentLink, score)
		if err != nil {
			crawcfg.observeEnqueue("rejected", "enqueue_error")
			log.Printf("Error enqueueing %v with score %.4f: %v", rawCurrentLink, score, err)
			continue
		}

		switch enqueueResult {
		case database.EnqueueAccepted:
			crawcfg.observeEnqueue("accepted", "ok")
		case database.EnqueueDuplicate:
			crawcfg.observeEnqueue("rejected", "duplicate")
		case database.EnqueueQueueFull:
			crawcfg.observeEnqueue("rejected", "queue_full")
		default:
			crawcfg.observeEnqueue("rejected", "unknown")
		}
	}

	return nil
}

func classifyFetchResult(statusCode int, err error) string {
	if err == nil {
		return "success"
	}

	lowerErr := strings.ToLower(err.Error())
	if strings.Contains(lowerErr, "timeout") || strings.Contains(lowerErr, "deadline exceeded") {
		return "timeout"
	}

	if statusCode >= 400 {
		return "http_error"
	}

	return "fetch_error"
}

func (crawcfg *CrawlerConfig) observeFetch(result string, statusCode int, duration time.Duration) {
	if crawcfg.Metrics == nil {
		return
	}

	crawcfg.Metrics.ObserveFetch(result, statusCode, duration)
}

func (crawcfg *CrawlerConfig) observeEnqueue(result string, reason string) {
	if crawcfg.Metrics == nil {
		return
	}

	crawcfg.Metrics.ObserveEnqueue(result, reason)
}

func (crawcfg *CrawlerConfig) observePolicyDecision(allowed bool, reason string) {
	if crawcfg.Metrics == nil {
		return
	}

	crawcfg.Metrics.ObservePolicyDecision(allowed, reason)
}

func (crawcfg *CrawlerConfig) observeBypass(granted bool, reason string) {
	if crawcfg.Metrics == nil {
		return
	}

	crawcfg.Metrics.ObserveBypass(granted, reason)
}

func (crawcfg *CrawlerConfig) observeBudgetDenied(reason string) {
	if crawcfg.Metrics == nil {
		return
	}

	crawcfg.Metrics.ObserveBudgetDenied(reason)
}

func (crawcfg *CrawlerConfig) observeBudgetRemaining(scope string, value int) {
	if crawcfg.Metrics == nil {
		return
	}

	crawcfg.Metrics.SetBudgetRemaining(scope, value)
}
