#!/bin/bash

# Cleanup old plugin backups on Unraid server
# This script removes all backup directories to free up space

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

# Update SSH command if password was overridden
if [ -n "$2" ]; then
    SSH_CMD="sshpass -p '$UNRAID_PASSWORD' ssh -o StrictHostKeyChecking=no root@$UNRAID_IP"
fi

echo "========================================="
echo "Unraid Plugin Backup Cleanup"
echo "========================================="
echo "Target Server: $UNRAID_IP"
echo "Plugin: $PLUGIN_NAME"
echo ""

# Check if server is reachable
echo "Checking server connectivity..."
if ! ping -c 1 -W 2 "$UNRAID_IP" > /dev/null 2>&1; then
    echo "❌ Error: Cannot reach server at $UNRAID_IP"
    exit 1
fi
echo "✅ Server is reachable"
echo ""

# Check for existing backups
echo "Checking for existing backups..."
BACKUP_COUNT=$(eval "$SSH_CMD 'ls -d /boot/config/plugins/${PLUGIN_NAME}/backup-* 2>/dev/null | wc -l'")

if [ "$BACKUP_COUNT" -eq 0 ]; then
    echo "✅ No backups found - nothing to clean up"
    echo ""
    exit 0
fi

echo "Found $BACKUP_COUNT backup(s)"
echo ""

# List backups with sizes
echo "Backup directories:"
echo "----------------------------------------"
eval "$SSH_CMD 'du -sh /boot/config/plugins/${PLUGIN_NAME}/backup-* 2>/dev/null'"
echo ""

# Calculate total size
TOTAL_SIZE=$(eval "$SSH_CMD 'du -sh /boot/config/plugins/${PLUGIN_NAME}/backup-* 2>/dev/null | awk \"{sum+=\\\$1} END {print sum}\"'")
echo "Total backup size: ${TOTAL_SIZE}MB (approximate)"
echo ""

# Confirm deletion
read -p "Delete all backups? (yes/no): " CONFIRM

if [ "$CONFIRM" != "yes" ]; then
    echo "❌ Cleanup cancelled"
    exit 0
fi

# Delete backups
echo ""
echo "Deleting backups..."
eval "$SSH_CMD 'rm -rf /boot/config/plugins/${PLUGIN_NAME}/backup-*'"
echo "✅ All backups deleted"
echo ""

# Verify deletion
REMAINING=$(eval "$SSH_CMD 'ls -d /boot/config/plugins/${PLUGIN_NAME}/backup-* 2>/dev/null | wc -l'")

if [ "$REMAINING" -eq 0 ]; then
    echo "✅ Cleanup successful - all backups removed"
else
    echo "⚠️  Warning: $REMAINING backup(s) still remain"
fi

echo ""
echo "Current plugin directory:"
echo "----------------------------------------"
eval "$SSH_CMD 'ls -lah /boot/config/plugins/${PLUGIN_NAME}/'"
echo ""

echo "========================================="
echo "Cleanup Complete!"
echo "========================================="
echo ""

