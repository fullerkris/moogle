import logging
import signal
import time
import sys
import os

from config import get_mongo_config, get_redis_config
from data.redis_client import RedisClient
from data.mongo_client import MongoClient

# SETUP LOGGER
logger = logging.getLogger(__name__)
logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)

# SHUTDOWN
shutdown_flag = False


def handle_shutdown(signum, frame):
    global shutdown_flag
    logger.info("Termination signal received - shutting down...")
    shutdown_flag = True


signal.signal(signal.SIGTERM, handle_shutdown)
signal.signal(signal.SIGINT, handle_shutdown)


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
        sys.exit(1)

    # CONNECT TO MONGO
    mongo = MongoClient(**mongo_config)

    if not mongo.client or mongo.client is None:
        logger.error("Could not initialize Mongo...")
        logger.error("Exiting...")
        sys.exit(1)

    # PROCESSING LOOP
    while True:

        logger.info(f"Processing backlinks...")

        # Retrieve backlinks' keys from Redis
        backlinks_keys = redis.get_all_backlinks_keys()
        if backlinks_keys is None or len(backlinks_keys) == 0:
            logger.info("No backlinks to process - sleep...")
            for _ in range(10):
                if shutdown_flag:
                    logger.info("Service stopped.")
                    sys.exit(0)
                time.sleep(1)
            continue

        # Retrieve backlinks from Redis
        backlinks = redis.get_all_backlinks(backlinks_keys)
        if backlinks is None:
            logger.error("Could not fetch backlinks - retry")
            continue

        # Remove all backlinks
        logger.info(f"Removing backlinks from Redis...")
        res = redis.remove_all_backlinks(backlinks_keys)
        if res:
            logger.info(f"{res} backlinks removed from Redis!")

        mongo.save_all_backlinks(backlinks)

        for _ in range(10):
            if shutdown_flag:
                logger.info("Service stopped.")
                sys.exit(0)
            time.sleep(1)
        continue

    logger.info("Service stopped.")
