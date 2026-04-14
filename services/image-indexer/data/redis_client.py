import time
import redis
import logging
from utils.constants import *

from typing import Optional, List
from models.image import Image

# SETUP LOGGER
logger = logging.getLogger(__name__)
logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)


class RedisClient:
    def __init__(
        self,
        host="localhost",
        port=6379,
        password="",
        db=0,
        decode_responses=True,
        redis_url=None,
    ):
        try:
            if redis_url:
                self.client = redis.Redis.from_url(
                    redis_url,
                    decode_responses=decode_responses,
                )
            else:
                self.client = redis.Redis(
                    host=host,
                    port=port,
                    password=password,
                    db=db,
                    decode_responses=decode_responses,
                )

            self.client.ping()
            logger.info("Successfully connected to redis!")
        except Exception as e:
            logger.error(f"Failed to connect to redis: {e}")
            self.client = None

    # --------------------- MESSAGE QUEUE ---------------------
    def pop_image(self) -> Optional[str]:
        try:
            popped = self.client.brpop(IMAGE_INDEXER_QUEUE_KEY)
            if not popped:
                logger.warning(f"Could not fetch from message queue")
                return None

            _, page_id = popped
            return page_id
        except Exception as e:
            logger.error(f"Could not fetch from message queue: {e}")
            return None

    def peek_page(self) -> Optional[str]:
        try:
            peeked = self.client.lrange(IMAGE_INDEXER_QUEUE_KEY, -1, -1)
            if not peeked:
                logger.warning(f"Could not peek from message queue")
                return None

            page_id = peeked[0]
            logger.debug(f"Peeked from message queue: {page_id}")
            return page_id
        except Exception as e:
            logger.error(f"Could not peek from message queue: {e}")
            return None

    def get_queue_size(self) -> Optional[int]:
        if self.client is None:
            logger.error(f"Redis connection not initialized")
            return
        return self.client.llen(IMAGE_INDEXER_QUEUE_KEY)

    # --------------------- PAGE IMAGES ---------------------
    def get_page_images(self, normalized_url: str) -> Optional[List[str]]:
        key = f"{PAGE_IMAGES_PREFIX}:{normalized_url}"
        page_images_urls = self.client.smembers(key)

        if not page_images_urls:
            return []

        return [url for url in page_images_urls]

    def delete_page_images(self, normalized_url: str) -> None:
        if self.client is None:
            logger.error(f"Redis connection not initialized")
            return

        key = f"{PAGE_IMAGES_PREFIX}:{normalized_url}"
        res = self.client.delete(key)
        if res <= 0:
            logger.error(f"Could not remove {key} from Redis")

    # # --------------------- PAGE IMAGES ---------------------

    # # --------------------- IMAGES ---------------------
    # Get image and delete it from Redis
    def pop_image_data(self, image_url: str) -> Optional[Image]:
        key = f"{IMAGE_PREFIX}:{image_url}"
        image_hashed = self.client.hgetall(key)

        if not image_hashed:
            logger.error(f"Could not fetch image {key} from Redis")
            return None

        # res = self.client.delete(key)
        # if res <= 0:
        #     logger.error(f'Could not remove {key} from Redis')

        return Image.from_hash(image_hashed, image_url)

    def delete_image_data(self, image_url: str) -> None:
        if self.client is None:
            logger.error(f"Redis connection not initialized")
            return

        key = f"{IMAGE_PREFIX}:{image_url}"
        res = self.client.delete(key)
        if res <= 0:
            logger.error(f"Could not remove {key} from Redis: {res}")

    # # --------------------- IMAGES ---------------------
