#!/bin/sh

if [ -z "$REDIS_URL"]; then 
    export REDIS_URL="redis://localhost:6379"
fi

if [ -z "$ROOT_URL"]; then
    export ROOT_URL="https://${OSC_HOSTNAME}"
fi

export ROOT_URL

echo "Using Redis URL: $REDIS_URL"
echo "Using Root URL: $ROOT_URL"

exec "$@"