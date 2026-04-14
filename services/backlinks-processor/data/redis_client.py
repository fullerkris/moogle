import logging
import redis
import time

from typing import Optional, List
from models.backlinks import Backlinks

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

    # --------------------- BACKLINKS ---------------------
    def get_all_backlinks_keys(self) -> Optional[List[str]]:
        if self.client is None:
            logger.error(f"Redis connection not initialized")
            return

        logger.info("Fetching all backlinks' keys")
        res = self.client.keys("backlinks:*")
        if res is None:
            logger.info(f"No backlinks found")
            return None

        return res

    def get_all_backlinks(self, backlinks: List[str]) -> Optional[List[Backlinks]]:
        if self.client is None:
            logger.error(f"Redis connection not initialized")
            return None

        # Perform a batch operation
        backlinks_url = []
        pipeline = self.client.pipeline()
        for backlinks_id in backlinks:
            url = backlinks_id[10:]
            backlinks_url.append(url)
            pipeline.smembers(backlinks_id)

        res = pipeline.execute()
        if res is None:
            logger.info("Pipeline execution was empty")
            return None

        # Convert the results to Backlinks
        returned_backlinks = []
        for i, backlinks_result in enumerate(res):
            newLink = Backlinks(_id=backlinks_url[i], links=set(backlinks_result))
            returned_backlinks.append(newLink)

        return returned_backlinks

    def remove_all_backlinks(self, backlinks: List[str]) -> Optional[int]:
        if self.client is None:
            logger.error(f"Redis connection not initialized")
            return None

        pipeline = self.client.pipeline()
        for key in backlinks:
            pipeline.delete(key)

        res = pipeline.execute()
        if not res:
            return None

        deleted_entries = sum(res)
        return deleted_entries if deleted_entries > 0 else None

    # --------------------- BACKLINKS ---------------------
