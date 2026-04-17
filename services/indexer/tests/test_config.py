import os
import unittest
from unittest.mock import patch

import config as indexer_config


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
                "REDIS_HOST": "should-not-be-used",
            },
            clear=True,
        ):
            config = indexer_config.get_redis_config(LoggerStub())

        self.assertEqual(config, {"redis_url": "redis://redis:6379/0"})

    def test_redis_invalid_port_raises_value_error(self):
        with patch.dict(
            os.environ,
            {
                "REDIS_HOST": "redis",
                "REDIS_PORT": "abc",
            },
            clear=True,
        ):
            with self.assertRaisesRegex(ValueError, "REDIS_PORT"):
                indexer_config.get_redis_config(LoggerStub())

    def test_mongo_invalid_port_raises_value_error(self):
        with patch.dict(
            os.environ,
            {
                "MONGO_HOST": "mongo",
                "MONGO_PORT": "bad-port",
                "MONGO_DB": "test",
            },
            clear=True,
        ):
            with self.assertRaisesRegex(ValueError, "MONGO_PORT"):
                indexer_config.get_mongo_config()


if __name__ == "__main__":
    unittest.main()
