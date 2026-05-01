import os
import json
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

import redis


PIPELINE_REDIS_URL = os.getenv("PIPELINE_REDIS_URL", "redis://pipeline-redis:6379/0")
QUERY_REDIS_URL = os.getenv("QUERY_REDIS_URL", "redis://query-redis:6379/0")
METRICS_PORT = int(os.getenv("EXPORTER_PORT", "9108"))
SCRAPE_INTERVAL_SECONDS = int(os.getenv("SCRAPE_INTERVAL_SECONDS", "10"))

INDEXER_QUEUE_KEY = os.getenv("INDEXER_QUEUE_KEY", "pages_queue")
INDEXER_QUEUE_AGE_KEY = os.getenv("INDEXER_QUEUE_AGE_KEY", "pages_queue_enqueued_at")

state_lock = threading.Lock()
state = {
    "exporter_up": 0,
    "queue_depth": 0,
    "queue_oldest_age_seconds": 0,
    "redis_memory_used_bytes": {
        "pipeline": 0,
        "query": 0,
    },
    "redis_memory_max_bytes": {
        "pipeline": 0,
        "query": 0,
    },
    "backup_last_success_timestamp_seconds": None,
    "last_collection_success_timestamp_seconds": None,
    "last_collection_error": "",
}


def _get_client(redis_url):
    return redis.Redis.from_url(redis_url, decode_responses=True)


def _to_float(value, fallback=0.0):
    try:
        return float(value)
    except (TypeError, ValueError):
        return fallback


def _collect_metrics():
    pipeline = _get_client(PIPELINE_REDIS_URL)
    query = _get_client(QUERY_REDIS_URL)

    queue_depth = int(pipeline.llen(INDEXER_QUEUE_KEY))

    oldest_entry = pipeline.zrange(INDEXER_QUEUE_AGE_KEY, 0, 0, withscores=True)
    if oldest_entry:
        oldest_timestamp = _to_float(oldest_entry[0][1], fallback=0)
        oldest_age_seconds = max(0.0, time.time() - oldest_timestamp)
    else:
        oldest_age_seconds = 0.0

    pipeline_info = pipeline.info("memory")
    query_info = query.info("memory")

    pipeline_used = _to_float(pipeline_info.get("used_memory"), fallback=0)
    pipeline_max = _to_float(pipeline_info.get("maxmemory"), fallback=0)
    if pipeline_max <= 0:
        pipeline_max = _to_float(pipeline_info.get("total_system_memory"), fallback=1)

    query_used = _to_float(query_info.get("used_memory"), fallback=0)
    query_max = _to_float(query_info.get("maxmemory"), fallback=0)
    if query_max <= 0:
        query_max = _to_float(query_info.get("total_system_memory"), fallback=1)

    backup_ts_raw = pipeline.get("backup_last_success_timestamp_seconds")
    backup_ts = _to_float(backup_ts_raw, fallback=0) if backup_ts_raw else None

    with state_lock:
        state["exporter_up"] = 1
        state["queue_depth"] = queue_depth
        state["queue_oldest_age_seconds"] = oldest_age_seconds
        state["redis_memory_used_bytes"]["pipeline"] = pipeline_used
        state["redis_memory_used_bytes"]["query"] = query_used
        state["redis_memory_max_bytes"]["pipeline"] = pipeline_max
        state["redis_memory_max_bytes"]["query"] = query_max
        state["backup_last_success_timestamp_seconds"] = backup_ts


def _collector_loop():
    while True:
        try:
            _collect_metrics()
            with state_lock:
                state["last_collection_success_timestamp_seconds"] = time.time()
                state["last_collection_error"] = ""
        except Exception as exc:
            with state_lock:
                state["exporter_up"] = 0
                state["last_collection_error"] = str(exc)
        time.sleep(max(1, SCRAPE_INTERVAL_SECONDS))


def _render_metrics():
    with state_lock:
        snapshot = {
            "exporter_up": state["exporter_up"],
            "queue_depth": state["queue_depth"],
            "queue_oldest_age_seconds": state["queue_oldest_age_seconds"],
            "redis_memory_used_bytes": dict(state["redis_memory_used_bytes"]),
            "redis_memory_max_bytes": dict(state["redis_memory_max_bytes"]),
            "backup_last_success_timestamp_seconds": state[
                "backup_last_success_timestamp_seconds"
            ],
        }

    lines = [
        "# HELP moogle_runtime_exporter_up 1 when exporter can collect data.",
        "# TYPE moogle_runtime_exporter_up gauge",
        f"moogle_runtime_exporter_up {snapshot['exporter_up']}",
        "# HELP queue_depth Current queue depth by queue name.",
        "# TYPE queue_depth gauge",
        f'queue_depth{{queue="{INDEXER_QUEUE_KEY}"}} {snapshot["queue_depth"]}',
        "# HELP queue_oldest_message_age_seconds Age in seconds of oldest queue message.",
        "# TYPE queue_oldest_message_age_seconds gauge",
        (
            "queue_oldest_message_age_seconds"
            f'{{queue="{INDEXER_QUEUE_KEY}"}} {snapshot["queue_oldest_age_seconds"]}'
        ),
        "# HELP redis_memory_used_bytes Redis memory currently used in bytes.",
        "# TYPE redis_memory_used_bytes gauge",
    ]

    for role in ("pipeline", "query"):
        lines.append(
            f'redis_memory_used_bytes{{redis_role="{role}"}} '
            f'{snapshot["redis_memory_used_bytes"][role]}'
        )

    lines.extend(
        [
            "# HELP redis_memory_max_bytes Redis memory ceiling in bytes.",
            "# TYPE redis_memory_max_bytes gauge",
        ]
    )

    for role in ("pipeline", "query"):
        lines.append(
            f'redis_memory_max_bytes{{redis_role="{role}"}} '
            f'{snapshot["redis_memory_max_bytes"][role]}'
        )

    backup_ts = snapshot["backup_last_success_timestamp_seconds"]
    if backup_ts is not None:
        lines.extend(
            [
                "# HELP backup_last_success_timestamp_seconds Unix timestamp of latest successful backup.",
                "# TYPE backup_last_success_timestamp_seconds gauge",
                f"backup_last_success_timestamp_seconds {backup_ts}",
            ]
        )

    return "\n".join(lines) + "\n"


def _ready_window_seconds():
    return max(10, SCRAPE_INTERVAL_SECONDS * 3)


def _readiness_snapshot():
    with state_lock:
        snapshot = {
            "exporter_up": state["exporter_up"],
            "last_collection_success_timestamp_seconds": state[
                "last_collection_success_timestamp_seconds"
            ],
            "last_collection_error": state["last_collection_error"],
        }

    last_success = snapshot["last_collection_success_timestamp_seconds"]
    last_success_age_seconds = None
    if last_success is not None:
        last_success_age_seconds = max(0.0, time.time() - last_success)

    is_ready = (
        snapshot["exporter_up"] == 1
        and last_success_age_seconds is not None
        and last_success_age_seconds <= _ready_window_seconds()
    )

    return {
        "is_ready": is_ready,
        "last_success_age_seconds": last_success_age_seconds,
        "last_collection_error": snapshot["last_collection_error"],
    }


def _write_json(handler, status_code, payload):
    body = json.dumps(payload).encode("utf-8")
    handler.send_response(status_code)
    handler.send_header("Content-Type", "application/json")
    handler.send_header("Content-Length", str(len(body)))
    handler.end_headers()
    handler.wfile.write(body)


class MetricsHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health/live":
            _write_json(
                self,
                200,
                {
                    "status": "up",
                    "service": "runtime-metrics-exporter",
                    "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                },
            )
            return

        if self.path == "/health/ready":
            snapshot = _readiness_snapshot()
            _write_json(
                self,
                200 if snapshot["is_ready"] else 503,
                {
                    "status": "ready" if snapshot["is_ready"] else "not_ready",
                    "service": "runtime-metrics-exporter",
                    "dependencies": {
                        "collection_loop": snapshot["is_ready"],
                    },
                    "last_success_age_seconds": snapshot["last_success_age_seconds"],
                    "last_collection_error": snapshot["last_collection_error"],
                    "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                },
            )
            return

        if self.path != "/metrics":
            self.send_response(404)
            self.end_headers()
            self.wfile.write(b"not found")
            return

        payload = _render_metrics().encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, format, *args):
        return


if __name__ == "__main__":
    collector_thread = threading.Thread(target=_collector_loop, daemon=True)
    collector_thread.start()

    server = HTTPServer(("0.0.0.0", METRICS_PORT), MetricsHandler)
    server.serve_forever()
