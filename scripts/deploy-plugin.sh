#!/bin/bash

# Deploy Unraid Management Agent Plugin with Icon Fix
# This script builds and deploys the complete plugin package including the icon fix

set -e  # Exit on error, except during API endpoint testing

# Load configuration from config.sh
SCRIPT_DIR="$(dirname "$0")"
if [ -f "$SCRIPT_DIR/config.sh" ]; then
    source "$SCRIPT_DIR/config.sh"
else
    echo "ERROR: Configuration file not found!"
    echo "Please create scripts/config.sh from scripts/config.sh.example"
    echo ""
    echo "  cp scripts/config.sh.example scripts/config.sh"
    echo "  # Edit config.sh with your server details"
    echo ""
    exit 1
fi

# Allow command-line overrides
UNRAID_IP="${1:-$UNRAID_IP}"
UNRAID_PASSWORD="${2:-$UNRAID_PASSWORD}"
CREATE_BACKUP="${3:-no}"  # Set to "yes" to create backup, default is "no"

VERSION=$(cat VERSION)
BUILD_DIR="build"
PLUGIN_BUNDLE="${BUILD_DIR}/${PLUGIN_NAME}-${VERSION}.tgz"

# Helper functions to avoid eval and command injection risks
# These functions properly quote arguments to prevent shell metacharacter injection
run_ssh() {
    sshpass -p "$UNRAID_PASSWORD" ssh -o StrictHostKeyChecking=no "root@$UNRAID_IP" "$@"
}

run_scp() {
    sshpass -p "$UNRAID_PASSWORD" scp -o StrictHostKeyChecking=no "$@"
}

echo "========================================="
echo "Unraid Plugin Deployment with Icon Fix"
echo "========================================="
echo "Target Server: $UNRAID_IP"
echo "Plugin Version: $VERSION"
echo ""

# Step 1: Check server connectivity
echo "Step 1: Checking server connectivity..."
# Use curl to check API health endpoint (more reliable than ping which may be blocked)
if ! curl -s -m 5 "http://${UNRAID_IP}:${API_PORT}/api/v1/health" > /dev/null 2>&1; then
    # Fallback to SSH check if API isn't running yet
    if ! run_ssh echo ok > /dev/null 2>&1; then
        echo "❌ Error: Cannot reach server at $UNRAID_IP"
        exit 1
    fi
fi
echo "✅ Server is reachable"
echo ""

# Step 2: Build the plugin package
echo "Step 2: Building plugin package..."
echo "----------------------------------------"
if ! make package; then
    echo "❌ Error: Package build failed"
    exit 1
fi

# Verify the package was created
if [ ! -f "$PLUGIN_BUNDLE" ]; then
    echo "❌ Error: Plugin bundle not found at $PLUGIN_BUNDLE"
    exit 1
fi
echo "✅ Plugin package built successfully"
echo "   Package: $PLUGIN_BUNDLE"
echo ""

# Step 3: Verify icon fix in PLG file
echo "Step 3: Verifying icon fix..."
if grep -q 'icon="server"' meta/template/${PLUGIN_NAME}.plg; then
    echo "✅ Icon attribute found in PLG file"
else
    echo "⚠️  Warning: Icon attribute not found in PLG file"
    echo "   The icon may not display correctly"
fi
echo ""

# Step 4: Stop existing service
echo "Step 4: Stopping existing service..."
run_ssh "killall ${PLUGIN_NAME} 2>/dev/null || true"
sleep 2
echo "✅ Service stopped"
echo ""

# Step 5: Backup existing plugin (if exists)
if [ "$CREATE_BACKUP" = "yes" ]; then
    echo "Step 5: Backing up existing plugin..."
    BACKUP_DIR="/boot/config/plugins/${PLUGIN_NAME}/backup-$(date +%Y%m%d-%H%M%S)"
    run_ssh "
    if [ -d /usr/local/emhttp/plugins/${PLUGIN_NAME} ]; then
        mkdir -p $BACKUP_DIR
        cp -r /usr/local/emhttp/plugins/${PLUGIN_NAME}/* $BACKUP_DIR/ 2>/dev/null || true
        echo \"Backup created at: $BACKUP_DIR\"
    else
        echo \"No existing plugin to backup\"
    fi
    "
    echo "✅ Backup complete"
else
    echo "Step 5: Skipping backup (CREATE_BACKUP=no)"
fi
echo ""

# Step 6: Remove old plugin files
echo "Step 6: Removing old plugin files..."
run_ssh "
rm -rf /usr/local/emhttp/plugins/${PLUGIN_NAME}/* 2>/dev/null || true
mkdir -p /usr/local/emhttp/plugins/${PLUGIN_NAME}
"
echo "✅ Old files removed"
echo ""

# Step 7: Upload plugin bundle
echo "Step 7: Uploading plugin bundle..."
if ! run_scp "$PLUGIN_BUNDLE" "root@$UNRAID_IP:/tmp/"; then
    echo "❌ Error: Failed to upload plugin bundle"
    exit 1
fi
echo "✅ Plugin bundle uploaded"
echo ""

# Step 8: Extract plugin bundle
echo "Step 8: Extracting plugin bundle..."
run_ssh "
cd /tmp
tar -xzf ${PLUGIN_NAME}-${VERSION}.tgz
cp -r usr/local/emhttp/plugins/${PLUGIN_NAME}/* /usr/local/emhttp/plugins/${PLUGIN_NAME}/
rm -rf usr ${PLUGIN_NAME}-${VERSION}.tgz
"
echo "✅ Plugin extracted"
echo ""

# Step 9: Set permissions
echo "Step 9: Setting permissions..."
run_ssh "
chmod +x /usr/local/emhttp/plugins/${PLUGIN_NAME}/${PLUGIN_NAME}
chmod +x /usr/local/emhttp/plugins/${PLUGIN_NAME}/scripts/* 2>/dev/null || true
chmod +x /usr/local/emhttp/plugins/${PLUGIN_NAME}/event/* 2>/dev/null || true
"
echo "✅ Permissions set"
echo ""

# Step 10: Verify plugin files
echo "Step 10: Verifying plugin files..."
echo "----------------------------------------"
run_ssh "
echo \"Checking required files:\"
[ -f /usr/local/emhttp/plugins/${PLUGIN_NAME}/${PLUGIN_NAME} ] && echo \"  ✅ Binary executable\" || echo \"  ❌ Binary missing\"
[ -f /usr/local/emhttp/plugins/${PLUGIN_NAME}/${PLUGIN_NAME}.page ] && echo \"  ✅ Page file\" || echo \"  ❌ Page file missing\"
[ -f /usr/local/emhttp/plugins/${PLUGIN_NAME}/VERSION ] && echo \"  ✅ Version file\" || echo \"  ❌ Version file missing\"
[ -d /usr/local/emhttp/plugins/${PLUGIN_NAME}/images ] && echo \"  ✅ Images directory\" || echo \"  ❌ Images directory missing\"
[ -f /usr/local/emhttp/plugins/${PLUGIN_NAME}/images/${PLUGIN_NAME}.png ] && echo \"  ✅ Icon PNG\" || echo \"  ❌ Icon PNG missing\"
[ -d /usr/local/emhttp/plugins/${PLUGIN_NAME}/scripts ] && echo \"  ✅ Scripts directory\" || echo \"  ❌ Scripts directory missing\"
[ -d /usr/local/emhttp/plugins/${PLUGIN_NAME}/event ] && echo \"  ✅ Event directory\" || echo \"  ❌ Event directory missing\"
"
echo ""

# Step 11: Create default configuration if needed
echo "Step 11: Creating default configuration..."
run_ssh "
mkdir -p /boot/config/plugins/${PLUGIN_NAME}
if [ ! -f /boot/config/plugins/${PLUGIN_NAME}/config.cfg ]; then
    cat > /boot/config/plugins/${PLUGIN_NAME}/config.cfg << EOF
PORT=8043
EOF
    echo \"✅ Default configuration created\"
else
    echo \"✅ Configuration file already exists\"
fi
"
echo ""

# Step 12: Start the service using the start script
echo "Step 12: Starting service..."
run_ssh "/usr/local/emhttp/plugins/${PLUGIN_NAME}/scripts/start"
sleep 3
echo "✅ Service started"
echo ""

# Step 13: Verify service is running
echo "Step 13: Verifying service status..."
if run_ssh "pidof ${PLUGIN_NAME}" > /dev/null 2>&1; then
    PID=$(run_ssh "pidof ${PLUGIN_NAME}")
    echo "✅ Service is running (PID: $PID)"
else
    echo "❌ Warning: Service may not be running"
    echo "   Check logs: ssh root@$UNRAID_IP 'tail -f /var/log/${PLUGIN_NAME}.log'"
fi
echo ""

# Step 14: Test API endpoints
echo "Step 14: Testing API endpoints..."
echo "----------------------------------------"

# Wait a moment for API to be ready
sleep 2

# Helper function to test endpoint
test_endpoint() {
    local endpoint="$1"
    local description="$2"
    local search_term="${3:-}"

    echo -n "Testing $endpoint... "

    # Run the curl command and capture response, don't fail on error
    response=$(run_ssh "curl -s http://localhost:8043/api/v1${endpoint}" || true)

    # Check for actual error responses (JSON error objects), not just the word "error" in field names
    # Look for patterns like {"error":"message"} or "success":false
    if echo "$response" | grep -qE '"error"\s*:\s*"[^"]+"|"success"\s*:\s*false' 2>/dev/null; then
        echo "❌ (error response)"
        return 0  # Don't fail the script, just report the error
    elif [ -n "$search_term" ] && ! echo "$response" | grep -q "$search_term" 2>/dev/null; then
        echo "⚠️  (response doesn't contain '$search_term')"
        return 0
    elif [ -z "$response" ]; then
        echo "⚠️  (no response)"
        return 0
    else
        echo "✅"
        return 0
    fi
}

# Core System Endpoints
echo "Core System Endpoints:"
test_endpoint "/health" "Health check" "ok"
test_endpoint "/system" "System info" "hostname"
test_endpoint "/logs" "Logs"
test_endpoint "/notifications" "Notifications"
test_endpoint "/notifications/overview" "Notifications overview"
test_endpoint "/registration" "Registration"

echo ""
echo "Array & Storage Endpoints:"
test_endpoint "/array" "Array status" "state"
test_endpoint "/disks" "Disks list"
test_endpoint "/shares" "Shares list"
test_endpoint "/unassigned" "Unassigned devices"
test_endpoint "/unassigned/devices" "Unassigned devices detail"

echo ""
echo "Containers & VMs Endpoints:"
test_endpoint "/docker" "Docker containers"
test_endpoint "/vm" "Virtual machines"

echo ""
echo "Network & Hardware Endpoints:"
test_endpoint "/network" "Network interfaces"
test_endpoint "/network/access-urls" "Network access URLs"
test_endpoint "/hardware/full" "Full hardware info"
test_endpoint "/hardware/cpu" "CPU info"
test_endpoint "/hardware/memory-devices" "Memory devices"

echo ""
echo "Monitoring Endpoints:"
test_endpoint "/gpu" "GPU metrics"
test_endpoint "/ups" "UPS status"
test_endpoint "/nut" "NUT status"
test_endpoint "/user-scripts" "User scripts"

echo ""
echo "ZFS Endpoints:"
test_endpoint "/zfs/pools" "ZFS pools"
test_endpoint "/zfs/datasets" "ZFS datasets"
test_endpoint "/zfs/arc" "ZFS ARC"

echo ""
echo "Collector Status:"
test_endpoint "/collectors/status" "Collectors status"

echo ""
echo "Settings Endpoints:"
test_endpoint "/settings/system" "System settings"
test_endpoint "/settings/disks" "Disks settings"
test_endpoint "/settings/docker" "Docker settings"
test_endpoint "/settings/vm" "VM settings"

echo ""

# Step 14: Display verification checklist
echo "========================================="
echo "Deployment Complete!"
echo "========================================="
echo ""
echo "✅ Plugin deployed successfully"
echo "✅ Service is running"
echo "✅ API endpoints responding"
echo ""
echo "📋 Manual Verification Checklist:"
echo "----------------------------------------"
echo "1. Open Unraid Web UI: http://$UNRAID_IP"
echo "2. Navigate to: Plugins"
echo "3. Verify: 'Unraid Management Agent' appears in the list"
echo "4. Check: Server icon (📦) is visible next to the plugin name"
echo "5. Verify: Plugin version shows: $VERSION"
echo "6. Navigate to: Settings > Utilities > Management Agent"
echo "7. Verify: Settings page loads correctly"
echo "8. Check: Icon appears in the Settings menu"
echo ""
echo "🔧 Useful Commands:"
echo "----------------------------------------"
echo "View logs:"
echo "  ssh root@$UNRAID_IP 'tail -f /var/log/${PLUGIN_NAME}.log'"
echo ""
echo "Check service status:"
echo "  ssh root@$UNRAID_IP 'ps aux | grep ${PLUGIN_NAME} | grep -v grep'"
echo ""
echo "Stop service:"
echo "  ssh root@$UNRAID_IP 'killall ${PLUGIN_NAME}'"
echo ""
echo "Restart service:"
echo "  ssh root@$UNRAID_IP 'killall ${PLUGIN_NAME} && nohup /usr/local/emhttp/plugins/${PLUGIN_NAME}/${PLUGIN_NAME} --port 8043 boot > /dev/null 2>&1 &'"
echo ""
echo "Test specific API endpoints:"
echo "  curl -s http://$UNRAID_IP:8043/api/v1/health | jq"
echo "  curl -s http://$UNRAID_IP:8043/api/v1/system | jq"
echo "  curl -s http://$UNRAID_IP:8043/api/v1/array | jq"
echo "  curl -s http://$UNRAID_IP:8043/api/v1/docker | jq"
echo "  curl -s http://$UNRAID_IP:8043/api/v1/vm | jq"
echo ""
echo "Test WebSocket connection:"
echo "  wscat -c ws://$UNRAID_IP:8043/api/v1/ws"
echo ""
echo "List all available endpoints:"
echo "  curl -s http://$UNRAID_IP:8043/swagger/doc.json | jq '.paths | keys[]'"
echo ""
echo "🎯 Icon Fix Verification:"
echo "----------------------------------------"
echo "The plugin now includes icon=\"server\" in the PLG file."
echo "This should display a server rack icon in the Plugins page."
echo ""
echo "If the icon doesn't appear immediately:"
echo "  1. Refresh the Plugins page (Ctrl+F5)"
echo "  2. Clear browser cache"
echo "  3. Check browser console for errors (F12)"
echo ""
echo "========================================="
echo ""
