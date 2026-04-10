#!/bin/sh

# Start tailscaled in the background
tailscaled --state=/var/lib/tailscale/tailscaled.state &

# Wait for tailscaled to start
sleep 2

# Authenticate with Tailscale if AUTHKEY is provided
if [ -n "$TAILSCALE_AUTHKEY" ]; then
    echo "Authenticating with Tailscale..."
    tailscale up --authkey=$TAILSCALE_AUTHKEY --hostname=${TAILSCALE_HOSTNAME:-flakyollama-ui}
fi

# Start Nginx
echo "Starting Nginx..."
exec nginx -g "daemon off;"
