#!/bin/bash
# MQTT Testing Helper for unraid-management-agent
# This script helps test MQTT connectivity and functionality

set -e

MQTT_BROKER="${MQTT_BROKER:-mosquitto}"
MQTT_PORT="${MQTT_PORT:-1883}"
MQTT_TOPIC="${MQTT_TOPIC:-unraid/#}"
WAIT_TIME="${WAIT_TIME:-5}"

echo "🔌 Unraid Management Agent - MQTT Testing Helper"
echo "=================================================="
echo ""
echo "MQTT Broker: $MQTT_BROKER:$MQTT_PORT"
echo "Test Topic: $MQTT_TOPIC"
echo ""

# Function to check MQTT broker connectivity
check_broker() {
    echo "🔍 Checking MQTT broker connectivity..."
    if nc -zv "$MQTT_BROKER" "$MQTT_PORT" 2>/dev/null; then
        echo "✅ MQTT Broker is accessible at $MQTT_BROKER:$MQTT_PORT"
        return 0
    else
        echo "❌ MQTT Broker is NOT accessible at $MQTT_BROKER:$MQTT_PORT"
        return 1
    fi
}

# Function to subscribe to MQTT topics
subscribe() {
    echo "📥 Subscribing to MQTT topics..."
    echo "   Press Ctrl+C to stop listening"
    echo ""
    mosquitto_sub -h "$MQTT_BROKER" -p "$MQTT_PORT" -t "$MQTT_TOPIC" -v
}

# Function to publish test message
publish_test() {
    local topic="$1"
    local message="$2"
    echo "📤 Publishing test message..."
    echo "   Topic: $topic"
    echo "   Message: $message"
    mosquitto_pub -h "$MQTT_BROKER" -p "$MQTT_PORT" -t "$topic" -m "$message"
    echo "✅ Message published"
}

# Function to monitor MQTT broker
monitor() {
    echo "📊 Monitoring MQTT broker system topics..."
    echo "   Press Ctrl+C to stop monitoring"
    echo ""
    mosquitto_sub -h "$MQTT_BROKER" -p "$MQTT_PORT" -t "\$SYS/broker/+/#" -v
}

# Main menu
show_menu() {
    echo ""
    echo "Available commands:"
    echo "  check    - Check MQTT broker connectivity"
    echo "  sub      - Subscribe to MQTT topics"
    echo "  pub      - Publish a test message"
    echo "  monitor  - Monitor broker system stats"
    echo "  help     - Show this help message"
    echo ""
}

# Parse command
case "${1:-help}" in
    check)
        check_broker
        ;;
    sub)
        subscribe
        ;;
    pub)
        if [ -z "$2" ] || [ -z "$3" ]; then
            echo "Usage: $0 pub <topic> <message>"
            echo "Example: $0 pub 'test/topic' 'hello world'"
            exit 1
        fi
        publish_test "$2" "$3"
        ;;
    monitor)
        monitor
        ;;
    help|--help|-h)
        show_menu
        ;;
    *)
        echo "Unknown command: $1"
        show_menu
        exit 1
        ;;
esac
