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
if $SSH_CMD "ps aux | grep -v grep | grep unraid-management-agent" > /dev/null 2>&1; then
    print_pass "Service is running"
    $SSH_CMD "ps aux | grep -v grep | grep unraid-management-agent"
else
    print_fail "Service is not running"
    exit 1
fi

# Test health endpoint
print_header "2. HEALTH CHECK"
if test_endpoint "/health" "Health check"; then
    :
fi

# Test system endpoint
print_header "3. SYSTEM INFORMATION"
print_test "Fetching system information from API"
SYSTEM_DATA=$(curl -s "${API_BASE}/system")
echo "$SYSTEM_DATA" | jq '.' 2>/dev/null || echo "$SYSTEM_DATA"

print_test "Comparing with actual system state"
print_info "Getting actual system information via SSH"
$SSH_CMD "uname -a"
$SSH_CMD "uptime"
$SSH_CMD "free -h"
$SSH_CMD "df -h | head -5"

# Validate system data fields
if echo "$SYSTEM_DATA" | jq -e '.hostname' > /dev/null 2>&1; then
    HOSTNAME=$(echo "$SYSTEM_DATA" | jq -r '.hostname')
    ACTUAL_HOSTNAME=$($SSH_CMD "hostname")
    if [ "$HOSTNAME" = "$ACTUAL_HOSTNAME" ]; then
        print_pass "Hostname matches: $HOSTNAME"
    else
        print_fail "Hostname mismatch: API=$HOSTNAME, Actual=$ACTUAL_HOSTNAME"
    fi
else
    print_fail "System data missing hostname field"
fi

# Test array endpoint
print_header "4. ARRAY STATUS"
print_test "Fetching array status from API"
ARRAY_DATA=$(curl -s "${API_BASE}/array")
echo "$ARRAY_DATA" | jq '.' 2>/dev/null || echo "$ARRAY_DATA"

print_test "Comparing with actual array state"
$SSH_CMD "cat /var/local/emhttp/var.ini | grep -E '(mdState|mdNumDisks|mdNumInvalid)'"

# Test disks endpoint
print_header "5. DISK INFORMATION"
print_test "Fetching disk information from API"
DISK_DATA=$(curl -s "${API_BASE}/disks")
echo "$DISK_DATA" | jq '.' 2>/dev/null || echo "$DISK_DATA"

DISK_COUNT=$(echo "$DISK_DATA" | jq '. | length' 2>/dev/null || echo "0")
print_info "API reports $DISK_COUNT disks"

print_test "Comparing with actual disk state"
$SSH_CMD "lsblk -o NAME,SIZE,TYPE,MOUNTPOINT"
$SSH_CMD "df -h | grep -E '(^/dev|Filesystem)'"

# Test Docker endpoint
print_header "6. DOCKER CONTAINERS"
print_test "Fetching Docker container information from API"
DOCKER_DATA=$(curl -s "${API_BASE}/docker/containers")
echo "$DOCKER_DATA" | jq '.' 2>/dev/null || echo "$DOCKER_DATA"

CONTAINER_COUNT=$(echo "$DOCKER_DATA" | jq '. | length' 2>/dev/null || echo "0")
print_info "API reports $CONTAINER_COUNT containers"

print_test "Comparing with actual Docker state"
$SSH_CMD "docker ps -a --format 'table {{.Names}}\t{{.Status}}\t{{.State}}'"

# Test VM endpoint
print_header "7. VIRTUAL MACHINES"
print_test "Fetching VM information from API"
VM_DATA=$(curl -s "${API_BASE}/vms")
echo "$VM_DATA" | jq '.' 2>/dev/null || echo "$VM_DATA"

VM_COUNT=$(echo "$VM_DATA" | jq '. | length' 2>/dev/null || echo "0")
print_info "API reports $VM_COUNT VMs"

print_test "Comparing with actual VM state"
$SSH_CMD "virsh list --all 2>/dev/null || echo 'Libvirt not available'"

# Test network endpoint
print_header "8. NETWORK INTERFACES"
print_test "Fetching network information from API"
NETWORK_DATA=$(curl -s "${API_BASE}/network")
echo "$NETWORK_DATA" | jq '.' 2>/dev/null || echo "$NETWORK_DATA"

INTERFACE_COUNT=$(echo "$NETWORK_DATA" | jq '. | length' 2>/dev/null || echo "0")
print_info "API reports $INTERFACE_COUNT network interfaces"

print_test "Comparing with actual network state"
$SSH_CMD "ip -br addr"
$SSH_CMD "ip -s link"

# Test shares endpoint
print_header "9. USER SHARES"
print_test "Fetching share information from API"
SHARE_DATA=$(curl -s "${API_BASE}/shares")
echo "$SHARE_DATA" | jq '.' 2>/dev/null || echo "$SHARE_DATA"

SHARE_COUNT=$(echo "$SHARE_DATA" | jq '. | length' 2>/dev/null || echo "0")
print_info "API reports $SHARE_COUNT shares"

print_test "Comparing with actual share state"
$SSH_CMD "ls -la /mnt/user/"

# Test UPS endpoint
print_header "10. UPS STATUS"
print_test "Fetching UPS information from API"
UPS_DATA=$(curl -s "${API_BASE}/ups")
echo "$UPS_DATA" | jq '.' 2>/dev/null || echo "$UPS_DATA"

# Test GPU endpoint
print_header "11. GPU INFORMATION"
print_test "Fetching GPU information from API"
GPU_DATA=$(curl -s "${API_BASE}/gpu")
echo "$GPU_DATA" | jq '.' 2>/dev/null || echo "$GPU_DATA"

print_test "Comparing with actual GPU state"
$SSH_CMD "lspci | grep -i vga"
$SSH_CMD "nvidia-smi 2>/dev/null || echo 'nvidia-smi not available'"

# Performance monitoring
print_header "12. PERFORMANCE MONITORING"
print_test "Monitoring resource usage"
print_info "Initial resource usage:"
$SSH_CMD "ps aux | grep unraid-management-agent | grep -v grep | awk '{print \"CPU: \"\$3\"% MEM: \"\$4\"% RSS: \"\$6\" KB\"}'"

print_info "Waiting 10 seconds to check for stability..."
sleep 10

print_info "Resource usage after 10 seconds:"
$SSH_CMD "ps aux | grep unraid-management-agent | grep -v grep | awk '{print \"CPU: \"\$3\"% MEM: \"\$4\"% RSS: \"\$6\" KB\"}'"

# Check logs for errors
print_header "13. LOG ANALYSIS"
print_test "Checking logs for errors"
$SSH_CMD "tail -50 /var/log/unraid-management-agent.log" || print_info "Log file not found or empty"

# Summary
print_header "VALIDATION SUMMARY"
echo "Total Tests: $TESTS_TOTAL"
echo -e "Passed: ${GREEN}${TESTS_PASSED}${NC}"
echo -e "Failed: ${RED}${TESTS_FAILED}${NC}"
echo ""
echo "Completed: $(date)"

