import os
import unittest
from unittest.mock import patch

import config as image_indexer_config


class LoggerStub:
    @staticmethod
    def info(*_args, **_kwargs):
        return None


class ConfigParsingTests(unittest.TestCase):
    def test_redis_uses_pipeline_url_when_available(self):
        with patch.dict(
            os.environ,
            {
                "PIPELINE_REDIS_URL": "redis://redis:6379/0",
                "REDIS_HOST": "ignored-host",
            },
            clear=True,
        ):
            config = image_indexer_config.get_redis_config(LoggerStub())

        self.assertEqual(config, {"redis_url": "redis://redis:6379/0"})

    def test_redis_invalid_db_raises_value_error(self):
        with patch.dict(
            os.environ,
            {
                "REDIS_HOST": "redis",
                "REDIS_PORT": "6379",
                "REDIS_DB": "invalid-db",
            },
            clear=True,
        ):
            with self.assertRaisesRegex(ValueError, "REDIS_DB"):
                image_indexer_config.get_redis_config(LoggerStub())

    def test_mongo_invalid_port_raises_value_error(self):
        with patch.dict(
            os.environ,
            {
                "MONGO_HOST": "mongo",
                "MONGO_PORT": "invalid-port",
                "MONGO_DB": "test",
            },
            clear=True,
        ):
            with self.assertRaisesRegex(ValueError, "MONGO_PORT"):
                image_indexer_config.get_mongo_config()


if __name__ == "__main__":
    unittest.main()
