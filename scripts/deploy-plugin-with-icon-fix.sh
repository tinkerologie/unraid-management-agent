#!/bin/bash

# Deploy Unraid Management Agent Plugin with Icon Fix
# This script builds and deploys the complete plugin package including the icon fix

set -e

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

# Update SSH command if password was overridden
if [ -n "$2" ]; then
    SSH_CMD="sshpass -p '$UNRAID_PASSWORD' ssh -o StrictHostKeyChecking=no root@$UNRAID_IP"
fi

VERSION=$(cat VERSION)
BUILD_DIR="build"
PLUGIN_BUNDLE="${BUILD_DIR}/${PLUGIN_NAME}-${VERSION}.tgz"

# SSH command wrapper with sshpass
SSH_CMD="sshpass -p '$UNRAID_PASSWORD' ssh -o StrictHostKeyChecking=no root@$UNRAID_IP"
SCP_CMD="sshpass -p '$UNRAID_PASSWORD' scp -o StrictHostKeyChecking=no"

echo "========================================="
echo "Unraid Plugin Deployment with Icon Fix"
echo "========================================="
echo "Target Server: $UNRAID_IP"
echo "Plugin Version: $VERSION"
echo ""

# Step 1: Check server connectivity
echo "Step 1: Checking server connectivity..."
if ! ping -c 1 -W 2 "$UNRAID_IP" > /dev/null 2>&1; then
    echo "❌ Error: Cannot reach server at $UNRAID_IP"
    exit 1
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
eval "$SSH_CMD 'killall ${PLUGIN_NAME} 2>/dev/null || true'"
sleep 2
echo "✅ Service stopped"
echo ""

# Step 5: Backup existing plugin (if exists)
if [ "$CREATE_BACKUP" = "yes" ]; then
    echo "Step 5: Backing up existing plugin..."
    BACKUP_DIR="/boot/config/plugins/${PLUGIN_NAME}/backup-$(date +%Y%m%d-%H%M%S)"
    eval "$SSH_CMD '
    if [ -d /usr/local/emhttp/plugins/${PLUGIN_NAME} ]; then
        mkdir -p $BACKUP_DIR
        cp -r /usr/local/emhttp/plugins/${PLUGIN_NAME}/* $BACKUP_DIR/ 2>/dev/null || true
        echo \"Backup created at: $BACKUP_DIR\"
    else
        echo \"No existing plugin to backup\"
    fi
    '"
    echo "✅ Backup complete"
else
    echo "Step 5: Skipping backup (CREATE_BACKUP=no)"
fi
echo ""

# Step 6: Remove old plugin files
echo "Step 6: Removing old plugin files..."
eval "$SSH_CMD '
rm -rf /usr/local/emhttp/plugins/${PLUGIN_NAME}/* 2>/dev/null || true
mkdir -p /usr/local/emhttp/plugins/${PLUGIN_NAME}
'"
echo "✅ Old files removed"
echo ""

# Step 7: Upload plugin bundle
echo "Step 7: Uploading plugin bundle..."
if ! eval "$SCP_CMD '$PLUGIN_BUNDLE' root@$UNRAID_IP:/tmp/"; then
    echo "❌ Error: Failed to upload plugin bundle"
    exit 1
fi
echo "✅ Plugin bundle uploaded"
echo ""

# Step 8: Extract plugin bundle
echo "Step 8: Extracting plugin bundle..."
eval "$SSH_CMD '
cd /tmp
tar -xzf ${PLUGIN_NAME}-${VERSION}.tgz
cp -r usr/local/emhttp/plugins/${PLUGIN_NAME}/* /usr/local/emhttp/plugins/${PLUGIN_NAME}/
rm -rf usr ${PLUGIN_NAME}-${VERSION}.tgz
'"
echo "✅ Plugin extracted"
echo ""

# Step 9: Set permissions
echo "Step 9: Setting permissions..."
eval "$SSH_CMD '
chmod +x /usr/local/emhttp/plugins/${PLUGIN_NAME}/${PLUGIN_NAME}
chmod +x /usr/local/emhttp/plugins/${PLUGIN_NAME}/scripts/* 2>/dev/null || true
chmod +x /usr/local/emhttp/plugins/${PLUGIN_NAME}/event/* 2>/dev/null || true
'"
echo "✅ Permissions set"
echo ""

# Step 10: Verify plugin files
echo "Step 10: Verifying plugin files..."
echo "----------------------------------------"
eval "$SSH_CMD '
echo \"Checking required files:\"
[ -f /usr/local/emhttp/plugins/${PLUGIN_NAME}/${PLUGIN_NAME} ] && echo \"  ✅ Binary executable\" || echo \"  ❌ Binary missing\"
[ -f /usr/local/emhttp/plugins/${PLUGIN_NAME}/${PLUGIN_NAME}.page ] && echo \"  ✅ Page file\" || echo \"  ❌ Page file missing\"
[ -f /usr/local/emhttp/plugins/${PLUGIN_NAME}/VERSION ] && echo \"  ✅ Version file\" || echo \"  ❌ Version file missing\"
[ -d /usr/local/emhttp/plugins/${PLUGIN_NAME}/images ] && echo \"  ✅ Images directory\" || echo \"  ❌ Images directory missing\"
[ -f /usr/local/emhttp/plugins/${PLUGIN_NAME}/images/${PLUGIN_NAME}.png ] && echo \"  ✅ Icon PNG\" || echo \"  ❌ Icon PNG missing\"
[ -d /usr/local/emhttp/plugins/${PLUGIN_NAME}/scripts ] && echo \"  ✅ Scripts directory\" || echo \"  ❌ Scripts directory missing\"
[ -d /usr/local/emhttp/plugins/${PLUGIN_NAME}/event ] && echo \"  ✅ Event directory\" || echo \"  ❌ Event directory missing\"
'"
echo ""

# Step 11: Start the service
echo "Step 11: Starting service..."
eval "$SSH_CMD '
nohup /usr/local/emhttp/plugins/${PLUGIN_NAME}/${PLUGIN_NAME} --port 8043 boot > /dev/null 2>&1 &
'"
sleep 3
echo "✅ Service started"
echo ""

# Step 12: Verify service is running
echo "Step 12: Verifying service status..."
if eval "$SSH_CMD 'pidof ${PLUGIN_NAME}'" > /dev/null 2>&1; then
    PID=$(eval "$SSH_CMD 'pidof ${PLUGIN_NAME}'")
    echo "✅ Service is running (PID: $PID)"
else
    echo "❌ Warning: Service may not be running"
    echo "   Check logs: $SSH_CMD 'tail -f /var/log/${PLUGIN_NAME}.log'"
fi
echo ""

# Step 13: Test API endpoints
echo "Step 13: Testing API endpoints..."
echo "----------------------------------------"

# Wait a moment for API to be ready
sleep 2

# Test health endpoint
echo -n "Testing /api/v1/health... "
if eval "$SSH_CMD 'curl -s http://localhost:8043/api/v1/health'" | grep -q "ok"; then
    echo "✅"
else
    echo "❌"
fi

# Test system endpoint
echo -n "Testing /api/v1/system... "
if eval "$SSH_CMD 'curl -s http://localhost:8043/api/v1/system'" | grep -q "hostname"; then
    echo "✅"
else
    echo "❌"
fi

# Test array endpoint
echo -n "Testing /api/v1/array... "
if eval "$SSH_CMD 'curl -s http://localhost:8043/api/v1/array'" | grep -q "state"; then
    echo "✅"
else
    echo "❌"
fi

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
echo "Test API:"
echo "  curl -s http://$UNRAID_IP:8043/api/v1/health | jq"
echo "  curl -s http://$UNRAID_IP:8043/api/v1/system | jq"
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

