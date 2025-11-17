#!/bin/bash

# Automated deployment script for Unraid Management Agent
# Usage: ./scripts/deploy-to-unraid.sh <unraid_ip> [--test]

set -e

UNRAID_IP="$1"
TEST_MODE="$2"

if [ -z "$UNRAID_IP" ]; then
    echo "Usage: $0 <unraid_ip> [--test]"
    echo "Example: $0 192.168.1.100"
    echo "Example: $0 192.168.1.100 --test  # Run in test mode (with debug logging)"
    exit 1
fi

echo "========================================="
echo "Unraid Management Agent Deployment"
echo "========================================="
echo "Target Server: $UNRAID_IP"
echo ""

# Check if server is reachable
echo "Checking server connectivity..."
if ! ping -c 1 -W 2 "$UNRAID_IP" > /dev/null 2>&1; then
    echo "❌ Error: Cannot reach server at $UNRAID_IP"
    exit 1
fi
echo "✅ Server is reachable"
echo ""

# Build the binary
echo "Building release binary..."
if ! make release; then
    echo "❌ Error: Build failed"
    exit 1
fi
echo "✅ Build complete"
echo ""

# Check if binary exists
if [ ! -f "build/unraid-management-agent" ]; then
    echo "❌ Error: Binary not found at build/unraid-management-agent"
    exit 1
fi
echo "✅ Binary ready for deployment"
echo ""

# Stop existing service
echo "Stopping existing service on Unraid..."
ssh root@"$UNRAID_IP" "killall unraid-management-agent 2>/dev/null || true"
sleep 2
echo "✅ Service stopped"
echo ""

# Deploy binary
echo "Deploying binary to Unraid..."
if ! scp build/unraid-management-agent root@"$UNRAID_IP":/usr/local/emhttp/plugins/unraid-management-agent/; then
    echo "❌ Error: Failed to copy binary"
    exit 1
fi
echo "✅ Binary deployed"
echo ""

# Start service
echo "Starting service..."
if [ "$TEST_MODE" = "--test" ]; then
    echo "Starting in TEST MODE with debug logging..."
    ssh root@"$UNRAID_IP" "nohup /usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent boot --debug > /tmp/unraid-agent-debug.log 2>&1 &"
    echo "✅ Service started in debug mode"
    echo "   Debug logs: /tmp/unraid-agent-debug.log"
else
    ssh root@"$UNRAID_IP" "nohup /usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent boot > /dev/null 2>&1 &"
    echo "✅ Service started"
    echo "   Logs: /var/log/unraid-management-agent.log"
fi
echo ""

# Wait for service to start
echo "Waiting for service to initialize..."
sleep 3
echo ""

# Verify service is running
echo "Verifying service status..."
if ssh root@"$UNRAID_IP" "ps aux | grep -v grep | grep unraid-management-agent" > /dev/null 2>&1; then
    echo "✅ Service is running"
else
    echo "❌ Warning: Service may not be running"
    echo "   Check logs for details"
fi
echo ""

# Test API endpoints
echo "Testing API endpoints..."
echo "----------------------------------------"

# Test health endpoint
echo -n "Testing /api/v1/health... "
if ssh root@"$UNRAID_IP" "curl -s http://localhost:8043/api/v1/health" | grep -q "ok"; then
    echo "✅"
else
    echo "❌"
fi

# Test network endpoint
echo -n "Testing /api/v1/network... "
NETWORK_RESULT=$(ssh root@"$UNRAID_IP" "curl -s http://localhost:8043/api/v1/network")
if echo "$NETWORK_RESULT" | grep -q "\["; then
    NETWORK_COUNT=$(echo "$NETWORK_RESULT" | grep -o '"name"' | wc -l | tr -d ' ')
    echo "✅ ($NETWORK_COUNT interfaces found)"
else
    echo "❌"
fi

# Test system endpoint
echo -n "Testing /api/v1/system... "
if ssh root@"$UNRAID_IP" "curl -s http://localhost:8043/api/v1/system" | grep -q "hostname"; then
    echo "✅"
else
    echo "❌"
fi

# Test array endpoint
echo -n "Testing /api/v1/array... "
if ssh root@"$UNRAID_IP" "curl -s http://localhost:8043/api/v1/array" | grep -q "state"; then
    echo "✅"
else
    echo "❌"
fi

# Test disks endpoint
echo -n "Testing /api/v1/disks... "
DISK_RESULT=$(ssh root@"$UNRAID_IP" "curl -s http://localhost:8043/api/v1/disks")
if echo "$DISK_RESULT" | grep -q "\["; then
    DISK_COUNT=$(echo "$DISK_RESULT" | grep -o '"name"' | wc -l | tr -d ' ')
    echo "✅ ($DISK_COUNT disks found)"
else
    echo "❌"
fi

# Test shares endpoint
echo -n "Testing /api/v1/shares... "
SHARE_RESULT=$(ssh root@"$UNRAID_IP" "curl -s http://localhost:8043/api/v1/shares")
if echo "$SHARE_RESULT" | grep -q "\["; then
    SHARE_COUNT=$(echo "$SHARE_RESULT" | grep -o '"name"' | wc -l | tr -d ' ')
    echo "✅ ($SHARE_COUNT shares found)"
else
    echo "❌"
fi

echo ""
echo "========================================="
echo "Deployment Complete!"
echo "========================================="
echo ""
echo "API Base URL: http://$UNRAID_IP:8043/api/v1"
echo ""
echo "Quick Commands:"
echo "  View logs:        ssh root@$UNRAID_IP 'tail -f /var/log/unraid-management-agent.log'"
if [ "$TEST_MODE" = "--test" ]; then
echo "  View debug logs:  ssh root@$UNRAID_IP 'tail -f /tmp/unraid-agent-debug.log'"
fi
echo "  Stop service:     ssh root@$UNRAID_IP 'killall unraid-management-agent'"
echo "  Check status:     ssh root@$UNRAID_IP 'ps aux | grep unraid-management-agent | grep -v grep'"
echo ""
echo "Test API endpoints:"
echo "  curl -s http://$UNRAID_IP:8043/api/v1/health | jq"
echo "  curl -s http://$UNRAID_IP:8043/api/v1/system | jq"
echo "  curl -s http://$UNRAID_IP:8043/api/v1/network | jq"
echo ""
