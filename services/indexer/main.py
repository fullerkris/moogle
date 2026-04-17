import logging
import signal
import sys
import os

from utils.constants import *
from config import get_mongo_config, get_redis_config
from data.redis_client import RedisClient
from data.mongo_client import MongoClient
from utils.utils import get_html_data, split_url

from collections import Counter

# SETUP LOGGER
logger = logging.getLogger(__name__)
logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)

# SHUTDOWN
running = True


def handle_exit(signum, frame):
    global running
    logger.info("Termination signal received - shutting down...")
    running = False

    # Perform final bulk operations regardless of the threshold
    logger.info("Performing final bulk operations...")
    mongo.create_words_bulk(create_words_entry_operations)
    mongo.create_metadata_bulk(create_metadata_operations)
    mongo.create_outlinks_bulk(create_outlinks_operations)

    sys.exit(0)


signal.signal(signal.SIGTERM, handle_exit)
signal.signal(signal.SIGINT, handle_exit)


if __name__ == "__main__":
    try:
        redis_config = get_redis_config(logger)
        mongo_config = get_mongo_config()
    except ValueError as e:
        logger.error(f"Invalid startup configuration: {e}")
        sys.exit(1)

    # CONNECT TO REDIS
    logger.info("Initializing Redis...")
    redis = RedisClient(**redis_config)

    if not redis.client or redis.client is None:
        logger.error("Could not initialize Redis...")
        logger.error("Exiting...")
        exit(1)

    # CONNECT TO MONGO
    logger.info("Initializing Mongo...")
    mongo = MongoClient(**mongo_config)

    if not mongo.client or mongo.client is None:
        logger.error("Could not initialize Mongo...")
        logger.error("Exiting...")
        exit(1)

    # Define thresholds for batch operations
    WORDS_OP_THRESHOLD = 1000
    METADATA_OP_THRESHOLD = 100
    OUTLINKS_OP_THRESHOLD = 100

    # Initialize operation buffers
    create_words_entry_operations = []
    create_metadata_operations = []
    create_outlinks_operations = []

    # Function to perform bulk operations when thresholds are met
    def perform_bulk_operations():
        global create_words_entry_operations, create_metadata_operations, create_outlinks_operations

        if len(create_words_entry_operations) >= WORDS_OP_THRESHOLD:
            logger.info("Performing words bulk operations...")
            mongo.create_words_bulk(create_words_entry_operations)
            create_words_entry_operations = []

        if len(create_metadata_operations) >= METADATA_OP_THRESHOLD:
            logger.info("Performing metadata bulk operations...")
            mongo.create_metadata_bulk(create_metadata_operations)
            create_metadata_operations = []

        if len(create_outlinks_operations) >= OUTLINKS_OP_THRESHOLD:
            logger.info("Performing outlinks bulk operations...")
            mongo.create_outlinks_bulk(create_outlinks_operations)
            create_outlinks_operations = []

    # INDEXING LOOP
    while running:
        queue_size = redis.get_queue_size()
        if queue_size == 0:
            redis.signal_crawler()
            logger.info(f"RESUME_CRAWL signal sent")

        logger.info(f"Waiting for message queue...")

        # Get the next page from the queue
        page_id = redis.pop_page()
        if not page_id:
            logger.error("Could not fetch data from indexer queue")
            continue

        # Fetch page data
        logger.info(f"Fetching {page_id}...")
        page = redis.get_page_data(page_id)
        if page is None:
            logger.warning(f"Could not fetch {page_id}. Skipping...")
            continue

        logger.info(f"Page url: {page.normalized_url}")
        logger.info(
            f'Page html: {page.html[:15] + "..." if len(page.html) > 15 else page.html}'
        )

        normalized_url = page.normalized_url

        logger.info(f"Getting {page_id} metadata...")
        old_metadata = mongo.get_metadata(normalized_url)
        if old_metadata and old_metadata.last_crawled == page.last_crawled:
            logger.info(f"No updates to {old_metadata._id}. Skipping...")
            continue

        logger.info(f"Parsing html data for {page_id}...")
        html_data = get_html_data(page.html)
        if not html_data:
            logger.error(f"Could not parse html data for {page_id}. Skipping...")
            continue

        logger.info(f"Parsed html data for {page_id}...")
        if html_data["language"] != "en":
            logger.info(f"{page_id} not english. Skipping...")
            continue

        text = html_data["text"]
        if not text:
            logger.error(f"Could not process text {page_id}. Skipping...")
            continue

        # Make a dictionary with the words in the text and their frequency
        logger.info(f"Counting words from {page_id}...")
        words_frequency = Counter(text)

        # Get the top MAX_INDEX_WORDS words
        keywords = dict(words_frequency.most_common(MAX_INDEX_WORDS))

        logger.info(f"Check words in url {normalized_url}...")
        # Iterate through the url name to add more frequency to some words
        words_in_url = split_url(normalized_url)
        for word in words_in_url:
            past_score = keywords.get(word, 0)

            # If a word in the url was already in our registry of words we multiply it
            if past_score != 0:
                new_score = past_score * 50
                keywords[word] = new_score
            else:
                # If the word is not in our registry we add it with a score of 100
                keywords[word] = 10

        # Iterate through the images and add them to the word operations
        for word, frequency in keywords.items():
            word_op = mongo.create_words_entry_operation(
                word, normalized_url, frequency
            )
            create_words_entry_operations.append(word_op)

        # Save the metadata
        metadata_op = mongo.create_metadata_entry_operation(page, html_data, keywords)
        create_metadata_operations.append(metadata_op)

        # Save the outlinks
        outlinks = redis.get_outlinks(normalized_url)
        outlinks_op = mongo.create_outlinks_entry_operation(outlinks)
        create_outlinks_operations.append(outlinks_op)

        # Store all the words in the dictionary
        wordsSet = {word.lower() for word in text}
        mongo.add_words_to_dictionary(wordsSet)
        logger.info(f"Added words to dictionary...")

        logger.info("Delete page data from redis...")
        redis.delete_page_data(page_id)
        redis.delete_outlinks(normalized_url)

        logger.info("Pushing to image indexer queue...")
        redis.push_to_image_indexer_queue(normalized_url)

        # Check if any thresholds are exceeded and perform bulk operations
        perform_bulk_operations()

    # Save all remaining operations regardless of threshold
    logger.info("Final bulk operations before exit...")
    mongo.create_metadata_bulk(create_metadata_operations)
    mongo.create_outlinks_bulk(create_outlinks_operations)
    mongo.create_metadata_bulk(create_metadata_operations)

    logger.info("Shutting down...")

    sys.exit(0)
