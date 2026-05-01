import os


def _parse_int_env(name, value):
    try:
        return int(value)
    except (TypeError, ValueError):
        raise ValueError(f"{name} must be an integer, got {value!r}")


def get_redis_config(logger):
    pipeline_redis_url = os.getenv("PIPELINE_REDIS_URL", "").strip()
    if pipeline_redis_url:
        logger.info("Using PIPELINE_REDIS_URL for Redis connection")
        return {"redis_url": pipeline_redis_url}

    raise ValueError("PIPELINE_REDIS_URL is required")


def get_mongo_config():
    mongo_host = os.getenv("MONGO_HOST", "localhost")
    mongo_port_raw = os.getenv("MONGO_PORT", "27017")
    mongo_db = os.getenv("MONGO_DB", "test")

    return {
        "host": mongo_host,
        "port": _parse_int_env("MONGO_PORT", mongo_port_raw),
        "password": os.getenv("MONGO_PASSWORD", ""),
        "db": mongo_db,
        "username": os.getenv("MONGO_USERNAME", ""),
    }
