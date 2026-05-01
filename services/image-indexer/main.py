import logging
import json
import signal
import sys
import os
import time
import threading

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from utils.constants import *
from config import get_mongo_config, get_redis_config
from data.redis_client import RedisClient
from data.mongo_client import MongoClient
from utils.utils import is_valid_image, split_name

import os.path

from concurrent.futures import ThreadPoolExecutor

# SETUP LOGGER
logger = logging.getLogger(__name__)
logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)

# SHUTDOWN
running = True
health_state_lock = threading.Lock()
health_state = {
    "startup_complete": False,
    "shutdown_requested": False,
}


def _health_timestamp():
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def _write_json(handler, status_code, payload):
    body = json.dumps(payload).encode("utf-8")
    handler.send_response(status_code)
    handler.send_header("Content-Type", "application/json")
    handler.send_header("Content-Length", str(len(body)))
    handler.end_headers()
    handler.wfile.write(body)


def start_health_server(port, service_name, redis_client, mongo_client):
    class HealthHandler(BaseHTTPRequestHandler):
        def do_GET(self):
            if self.path == "/health/live":
                _write_json(
                    self,
                    200,
                    {
                        "status": "up",
                        "service": service_name,
                        "timestamp": _health_timestamp(),
                    },
                )
                return

            if self.path == "/health/ready":
                with health_state_lock:
                    startup_complete = health_state["startup_complete"]
                    shutdown_requested = health_state["shutdown_requested"]

                dependencies = {
                    "startup_complete": startup_complete,
                    "shutdown_requested": not shutdown_requested,
                    "pipeline_redis": redis_client.ping(),
                    "mongodb": mongo_client.ping(),
                }

                is_ready = all(dependencies.values())
                _write_json(
                    self,
                    200 if is_ready else 503,
                    {
                        "status": "ready" if is_ready else "not_ready",
                        "service": service_name,
                        "dependencies": dependencies,
                        "timestamp": _health_timestamp(),
                    },
                )
                return

            self.send_response(404)
            self.end_headers()
            self.wfile.write(b"not found")

        def log_message(self, format, *args):
            return

    server = ThreadingHTTPServer(("0.0.0.0", port), HealthHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    logger.info("Health server listening on port %d", port)
    return server


def handle_exit(signum, frame):
    global running
    logger.info("Termination signal received - shutting down...")
    running = False
    with health_state_lock:
        health_state["shutdown_requested"] = True

    # Perform final bulk operations regardless of the threshold
    logger.info("Performing final bulk operations...")
    mongo.create_word_images_bulk(create_word_images_entry_operations)
    mongo.create_images_bulk(create_images_entry_operations)

    sys.exit(0)


signal.signal(signal.SIGTERM, handle_exit)
signal.signal(signal.SIGINT, handle_exit)


if __name__ == "__main__":
    health_port = int(os.getenv("IMAGE_INDEXER_HEALTH_PORT", "2115"))
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
    mongo = MongoClient(**mongo_config)

    if not mongo.client or mongo.client is None:
        logger.error("Could not initialize Mongo...")
        logger.error("Exiting...")
        exit(1)

    # Define thresholds for batch operations
    WORD_IMAGES_OP_THRESHOLD = 500
    IMAGES_OP_THRESHOLD = 100

    # Initialize operation buffers
    create_word_images_entry_operations = []
    create_images_entry_operations = []

    start_health_server(health_port, "image-indexer", redis, mongo)
    with health_state_lock:
        health_state["startup_complete"] = True

    # Function to perform bulk operations when thresholds are met
    def perform_bulk_operations():
        global create_word_images_entry_operations, create_images_entry_operations

        logger.info(
            "Word images operations: %d", len(create_word_images_entry_operations)
        )
        logger.info("Image operations: %d", len(create_images_entry_operations))

        if len(create_word_images_entry_operations) >= WORD_IMAGES_OP_THRESHOLD:
            logger.info("Performing word images bulk operations...")
            mongo.create_word_images_bulk(create_word_images_entry_operations)
            create_word_images_entry_operations = []

        if len(create_images_entry_operations) >= IMAGES_OP_THRESHOLD:
            logger.info("Performing save image bulk operations...")
            mongo.create_images_bulk(create_images_entry_operations)
            create_images_entry_operations = []

    # INDEXING LOOP
    while running:
        logger.info(f"Checking for message queue...")
        # Get the next image from the queue
        page_id = redis.pop_image()
        # page_id = redis.peek_page()
        if not page_id:
            logger.error("Could not fetch data from indexer queue")
            continue

        logger.info(f"Got {page_id} from queue...")

        # Fetch keywords from Mongo
        mongo_id = page_id.split("page_images:")[-1]
        keywords = mongo.get_keywords(mongo_id)

        logger.info(f"Got {len(keywords)} keywords from Mongo...")

        # Get the members of the page_id
        page_images = redis.get_page_images(page_id)
        if page_images is None:
            logger.warning(f"Could not fetch {page_id} images. Skipping...")
            continue

        logger.info(f"Page images: {page_images}")

        if len(page_images) == 0:
            logger.warning(f"No images found for {page_id}. Skipping...")
            # continue
        logger.info(f"Got {len(page_images)} images from Redis...")

        # Check if the urls are valid using workers
        logger.info("Processing images...")

        def process_image_url(image_url):
            logger.info(f"Processing {image_url}...")
            if is_valid_image(image_url):
                # Check file extension
                _, file_extension = os.path.splitext(image_url.split("/")[-1])
                file_extension = file_extension.lstrip(".")

                # Skip SVG files and icons
                if file_extension == "svg" or "icons" in image_url:
                    # Delete image from Redis
                    redis.delete_image_data(image_url)
                    return None

                # Fetch image data from redis
                image_data = redis.pop_image_data(image_url)
                if not image_data:
                    logger.error(f"Could not fetch image {image_url} from Redis")
                    return None

                # Update image data with filename
                image_data.filename = image_url.split("/")[-1]

                # Extract words from the image name
                filename_words_op = []
                words_in_filename = split_name(image_data.filename)
                for word in words_in_filename:
                    if word in keywords:
                        # Get the score of the word
                        past_score = keywords[word]
                        new_score = past_score * 100
                    else:
                        new_score = 30

                    # Create operation to add image URL to word images
                    # image_op = mongo.add_url_to_word_images_operation(
                    #     word, image_url, new_score
                    # )
                    image_op = mongo.create_word_images_entry_operation(
                        word, image_url, new_score
                    )
                    filename_words_op.append(image_op)

                # Add save image operation to the buffer
                save_image_op = mongo.create_image_operation(image_data)
                return (image_url, save_image_op, filename_words_op)
            else:
                # If the image is not valid, delete it from Redis
                logger.info(f"Deleting {image_url} from Redis - Not valid...")
                redis.delete_image_data(image_url)

                return None

        with ThreadPoolExecutor() as executor:
            # First parallel operation - processing image URLs
            results = list(executor.map(process_image_url, page_images))
            valid_results = [result for result in results if result is not None]

            # Unpack the results into image URLs and save image operations
            if valid_results:
                images_urls = []
                save_image_ops = []
                filename_words_ops_all = []

                for url, save_op, word_ops in valid_results:
                    images_urls.append(url)
                    save_image_ops.append(save_op)
                    filename_words_ops_all.extend(word_ops)

                create_images_entry_operations.extend(save_image_ops)
                create_word_images_entry_operations.extend(filename_words_ops_all)
            else:
                images_urls = []

            logger.info(f"Got {len(images_urls)} valid images from Redis...")

            # Second parallel operation - processing word-image operations
            # Create a list of all the (word, image_url, weight) combinations
            logger.info(f"Keywords: {keywords}")
            operations = [
                (word, image_url, weight)
                for word, weight in keywords.items()
                for image_url in images_urls
            ]

            # Define a function to process each operation
            def process_word_image(operation):
                word, image_url, weight = operation

                return mongo.create_word_images_entry_operation(word, image_url, weight)

                # return mongo.add_url_to_word_images_operation(word, image_url, weight)

            # Execute the operations in parallel and wait for completion
            keyword_ops = list(executor.map(process_word_image, operations))
            create_word_images_entry_operations.extend(keyword_ops)

        # Check if any thresholds are exceeded and perform bulk operations
        perform_bulk_operations()

        # Delete page_images
        # It's stupid to pass the mongo_id here but lol
        redis.delete_page_images(mongo_id)

    # Save all remaining operations regardless of threshold
    logger.info("Final bulk operations before exit...")
    # Save all remaining operations regardless of threshold
    mongo.create_word_images_bulk(create_word_images_entry_operations)
    mongo.create_images_bulk(create_images_entry_operations)

    logger.info("Shutting down...")

    sys.exit(0)
