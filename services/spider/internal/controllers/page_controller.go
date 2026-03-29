package controllers

import (
	"log"

	"github.com/redis/go-redis/v9"

	"github.com/IonelPopJara/search-engine/services/spider/internal/crawler"
	"github.com/IonelPopJara/search-engine/services/spider/internal/database"
	"github.com/IonelPopJara/search-engine/services/spider/internal/pages"
	"github.com/IonelPopJara/search-engine/services/spider/internal/utils"
)

type PageController struct {
	db *database.Database
}

func NewPageController(db *database.Database) *PageController {
	return &PageController{
		db: db,
	}
}

func (pgc *PageController) GetAllPages() map[string]*pages.Page {
	log.Printf("Fetching data from Redis...\n")
	redisPages := make(map[string]*pages.Page)

	keys, err := pgc.db.Client.Keys(pgc.db.Context, utils.PagePrefix+":*").Result()
	if err != nil {
		log.Printf("Error fetching data from Redis: %v\n", err)
		return nil
	}

	// Process the redis data using a pipeline
	pipeline := pgc.db.Client.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(keys))

	for i, key := range keys {
		cmds[i] = pipeline.HGetAll(pgc.db.Context, key)
	}

	_, err = pipeline.Exec(pgc.db.Context)
	if err != nil {
		log.Printf("Error fetching data from Redis pipeline: %v", err)
		return nil
	}

	for _, cmd := range cmds {
		data, err := cmd.Result()
		if err != nil {
			log.Printf("Error fetching pipeline result: %v", err)
			return nil
		}

		page, err := pages.DehashPage(data)
		if err != nil {
			log.Printf("Error dehashing page from Redis: %v", err)
			return nil
		}

		redisPages[page.NormalizedURL] = page
	}

	return redisPages
}

func (pgc *PageController) SavePages(crawcfg *crawler.CrawlerConfig) {
	data := crawcfg.Pages
	log.Printf("Writing %d entries to the db...\n", len(data))

	// Process the redis entries using one transactional pipeline so
	// page_data writes and pages_queue publish happen atomically.
	pipeline := pgc.db.Client.TxPipeline()

	type pageWriteCmd struct {
		normalizedURL string
		hSetCmd       *redis.IntCmd
		lPushCmd      *redis.IntCmd
	}

	writeCmds := make([]pageWriteCmd, 0, len(data))

	for _, page := range data {
		pageHash, err := pages.HashPage(page)
		if err != nil {
			log.Printf("Error hashing page %s: %v", page.NormalizedURL, err)
			continue
		}

		pageKey := utils.PagePrefix + ":" + page.NormalizedURL
		writeCmds = append(writeCmds, pageWriteCmd{
			normalizedURL: page.NormalizedURL,
			hSetCmd:       pipeline.HSet(pgc.db.Context, pageKey, pageHash),
			lPushCmd:      pipeline.LPush(pgc.db.Context, utils.IndexerQueueKey, pageKey),
		})
	}

	if len(writeCmds) == 0 {
		log.Printf("No page entries queued for persistence")
		return
	}

	_, err := pipeline.Exec(pgc.db.Context)
	if err != nil {
		log.Printf("Error executing page persistence transaction: %v", err)
		return
	}

	for _, cmd := range writeCmds {
		if err := cmd.hSetCmd.Err(); err != nil {
			log.Printf("HSET failed for %s: %v", cmd.normalizedURL, err)
		}

		if err := cmd.lPushCmd.Err(); err != nil {
			log.Printf("LPUSH failed for %s: %v", cmd.normalizedURL, err)
		}
	}

	log.Printf("Successfully written %d entries to the db!", len(writeCmds))
}
