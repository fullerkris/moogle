#!/bin/bash
set -e

if [ "${SKIP_ASSET_MANIFEST_CHECK:-false}" != "true" ]; then
    if [ ! -f /var/www/public/build/manifest.json ]; then
        echo "ERROR: Missing /var/www/public/build/manifest.json. Build frontend assets before starting the container." >&2
        exit 1
    fi
fi

# Clear Laravel caches if artisan exists
if [ -f /var/www/artisan ]; then
    php artisan config:clear
    php artisan cache:clear
    php artisan view:clear
    php artisan route:clear
    php artisan optimize:clear
fi

# Execute the main command
exec "$@"
