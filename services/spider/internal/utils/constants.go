package utils

import (
	"time"
)

const (
	// Crawler constants
	Timeout  = 5 * time.Second
	MaxScore = 10000
	MinScore = -1000

	// Frontier scoring constants
	FrontierDepthFractionBase = 0.05
	FrontierPenaltyScale      = 0.85
	FrontierDefaultPenalty    = 0.65
	FrontierTieBreakerScale   = 0.01
	FrontierTieBreakerBuckets = 1000

	DefaultHTTPTimeoutSeconds = 10
	DefaultHTTPMaxBodyBytes   = 2 * 1024 * 1024 // 2 MiB
	DefaultHTTPUserAgent      = "MoogleSpider/1.0 (+https://github.com/IonelPopJara/search-engine)"

	// FIXME: There is a weird "bug" where pages_queue starts appearing in redis even if it is not used in the code.
	// No idea why :/ and I don't have time to investigate it now.
	// Message Queues
	SpiderQueueKey      = "spider_queue"
	IndexerQueueKey     = "pages_queue"
	SignalQueueKey      = "signal_queue"
	SeenURLsKey         = "spider_seen_urls"
	VisitedURLsKey      = "spider_visited_urls"
	ResumeCrawl         = "RESUME_CRAWL"
	MaxIndexerQueueSize = 5000

	// Redis Data: some keys stay in Redis indefinitely, while others are transfer to MongoDB by other services
	NormalizedURLPrefix = "normalized_url" // Stays in Redis indefinitely
	PagePrefix          = "page_data"      // Transferred by the indexer
	ImagePrefix         = "image_data"     // Transferred by the image indexer
	PageImagesPrefix    = "page_images"    // Transferred by the image indexer
	BacklinksPrefix     = "backlinks"      // Transferred by the backlinks processor
	OutlinksPrefix      = "outlinks"       // Transferred by the indexer
)
