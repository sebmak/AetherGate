#!/bin/sh
set -e

# Default Unraid IDs: nobody:users (99:100)
PUID=${PUID:-99}
PGID=${PGID:-100}

# Create appgroup if not exists
if ! getent group "$PGID" >/dev/null 2>&1; then
    addgroup -g "$PGID" appgroup
else
    GROUP_NAME=$(getent group "$PGID" | cut -d: -f1)
fi

# Create appuser if not exists
if ! getent passwd "$PUID" >/dev/null 2>&1; then
    adduser -u "$PUID" -G "${GROUP_NAME:-appgroup}" -D -s /bin/sh appuser
fi

# Ensure data directories exist
DATA_DIR=${DATA_DIR:-/data}
mkdir -p "$DATA_DIR" "$DATA_DIR/certs"

# Check and fix ownership of data directory if needed
chown -R "$PUID:$PGID" "$DATA_DIR" 2>/dev/null || true

# If running as root, drop privileges with su-exec; otherwise run directly
if [ "$(id -u)" = "0" ] && [ "$PUID" != "0" ]; then
    exec su-exec "$PUID:$PGID" /var/app/proxyserver "$@"
else
    exec /var/app/proxyserver "$@"
fi
