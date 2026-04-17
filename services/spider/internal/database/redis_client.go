package database

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/IonelPopJara/search-engine/services/spider/internal/utils"
)

var enqueueIfUnseenScript = redis.NewScript(`
local seenKey = KEYS[1]
local queueKey = KEYS[2]
local urlLookupKey = KEYS[3]

local normalizedURL = ARGV[1]
local rawURL = ARGV[2]
local score = tonumber(ARGV[3])

if redis.call("SISMEMBER", seenKey, normalizedURL) == 1 then
    return 0
end

redis.call("SADD", seenKey, normalizedURL)
redis.call("HSET", urlLookupKey, "raw_url", rawURL, "visited", 0)
redis.call("ZADD", queueKey, score, normalizedURL)

return 1
`)

// ------------------- REDIS SETUP -------------------
type Database struct {
	Client  *redis.Client
	Context context.Context
}

func (db *Database) connectRedis(options *redis.Options) error {
	db.Client = redis.NewClient(options)
	db.Context = context.Background()

	_, err := db.Client.Ping(db.Context).Result()
	if err != nil {
		return fmt.Errorf("could not connect to redis at %q: %w", options.Addr, err)
	}

	log.Println("Successfully connected to Redis!")
	return nil
}

func (db *Database) ConnectToRedis(redisHost, redisPort, redisPassword, redisDB string) error {
	log.Println("Connecting to Redis...")

	dbIndex, err := strconv.Atoi(redisDB)
	if err != nil {
		return fmt.Errorf("could not parse DB value: %w", err)
	}

	return db.connectRedis(&redis.Options{
		Addr:     redisHost + ":" + redisPort,
		Password: redisPassword,
		DB:       dbIndex,
	})
}

func (db *Database) ConnectToRedisURL(redisURL string) error {
	log.Println("Connecting to Redis using PIPELINE_REDIS_URL...")

	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return fmt.Errorf("could not parse PIPELINE_REDIS_URL: %w", err)
	}

	return db.connectRedis(options)
}

// ------------------- REDIS SETUP -------------------

// ------------------- CRAWL LINKS -------------------
func lookupKey(normalizedURL string) string {
	return utils.NormalizedURLPrefix + ":" + normalizedURL
}

func (db *Database) PushURL(rawURL string, score float64) error {
	// Remove fragments and queries from rawURL
	rawURL, err := utils.StripURL(rawURL)
	if err != nil {
		return fmt.Errorf("Could not strip URL: %w", err)
	}

	// Normalize URL
	normalizedURL, err := utils.NormalizeURL(rawURL)
	if err != nil {
		return fmt.Errorf("Could not normalize URL: %w", err)
	}

	res, err := enqueueIfUnseenScript.Run(
		db.Context,
		db.Client,
		[]string{utils.SeenURLsKey, utils.SpiderQueueKey, lookupKey(normalizedURL)},
		normalizedURL,
		rawURL,
		score,
	).Int()

	if err != nil {
		return fmt.Errorf("Could not add URL to queue: %w", err)
	}

	if res == 0 {
		return nil
	}

	fmt.Printf("Pushed %v (%v) to queue\n", rawURL, normalizedURL)

	return nil
}

func (db *Database) ExistsInQueue(rawURL string) (float64, bool) {
	rawURL, err := utils.StripURL(rawURL)
	if err != nil {
		return 0.0, false
	}

	// Normalize URL
	normalizedURL, err := utils.NormalizeURL(rawURL)
	if err != nil {
		return 0.0, false
	}

	result, err := db.Client.ZScore(db.Context, utils.SpiderQueueKey, normalizedURL).Result()
	if err != nil {
		return 0.0, false
	}

	return result, true
}

// ------------------- CRAWL LINKS -------------------

// ------------------- VISIT PAGE -------------------
func (db *Database) HasURLBeenVisited(normalizedURL string) (bool, error) {
	visited, err := db.Client.SIsMember(db.Context, utils.VisitedURLsKey, normalizedURL).Result()
	if err != nil {
		return false, fmt.Errorf("Could not fetch visit marker for %v: %w", normalizedURL, err)
	}

	return visited, nil
}

func (db *Database) VisitPage(normalizedURL string) error {
	pipeline := db.Client.TxPipeline()
	pipeline.SAdd(db.Context, utils.VisitedURLsKey, normalizedURL)
	pipeline.HSet(db.Context, lookupKey(normalizedURL), "visited", 1)

	if _, err := pipeline.Exec(db.Context); err != nil {
		return fmt.Errorf("Could not mark %v as visited: %w", normalizedURL, err)
	}

	return nil
}

// ------------------- VISIT PAGE -------------------

// ------------------- GET NEXT ENTRY -------------------
func (db *Database) PopURL() (string, float64, string, error) {
	// Get the next normalized URL from the priority queue
	result, err := db.Client.BZPopMin(db.Context, utils.Timeout, utils.SpiderQueueKey).Result()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unknown command") {
			fallbackResult, fallbackErr := db.Client.ZPopMin(db.Context, utils.SpiderQueueKey, 1).Result()
			if fallbackErr != nil {
				return "", 0.0, "", fmt.Errorf("Could not pop URL from queue (fallback): %v", fallbackErr)
			}

			if len(fallbackResult) == 0 {
				return "", 0.0, "", fmt.Errorf("Could not pop URL from queue: empty queue")
			}

			normalizedURL := fallbackResult[0].Member.(string)
			rawURL, lookupErr := db.Client.HGet(db.Context, lookupKey(normalizedURL), "raw_url").Result()
			if lookupErr != nil {
				if lookupErr == redis.Nil {
					return "", 0.0, "", fmt.Errorf("Missing raw URL mapping for %v", normalizedURL)
				}

				return "", 0.0, "", fmt.Errorf("Could not fetch raw URL for %v: %w", normalizedURL, lookupErr)
			}

			return rawURL, fallbackResult[0].Score, normalizedURL, nil
		}

		return "", 0.0, "", fmt.Errorf("Could not pop URL from queue: %v", err)
	}

	// Format the proper Redis queue to fetch data
	// normalized_url_key := fmt.Sprintf("%v:%v", utils.NormalizedURLPrefix, result.Z.Member)

	// Fetch the raw url from Redis
	// raw_url, err := db.Client.HGet(db.Context, normalized_url_key, "raw_url").Result()
	// if err != nil {
	// return "", 0.0, "", fmt.Errorf("Could not fetch raw URL from %v: %v\n", normalized_url_key, err)
	// }

	normalizedURL := result.Z.Member.(string)
	rawURL, err := db.Client.HGet(db.Context, lookupKey(normalizedURL), "raw_url").Result()
	if err != nil {
		if err == redis.Nil {
			return "", 0.0, "", fmt.Errorf("Missing raw URL mapping for %v", normalizedURL)
		}

		return "", 0.0, "", fmt.Errorf("Could not fetch raw URL for %v: %w", normalizedURL, err)
	}

	return rawURL, result.Z.Score, normalizedURL, nil
}

func (db *Database) PopSignalQueue() (string, error) {
	result, err := db.Client.BRPop(db.Context, 0, utils.SignalQueueKey).Result()
	if err != nil {
		return "", fmt.Errorf("Could not pop from signal queue: %v\n", err)
	}

	return result[1], nil
}

func (db *Database) GetIndexerQueueSize() (int64, error) {
	size, err := db.Client.LLen(db.Context, utils.IndexerQueueKey).Result()
	if err != nil {
		return -1, fmt.Errorf("Could not get %v size: %v\n", utils.IndexerQueueKey, err)
	}

	return size, nil
}

// ------------------- GET NEXT ENTRY -------------------
