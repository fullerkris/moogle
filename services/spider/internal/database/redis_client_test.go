package database

import (
	"net"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"

	"github.com/IonelPopJara/search-engine/services/spider/internal/utils"
)

func newTestDatabase(t *testing.T) (*Database, *miniredis.Miniredis) {
	t.Helper()

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	host, port, err := net.SplitHostPort(redisServer.Addr())
	if err != nil {
		redisServer.Close()
		t.Fatalf("failed to parse redis address: %v", err)
	}

	db := &Database{}
	if err := db.ConnectToRedis(host, port, "", "0"); err != nil {
		redisServer.Close()
		t.Fatalf("failed to connect test db: %v", err)
	}

	return db, redisServer
}

func TestPushURLDedupAndRawMapping(t *testing.T) {
	db, redisServer := newTestDatabase(t)
	defer redisServer.Close()

	if err := db.PushURL("http://example.com/docs?a=1#section", 0); err != nil {
		t.Fatalf("unexpected push error: %v", err)
	}

	if err := db.PushURL("http://example.com/docs", 1); err != nil {
		t.Fatalf("unexpected dedup push error: %v", err)
	}

	queueSize, err := db.Client.ZCard(db.Context, utils.SpiderQueueKey).Result()
	if err != nil {
		t.Fatalf("failed to read spider queue size: %v", err)
	}
	if queueSize != 1 {
		t.Fatalf("expected spider queue size 1, got %d", queueSize)
	}

	rawURL, score, normalizedURL, err := db.PopURL()
	if err != nil {
		t.Fatalf("unexpected pop error: %v", err)
	}

	if rawURL != "http://example.com/docs" {
		t.Fatalf("expected raw fetch URL http://example.com/docs, got %q", rawURL)
	}

	if normalizedURL != "example.com/docs" {
		t.Fatalf("expected normalized URL example.com/docs, got %q", normalizedURL)
	}

	if score != 0 {
		t.Fatalf("expected depth score 0, got %v", score)
	}
}

func TestVisitPageAndSeenDedup(t *testing.T) {
	db, redisServer := newTestDatabase(t)
	defer redisServer.Close()

	if err := db.PushURL("https://example.org/path", 0); err != nil {
		t.Fatalf("unexpected push error: %v", err)
	}

	_, _, normalizedURL, err := db.PopURL()
	if err != nil {
		t.Fatalf("unexpected pop error: %v", err)
	}

	visited, err := db.HasURLBeenVisited(normalizedURL)
	if err != nil {
		t.Fatalf("unexpected visited lookup error: %v", err)
	}

	if visited {
		t.Fatalf("expected URL to be unvisited initially")
	}

	if err := db.VisitPage(normalizedURL); err != nil {
		t.Fatalf("unexpected visit error: %v", err)
	}

	visited, err = db.HasURLBeenVisited(normalizedURL)
	if err != nil {
		t.Fatalf("unexpected visited lookup error: %v", err)
	}

	if !visited {
		t.Fatalf("expected URL to be marked visited")
	}

	if err := db.PushURL("https://example.org/path", 10); err != nil {
		t.Fatalf("unexpected push error for already-seen URL: %v", err)
	}

	got, err := db.Client.ZCard(db.Context, utils.SpiderQueueKey).Result()
	if err != nil {
		t.Fatalf("failed to read spider queue size: %v", err)
	}

	if got != 0 {
		t.Fatalf("expected no re-enqueue for seen URL, got queue size %d", got)
	}
}
