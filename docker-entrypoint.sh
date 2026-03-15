#!/bin/sh
set -e

# Fix ownership of bind-mounted volumes at container startup so the non-root
# volt user can read and write the database and uploaded icons.
# This runs as root (no USER directive before ENTRYPOINT), then drops
# privileges via su-exec before executing the application.
chown -R volt:volt /app/data /app/uploads

exec su-exec volt "$@"
