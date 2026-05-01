import logging
import json
import signal
import time
import sys
import os
import threading

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

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


def handle_shutdown(signum, frame):
    global shutdown_flag
    logger.info("Termination signal received - shutting down...")
    shutdown_flag = True
    with health_state_lock:
        health_state["shutdown_requested"] = True


signal.signal(signal.SIGTERM, handle_shutdown)
signal.signal(signal.SIGINT, handle_shutdown)


if __name__ == "__main__":
    health_port = int(os.getenv("BACKLINKS_PROCESSOR_HEALTH_PORT", "2116"))
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

    start_health_server(health_port, "backlinks-processor", redis, mongo)
    with health_state_lock:
        health_state["startup_complete"] = True

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
