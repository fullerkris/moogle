#!/bin/bash
set -euo pipefail

BACKUP_ROOT="${BACKUP_ROOT:-/backup}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"

echo "[$(date -Iseconds)] mongo-backup starting (retention: ${RETENTION_DAYS}d)"

while true; do
    TS=$(date +%Y%m%d_%H%M%S)
    BACKUP_DIR="${BACKUP_ROOT}/${TS}"

    echo "[$(date -Iseconds)] Starting backup → ${BACKUP_DIR}"

    if mongodump \
        --host="${MONGO_HOST:-mongo.internal}" \
        --port="${MONGO_PORT:-27017}" \
        --username="${MONGO_INITDB_ROOT_USERNAME}" \
        --password="${MONGO_INITDB_ROOT_PASSWORD}" \
        --authenticationDatabase=admin \
        --out="${BACKUP_DIR}" 2>&1; then
        echo "[$(date -Iseconds)] Backup complete: ${BACKUP_DIR}"
    else
        echo "[$(date -Iseconds)] ERROR: mongodump failed — skipping prune this cycle" >&2
        sleep 3600
        continue
    fi

    # Prune directories older than RETENTION_DAYS (only timestamped dirs, not accidental top-level files)
    find "${BACKUP_ROOT}" -maxdepth 1 -mindepth 1 -type d -mtime "+${RETENTION_DAYS}" -exec rm -rf {} +
    echo "[$(date -Iseconds)] Pruned backups older than ${RETENTION_DAYS} days"

    sleep 86400
done
