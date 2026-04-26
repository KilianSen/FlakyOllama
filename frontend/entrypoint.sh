#!/bin/sh

# Set default backend URL if not provided
if [ -z "$BACKEND_URL" ]; then
    BACKEND_URL="http://balancer:8080"
fi

# Set default frontend port if not provided
if [ -z "$FRONTEND_PORT" ]; then
    FRONTEND_PORT="80"
fi

# Replace the placeholders in the Nginx config
sed -i "s|BACKEND_URL|$BACKEND_URL|g" /etc/nginx/nginx.conf
sed -i "s|FRONTEND_PORT|$FRONTEND_PORT|g" /etc/nginx/nginx.conf

# Start Nginx
echo "Starting Nginx on port $FRONTEND_PORT with proxy to $BACKEND_URL..."
exec nginx -g "daemon off;"
