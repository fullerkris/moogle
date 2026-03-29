package controllers

import (
	"net"
	"sync"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"

	"github.com/IonelPopJara/search-engine/services/spider/internal/crawler"
	"github.com/IonelPopJara/search-engine/services/spider/internal/database"
	"github.com/IonelPopJara/search-engine/services/spider/internal/pages"
	"github.com/IonelPopJara/search-engine/services/spider/internal/utils"
)

func TestSavePagesWritesPageDataAndIndexerQueue(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer redisServer.Close()

	host, port, err := net.SplitHostPort(redisServer.Addr())
	if err != nil {
		t.Fatalf("failed to parse redis address: %v", err)
	}

	db := &database.Database{}
	if err := db.ConnectToRedis(host, port, "", "0"); err != nil {
		t.Fatalf("failed to connect redis db: %v", err)
	}

	page := pages.CreatePage("example.com/docs", "<html>Hello</html>", "text/html", 200)
	crawcfg := &crawler.CrawlerConfig{
		Mu:    &sync.Mutex{},
		Pages: map[string]*pages.Page{page.NormalizedURL: page},
	}

	controller := NewPageController(db)
	controller.SavePages(crawcfg)

	pageKey := utils.PagePrefix + ":" + page.NormalizedURL
	if !redisServer.Exists(pageKey) {
		t.Fatalf("expected page hash key %q to exist", pageKey)
	}

	queueItems, err := db.Client.LRange(db.Context, utils.IndexerQueueKey, 0, -1).Result()
	if err != nil {
		t.Fatalf("failed to read indexer queue: %v", err)
	}

	if len(queueItems) != 1 {
		t.Fatalf("expected exactly one indexer queue item, got %d", len(queueItems))
	}

	if queueItems[0] != pageKey {
		t.Fatalf("expected queue item %q, got %q", pageKey, queueItems[0])
	}
}
