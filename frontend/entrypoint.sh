#!/bin/sh

# Set default backend URL if not provided
# BACKEND_URL should be something like http://balancer:8080
if [ -z "$BACKEND_URL" ]; then
    BACKEND_URL="http://balancer:8080"
fi

# Replace the placeholder in the Nginx config
sed -i "s|BACKEND_URL|$BACKEND_URL|g" /etc/nginx/nginx.conf

# Start Nginx
echo "Starting Nginx with proxy to $BACKEND_URL..."
exec nginx -g "daemon off;"
