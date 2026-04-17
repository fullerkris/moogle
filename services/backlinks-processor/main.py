import logging
import signal
import time
import sys
import os

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


def _parse_int_env(name, value):
    try:
        return int(value)
    except (TypeError, ValueError):
        raise ValueError(f"{name} must be an integer, got {value!r}")


def get_redis_config():
    pipeline_redis_url = os.getenv("PIPELINE_REDIS_URL", "").strip()
    if pipeline_redis_url:
        logger.info("Using PIPELINE_REDIS_URL for Redis connection")
        return {"redis_url": pipeline_redis_url}

    redis_host = os.getenv("REDIS_HOST", "localhost")
    redis_port_raw = os.getenv("REDIS_PORT", "6379")

    redis_password = os.getenv("REDIS_PASSWORD", None)
    redis_db_raw = os.getenv("REDIS_DB", "0")

    logger.info("Using REDIS_HOST/REDIS_PORT fallback for Redis connection")
    return {
        "host": redis_host,
        "port": _parse_int_env("REDIS_PORT", redis_port_raw),
        "password": redis_password,
        "db": _parse_int_env("REDIS_DB", redis_db_raw),
    }


def get_mongo_config():
    mongo_host = os.getenv("MONGO_HOST", "localhost")
    mongo_port_raw = os.getenv("MONGO_PORT", "27017")
    mongo_db = os.getenv("MONGO_DB", "test")

    return {
        "host": mongo_host,
        "port": _parse_int_env("MONGO_PORT", mongo_port_raw),
        "password": os.getenv("MONGO_PASSWORD", None),
        "db": mongo_db,
        "username": os.getenv("MONGO_USERNAME", ""),
    }

if __name__ == "__main__":
    try:
        redis_config = get_redis_config()
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
