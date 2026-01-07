#!/bin/bash

# Live Validation Script for Unraid Management Agent
# Tests all endpoints and compares with actual system state

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

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test results
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0

# Helper functions
print_header() {
    echo ""
    echo "========================================="
    echo "$1"
    echo "========================================="
}

print_test() {
    echo -e "${BLUE}[TEST]${NC} $1"
}

print_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((TESTS_PASSED++))
    ((TESTS_TOTAL++))
}

print_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((TESTS_FAILED++))
    ((TESTS_TOTAL++))
}

print_info() {
    echo -e "${YELLOW}[INFO]${NC} $1"
}

# Test API endpoint
test_endpoint() {
    local endpoint=$1
    local description=$2
    
    print_test "Testing ${endpoint} - ${description}"
    
    local response=$(curl -s -w "\n%{http_code}" "${API_BASE}${endpoint}")
    local http_code=$(echo "$response" | tail -n1)
    local body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "200" ]; then
        print_pass "HTTP 200 OK"
        echo "$body"
        return 0
    else
        print_fail "HTTP ${http_code}"
        echo "$body"
        return 1
    fi
}

# Start validation
print_header "UNRAID MANAGEMENT AGENT - LIVE VALIDATION"
echo "Target: ${UNRAID_IP}:${API_PORT}"
echo "Started: $(date)"
echo ""

# Check service is running
print_header "1. SERVICE STATUS CHECK"
print_test "Checking if service is running"
if eval "$SSH_CMD 'ps aux | grep -v grep | grep unraid-management-agent'" > /dev/null 2>&1; then
    print_pass "Service is running"
    eval "$SSH_CMD 'ps aux | grep -v grep | grep unraid-management-agent'"
else
    print_fail "Service is not running"
    exit 1
fi

# Test all API endpoints (18 tests including new log endpoints)
print_header "2. API ENDPOINT VALIDATION"
echo "Testing API endpoints..."
echo ""

# 1. Health endpoint
print_test "1. Testing /health"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/health")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ] && echo "$body" | jq -e '.status == "ok"' >/dev/null 2>&1; then
    print_pass "HTTP $http_code | ${time}s | Status: OK"
else
    print_fail "HTTP $http_code | Failed"
fi

# 2. System endpoint
print_test "2. Testing /system"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/system")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    hostname=$(echo "$body" | jq -r '.hostname // "N/A"')
    cpu=$(echo "$body" | jq -r '.cpu_usage_percent // "N/A"')
    ram=$(echo "$body" | jq -r '.ram_usage_percent // "N/A"')
    uptime=$(echo "$body" | jq -r '.uptime_seconds // "N/A"')
    print_pass "HTTP $http_code | ${time}s | Host: $hostname | CPU: ${cpu}% | RAM: ${ram}%"
else
    print_fail "HTTP $http_code | Failed"
fi

# 3. Array endpoint
print_test "3. Testing /array"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/array")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    state=$(echo "$body" | jq -r '.state // "N/A"')
    disks=$(echo "$body" | jq -r '.num_disks // 0')
    total_tb=$(echo "$body" | jq -r '(.total_bytes // 0) / 1099511627776 | floor')
    free_tb=$(echo "$body" | jq -r '(.free_bytes // 0) / 1099511627776 | floor')
    print_pass "HTTP $http_code | ${time}s | State: $state | Disks: $disks | Total: ${total_tb}TB | Free: ${free_tb}TB"
else
    print_fail "HTTP $http_code | Failed"
fi

# 4. Disks endpoint
print_test "4. Testing /disks"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/disks")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq 'length')
    print_pass "HTTP $http_code | ${time}s | Count: $count disks"
else
    print_fail "HTTP $http_code | Failed"
fi

# 5. Network endpoint
print_test "5. Testing /network"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/network")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq 'length')
    print_pass "HTTP $http_code | ${time}s | Count: $count interfaces"
else
    print_fail "HTTP $http_code | Failed"
fi

# 6. Shares endpoint
print_test "6. Testing /shares"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/shares")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq 'length')
    print_pass "HTTP $http_code | ${time}s | Count: $count shares"
else
    print_fail "HTTP $http_code | Failed"
fi

# 7. Docker endpoint
print_test "7. Testing /docker"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/docker")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq 'length')
    running=$(echo "$body" | jq '[.[] | select(.status == "running")] | length')
    print_pass "HTTP $http_code | ${time}s | Total: $count | Running: $running"
else
    print_fail "HTTP $http_code | Failed"
fi

# 8. VM endpoint
print_test "8. Testing /vm"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/vm")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq 'length')
    running=$(echo "$body" | jq '[.[] | select(.state == "running")] | length')
    print_pass "HTTP $http_code | ${time}s | Total: $count | Running: $running"
else
    print_fail "HTTP $http_code | Failed"
fi

# 9. UPS endpoint
print_test "9. Testing /ups"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/ups")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    status=$(echo "$body" | jq -r '.status // "N/A"')
    charge=$(echo "$body" | jq -r '.battery_charge_percent // "N/A"')
    print_pass "HTTP $http_code | ${time}s | Status: $status | Charge: ${charge}%"
else
    print_fail "HTTP $http_code | Failed"
fi

# 10. GPU endpoint
print_test "10. Testing /gpu"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/gpu")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq 'length')
    print_pass "HTTP $http_code | ${time}s | Count: $count GPUs"
else
    print_fail "HTTP $http_code | Failed"
fi

# 11. Registration endpoint
print_test "11. Testing /registration"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/registration")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    type=$(echo "$body" | jq -r '.type // "N/A"')
    state=$(echo "$body" | jq -r '.state // "N/A"')
    print_pass "HTTP $http_code | ${time}s | Type: $type | State: $state"
else
    print_fail "HTTP $http_code | Failed"
fi

# 12. Logs list endpoint
print_test "12. Testing /logs (log list)"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/logs")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq '.logs | length')
    print_pass "HTTP $http_code | ${time}s | Available logs: $count"
else
    print_fail "HTTP $http_code | Failed"
fi

# 12a. Logs by filename - syslog
print_test "12a. Testing /logs/syslog?lines=5 (new endpoint)"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/logs/syslog?lines=5")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    lines=$(echo "$body" | jq -r '.lines_returned // 0')
    print_pass "HTTP $http_code | ${time}s | Lines returned: $lines"
else
    print_fail "HTTP $http_code | Failed"
fi

# 12b. Logs by filename - dmesg
print_test "12b. Testing /logs/dmesg?lines=5 (new endpoint)"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/logs/dmesg?lines=5")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    lines=$(echo "$body" | jq -r '.lines_returned // 0')
    print_pass "HTTP $http_code | ${time}s | Lines returned: $lines"
else
    print_fail "HTTP $http_code | Failed"
fi

# 13. Notifications endpoint
print_test "13. Testing /notifications"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/notifications")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq 'length')
    print_pass "HTTP $http_code | ${time}s | Count: $count notifications"
else
    print_fail "HTTP $http_code | Failed"
fi

# 14. Unassigned devices endpoint
print_test "14. Testing /unassigned"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/unassigned")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    devices=$(echo "$body" | jq '.devices | length')
    shares=$(echo "$body" | jq '.remote_shares | length')
    print_pass "HTTP $http_code | ${time}s | Devices: $devices | Shares: $shares"
else
    print_fail "HTTP $http_code | Failed"
fi

# 15. Unassigned devices (devices only)
print_test "15. Testing /unassigned/devices"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/unassigned/devices")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq 'length')
    print_pass "HTTP $http_code | ${time}s | Count: $count devices"
else
    print_fail "HTTP $http_code | Failed"
fi

# 16. Unassigned devices (remote shares only)
print_test "16. Testing /unassigned/remote-shares"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/unassigned/remote-shares")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq 'length')
    print_pass "HTTP $http_code | ${time}s | Count: $count shares"
else
    print_fail "HTTP $http_code | Failed"
fi

# 17. Hardware info endpoint
print_test "17. Testing /hardware/full"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/hardware/full")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    bios=$(echo "$body" | jq -r '.bios.vendor // "N/A"')
    print_pass "HTTP $http_code | ${time}s | BIOS Vendor: $bios"
else
    print_fail "HTTP $http_code | Failed"
fi

# 18. Network access URLs endpoint
print_test "18. Testing /network/access-urls"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/network/access-urls")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq '.urls | length')
    print_pass "HTTP $http_code | ${time}s | URLs: $count"
else
    print_fail "HTTP $http_code | Failed"
fi

# 19. System settings endpoint
print_test "19. Testing /settings/system"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/settings/system")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    server=$(echo "$body" | jq -r '.server_name // "N/A"')
    print_pass "HTTP $http_code | ${time}s | Server: $server"
else
    print_fail "HTTP $http_code | Failed"
fi

# 20. Docker settings endpoint
print_test "20. Testing /settings/docker"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/settings/docker")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    enabled=$(echo "$body" | jq -r '.enabled // "N/A"')
    print_pass "HTTP $http_code | ${time}s | Docker Enabled: $enabled"
else
    print_fail "HTTP $http_code | Failed"
fi

# 21. VM settings endpoint
print_test "21. Testing /settings/vm"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/settings/vm")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    enabled=$(echo "$body" | jq -r '.enabled // "N/A"')
    print_pass "HTTP $http_code | ${time}s | VM Enabled: $enabled"
else
    print_fail "HTTP $http_code | Failed"
fi

# 22. Disk settings endpoint
print_test "22. Testing /settings/disks"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/settings/disks")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    spindown=$(echo "$body" | jq -r '.spindown_delay_minutes // "N/A"')
    print_pass "HTTP $http_code | ${time}s | Spindown: ${spindown} min"
else
    print_fail "HTTP $http_code | Failed"
fi

# 23. User scripts endpoint
print_test "23. Testing /user-scripts"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/user-scripts")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq 'length')
    print_pass "HTTP $http_code | ${time}s | Scripts: $count"
else
    print_fail "HTTP $http_code | Failed"
fi

# 24. Parity check history endpoint
print_test "24. Testing /array/parity-check/history"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/array/parity-check/history")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq '.records | length')
    print_pass "HTTP $http_code | ${time}s | Records: $count"
else
    print_fail "HTTP $http_code | Failed"
fi

# 25. ZFS pools endpoint
print_test "25. Testing /zfs/pools"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/zfs/pools")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq 'length')
    print_pass "HTTP $http_code | ${time}s | ZFS Pools: $count"
else
    print_fail "HTTP $http_code | Failed"
fi

# 26. ZFS datasets endpoint
print_test "26. Testing /zfs/datasets"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/zfs/datasets")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq 'length')
    print_pass "HTTP $http_code | ${time}s | ZFS Datasets: $count"
else
    print_fail "HTTP $http_code | Failed"
fi

# 27. ZFS snapshots endpoint
print_test "27. Testing /zfs/snapshots"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/zfs/snapshots")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    count=$(echo "$body" | jq 'length')
    print_pass "HTTP $http_code | ${time}s | ZFS Snapshots: $count"
else
    print_fail "HTTP $http_code | Failed"
fi

# 28. ZFS ARC stats endpoint
print_test "28. Testing /zfs/arc"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/zfs/arc")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    hitratio=$(echo "$body" | jq -r '.hit_ratio_percent // "N/A"')
    print_pass "HTTP $http_code | ${time}s | ARC Hit Ratio: ${hitratio}%"
else
    print_fail "HTTP $http_code | Failed"
fi

# 29. Notifications overview endpoint
print_test "29. Testing /notifications/overview"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/notifications/overview")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    unread=$(echo "$body" | jq -r '.unread // 0')
    print_pass "HTTP $http_code | ${time}s | Unread: $unread"
else
    print_fail "HTTP $http_code | Failed"
fi

# 30. NUT endpoint
print_test "30. Testing /nut"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/nut")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    installed=$(echo "$body" | jq -r '.installed // "N/A"')
    print_pass "HTTP $http_code | ${time}s | NUT Installed: $installed"
else
    print_fail "HTTP $http_code | Failed"
fi

# 31. Collectors status endpoint
print_test "31. Testing /collectors/status"
response=$(curl -s -w "\n%{http_code}|%{time_total}" "${API_BASE}/collectors/status")
http_code=$(echo "$response" | tail -n1 | cut -d'|' -f1)
time=$(echo "$response" | tail -n1 | cut -d'|' -f2)
body=$(echo "$response" | sed '$d')
if [ "$http_code" = "200" ]; then
    total=$(echo "$body" | jq -r '.total // 0')
    enabled=$(echo "$body" | jq -r '.enabled_count // 0')
    print_pass "HTTP $http_code | ${time}s | Total: $total | Enabled: $enabled"
else
    print_fail "HTTP $http_code | Failed"
fi

# Performance monitoring
print_header "3. PERFORMANCE MONITORING"
print_test "Monitoring resource usage"
print_info "Current resource usage:"
eval "$SSH_CMD 'ps aux | grep unraid-management-agent | grep -v grep | awk '\"'\"'{print \"PID: \"\$2\" | CPU: \"\$3\"% | MEM: \"\$4\"% | RSS: \"\$6/1024\" MB | VSZ: \"\$5/1024\" MB\"}'\"'\"''"

print_info "Waiting 5 seconds to check for stability..."
sleep 5

print_info "Resource usage after 5 seconds:"
eval "$SSH_CMD 'ps aux | grep unraid-management-agent | grep -v grep | awk '\"'\"'{print \"PID: \"\$2\" | CPU: \"\$3\"% | MEM: \"\$4\"% | RSS: \"\$6/1024\" MB | VSZ: \"\$5/1024\" MB\"}'\"'\"''"

# Check logs for errors
print_header "4. LOG ANALYSIS"
print_test "Checking logs for errors and warnings"
print_info "Last 30 lines of log:"
eval "$SSH_CMD 'tail -30 /var/log/unraid-management-agent.log'" || print_info "Log file not found or empty"

print_test "Checking for errors in logs"
ERROR_COUNT=$(eval "$SSH_CMD 'grep -i error /var/log/unraid-management-agent.log | grep -v \"http: Server closed\" | wc -l'" || echo "0")
WARNING_COUNT=$(eval "$SSH_CMD 'grep -i warning /var/log/unraid-management-agent.log | grep -v \"Received terminated signal\" | wc -l'" || echo "0")

if [ "$ERROR_COUNT" -eq 0 ]; then
    print_pass "No errors found in logs"
else
    print_fail "Found $ERROR_COUNT errors in logs"
fi

if [ "$WARNING_COUNT" -eq 0 ]; then
    print_pass "No warnings found in logs"
else
    print_info "Found $WARNING_COUNT warnings in logs (may be normal)"
fi

# Check version
print_header "5. VERSION VERIFICATION"
print_test "Checking deployed version"
DEPLOYED_VERSION=$(eval "$SSH_CMD 'cat /usr/local/emhttp/plugins/unraid-management-agent/VERSION'" || echo "unknown")
EXPECTED_VERSION=$(cat VERSION)

if [ "$DEPLOYED_VERSION" = "$EXPECTED_VERSION" ]; then
    print_pass "Version matches: $DEPLOYED_VERSION"
else
    print_fail "Version mismatch: Deployed=$DEPLOYED_VERSION, Expected=$EXPECTED_VERSION"
fi

# Summary
print_header "VALIDATION SUMMARY"
echo "Total Tests: $TESTS_TOTAL"
echo -e "Passed: ${GREEN}${TESTS_PASSED}${NC}"
echo -e "Failed: ${RED}${TESTS_FAILED}${NC}"
echo ""
echo "Completed: $(date)"

