#!/bin/bash
# Diagnostic script for Unraid Management Agent
# Loads server config from scripts/config.sh

SCRIPT_DIR="$(dirname "$0")"
if [ -f "$SCRIPT_DIR/config.sh" ]; then
    source "$SCRIPT_DIR/config.sh"
else
    echo "ERROR: scripts/config.sh not found. See config.sh.example" >&2
    exit 1
fi

HOST="${UNRAID_IP}:${API_PORT}"
API="http://${HOST}/api/v1"
PASS=0
FAIL=0
WARN=0

test_ep() {
  local path="$1" desc="$2" check="$3"
  printf "  %-45s " "$desc"
  resp=$(curl -s -m 10 "$API$path" 2>/dev/null)
  code=$?
  if [ $code -ne 0 ]; then
    echo "TIMEOUT"; FAIL=$((FAIL+1)); return
  fi
  if [ -z "$resp" ]; then
    echo "EMPTY"; WARN=$((WARN+1)); return
  fi
  if [ -n "$check" ]; then
    if echo "$resp" | grep -q "$check" 2>/dev/null; then
      echo "OK"; PASS=$((PASS+1))
    else
      echo "WARN (missing '$check')"; WARN=$((WARN+1))
    fi
  else
    bytes=$(echo "$resp" | wc -c | tr -d ' ')
    echo "OK (${bytes}B)"; PASS=$((PASS+1))
  fi
}

echo "======================================"
echo "   ENDPOINT DIAGNOSTICS - Go 1.26"
echo "======================================"
echo ""
echo "--- Core System ---"
test_ep "/health" "Health" "ok"
test_ep "/system" "System info" "hostname"
test_ep "/registration" "Registration"
test_ep "/notifications" "Notifications"
test_ep "/notifications/overview" "Notifications overview"

echo ""
echo "--- Storage ---"
test_ep "/array" "Array status" "state"
test_ep "/disks" "Disks list"
test_ep "/shares" "Shares list"
test_ep "/unassigned" "Unassigned devices"
test_ep "/unassigned/devices" "Unassigned detail"

echo ""
echo "--- Containers & VMs ---"
test_ep "/docker" "Docker containers"
test_ep "/vm" "Virtual machines"

echo ""
echo "--- Network ---"
test_ep "/network" "Network interfaces"
test_ep "/network/access-urls" "Access URLs"

echo ""
echo "--- Hardware ---"
test_ep "/hardware/full" "Full hardware"
test_ep "/hardware/cpu" "CPU info"
test_ep "/hardware/memory-devices" "Memory devices"

echo ""
echo "--- Monitoring ---"
test_ep "/gpu" "GPU metrics"
test_ep "/ups" "UPS status"
test_ep "/nut" "NUT status"
test_ep "/user-scripts" "User scripts"

echo ""
echo "--- ZFS ---"
test_ep "/zfs/pools" "ZFS pools"
test_ep "/zfs/datasets" "ZFS datasets"
test_ep "/zfs/arc" "ZFS ARC"

echo ""
echo "--- Settings ---"
test_ep "/settings/system" "System settings"
test_ep "/settings/disks" "Disk settings"
test_ep "/settings/docker" "Docker settings"
test_ep "/settings/vm" "VM settings"

echo ""
echo "--- Collectors ---"
test_ep "/collectors/status" "Collectors status"

echo ""
echo "--- Logs ---"
test_ep "/logs" "Log files"

echo ""
echo "--- New Features (Alerting) ---"
test_ep "/alerts" "Alert rules"
test_ep "/alerts/status" "Alerts status"
test_ep "/alerts/history" "Alert history"

echo ""
echo "--- New Features (Health Checks) ---"
test_ep "/healthchecks" "Health checks"
test_ep "/healthchecks/status" "Health checks status"
test_ep "/healthchecks/history" "Health check history"

echo ""
echo "--- Docker Updates ---"
test_ep "/docker/updates" "Docker updates check"

echo ""
echo "--- Swagger ---"
# Swagger is at /swagger/doc.json (not under /api/v1)
printf "  %-45s " "Swagger JSON"
swagger_resp=$(curl -s -m 5 "http://${HOST}/swagger/doc.json" 2>/dev/null)
if echo "$swagger_resp" | grep -q "paths" 2>/dev/null; then
  PASS=$((PASS+1)); echo "PASS"
else
  FAIL=$((FAIL+1)); echo "FAIL"
fi

echo ""
echo "--- MCP ---"
printf "  %-45s " "MCP initialize (POST /mcp)"
mcp_resp=$(curl -s -m 10 -X POST "http://${HOST}/mcp" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"diag","version":"1.0"}}}' 2>/dev/null)
if echo "$mcp_resp" | grep -q "protocolVersion" 2>/dev/null; then
  ver=$(echo "$mcp_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['result']['protocolVersion'])" 2>/dev/null)
  echo "OK (protocol=$ver)"
  PASS=$((PASS+1))
else
  echo "FAIL"
  FAIL=$((FAIL+1))
fi

echo ""
echo "--- MCP Tools Count ---"
# Send initialized notification then list tools
curl -s -X POST "http://${HOST}/mcp" \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: $(echo "$mcp_resp" | python3 -c "import sys,json; print(json.load(sys.stdin).get('result',{}).get('_meta',{}).get('sessionId',''))" 2>/dev/null)" \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' > /dev/null 2>&1

printf "  %-45s " "MCP tools/list"
tools_resp=$(curl -s -m 10 -X POST "http://${HOST}/mcp" \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: $(echo "$mcp_resp" | python3 -c "import sys,json; print(json.load(sys.stdin).get('result',{}).get('_meta',{}).get('sessionId',''))" 2>/dev/null)" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' 2>/dev/null)
if echo "$tools_resp" | grep -q "tools" 2>/dev/null; then
  count=$(echo "$tools_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d['result']['tools']))" 2>/dev/null)
  echo "OK ($count tools)"
  PASS=$((PASS+1))
else
  echo "FAIL"
  FAIL=$((FAIL+1))
fi

echo ""
echo "======================================"
echo "  RESULTS: $PASS passed, $WARN warnings, $FAIL failed"
echo "======================================"
