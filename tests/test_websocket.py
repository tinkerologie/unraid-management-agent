#!/usr/bin/env python3
"""
WebSocket Test Client for Unraid Management Agent

Tests the WebSocket endpoint and documents all events received.
"""

import asyncio
import json
import sys
from datetime import datetime
from collections import defaultdict
import websockets

# Configuration
WS_URL = "ws://192.168.20.21:8043/api/v1/ws"
TEST_DURATION = 120  # 2 minutes (reduced for faster testing)
CONNECTION_TIMEOUT = 10

# Event tracking
events_received = defaultdict(list)
event_counts = defaultdict(int)
event_timestamps = defaultdict(list)
connection_events = []


def log(message):
    """Log message with timestamp"""
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] {message}")


def identify_event_type(data):
    """Identify event type based on data structure"""
    if not isinstance(data, dict):
        return "unknown"

    # Check for specific fields to identify event type
    if "hostname" in data and "cpu_usage_percent" in data:
        return "system_update"
    elif "state" in data and "parity_check_status" in data and "num_disks" in data:
        return "array_status_update"
    elif "device" in data and "mount_point" in data:
        return "disk_list_update"
    elif "name" in data and "path" in data and "size_bytes" in data:
        return "share_list_update"
    elif "image" in data and "ports" in data and ("id" in data or "container_id" in data):
        return "container_list_update"
    elif ("vm_name" in data or "name" in data) and "state" in data and "vcpus" in data:
        return "vm_list_update"
    elif "connected" in data and "battery_charge_percent" in data:
        return "ups_status_update"
    elif "available" in data and "driver_version" in data and "utilization_gpu_percent" in data:
        return "gpu_update"
    elif "mac_address" in data and "bytes_received" in data:
        return "network_list_update"
    else:
        return "unknown"


def save_event(event_type, payload):
    """Save event for analysis"""
    timestamp = datetime.now()
    event_counts[event_type] += 1
    event_timestamps[event_type].append(timestamp)

    # Save first 3 examples of each event type
    if len(events_received[event_type]) < 3:
        events_received[event_type].append({
            "timestamp": timestamp.isoformat(),
            "payload": payload
        })


async def test_websocket_connection():
    """Test WebSocket connection and monitor events"""
    log("=" * 80)
    log("WebSocket Test Client Starting")
    log(f"Target: {WS_URL}")
    log(f"Test Duration: {TEST_DURATION} seconds")
    log("=" * 80)
    
    try:
        log("Attempting to connect...")
        connection_events.append({
            "event": "connection_attempt",
            "timestamp": datetime.now().isoformat()
        })
        
        async with websockets.connect(
            WS_URL,
            ping_interval=20,
            ping_timeout=10,
            close_timeout=10
        ) as websocket:
            log("✅ WebSocket connection established successfully!")
            connection_events.append({
                "event": "connection_established",
                "timestamp": datetime.now().isoformat()
            })
            
            # Start time
            start_time = datetime.now()
            last_event_time = start_time
            
            log(f"Monitoring events for {TEST_DURATION} seconds...")
            log("Press Ctrl+C to stop early")
            log("-" * 80)
            
            while True:
                # Check if test duration exceeded
                elapsed = (datetime.now() - start_time).total_seconds()
                if elapsed >= TEST_DURATION:
                    log(f"\n✅ Test duration ({TEST_DURATION}s) completed")
                    break
                
                try:
                    # Wait for message with timeout
                    message = await asyncio.wait_for(
                        websocket.recv(),
                        timeout=30.0
                    )
                    
                    # Parse message
                    try:
                        data = json.loads(message)

                        # Identify event type based on data structure
                        event_data = data.get("data", {})
                        if isinstance(event_data, list) and len(event_data) > 0:
                            # For list events, check first item
                            event_type = identify_event_type(event_data[0])
                        else:
                            event_type = identify_event_type(event_data)

                        # Log event
                        now = datetime.now()
                        time_since_last = (now - last_event_time).total_seconds()
                        last_event_time = now

                        log(f"📨 Event: {event_type:30s} (Δt: {time_since_last:6.2f}s)")

                        # Save event
                        save_event(event_type, data)
                        
                    except json.JSONDecodeError as e:
                        log(f"⚠️  Failed to parse JSON: {e}")
                        log(f"   Raw message: {message[:200]}")
                        
                except asyncio.TimeoutError:
                    log("⚠️  No message received for 30 seconds")
                    
                except websockets.exceptions.ConnectionClosed as e:
                    log(f"❌ Connection closed: {e}")
                    connection_events.append({
                        "event": "connection_closed",
                        "timestamp": datetime.now().isoformat(),
                        "reason": str(e)
                    })
                    break
                    
    except websockets.exceptions.WebSocketException as e:
        log(f"❌ WebSocket error: {e}")
        connection_events.append({
            "event": "connection_error",
            "timestamp": datetime.now().isoformat(),
            "error": str(e)
        })
        return False
        
    except Exception as e:
        log(f"❌ Unexpected error: {e}")
        connection_events.append({
            "event": "unexpected_error",
            "timestamp": datetime.now().isoformat(),
            "error": str(e)
        })
        return False
    
    return True


def analyze_events():
    """Analyze collected events"""
    log("\n" + "=" * 80)
    log("EVENT ANALYSIS")
    log("=" * 80)
    
    if not event_counts:
        log("❌ No events received!")
        return
    
    # Event counts
    log("\n📊 Event Counts:")
    for event_type in sorted(event_counts.keys()):
        count = event_counts[event_type]
        log(f"   {event_type:30s}: {count:4d} events")
    
    # Event frequencies
    log("\n⏱️  Event Frequencies:")
    for event_type in sorted(event_timestamps.keys()):
        timestamps = event_timestamps[event_type]
        if len(timestamps) >= 2:
            # Calculate average interval
            intervals = []
            for i in range(1, len(timestamps)):
                interval = (timestamps[i] - timestamps[i-1]).total_seconds()
                intervals.append(interval)
            
            avg_interval = sum(intervals) / len(intervals)
            min_interval = min(intervals)
            max_interval = max(intervals)
            
            log(f"   {event_type:30s}: avg={avg_interval:6.2f}s  min={min_interval:6.2f}s  max={max_interval:6.2f}s")
        else:
            log(f"   {event_type:30s}: insufficient data (only {len(timestamps)} event)")
    
    # Expected frequencies
    log("\n✅ Expected Frequencies:")
    expected = {
        "system_update": 5,
        "gpu_update": 10,
        "array_status_update": 10,
        "network_list_update": 15,
        "container_list_update": 30,
    }
    
    for event_type, expected_interval in expected.items():
        if event_type in event_timestamps:
            timestamps = event_timestamps[event_type]
            if len(timestamps) >= 2:
                intervals = []
                for i in range(1, len(timestamps)):
                    interval = (timestamps[i] - timestamps[i-1]).total_seconds()
                    intervals.append(interval)
                avg_interval = sum(intervals) / len(intervals)
                
                diff = abs(avg_interval - expected_interval)
                status = "✅" if diff < 2 else "⚠️"
                log(f"   {status} {event_type:30s}: expected={expected_interval:2d}s  actual={avg_interval:6.2f}s")
            else:
                log(f"   ⚠️  {event_type:30s}: expected={expected_interval:2d}s  actual=insufficient data")
        else:
            log(f"   ❌ {event_type:30s}: expected={expected_interval:2d}s  actual=NOT RECEIVED")


def save_results():
    """Save results to file"""
    log("\n" + "=" * 80)
    log("SAVING RESULTS")
    log("=" * 80)
    
    results = {
        "test_info": {
            "url": WS_URL,
            "duration": TEST_DURATION,
            "timestamp": datetime.now().isoformat()
        },
        "connection_events": connection_events,
        "event_counts": dict(event_counts),
        "event_examples": {
            event_type: examples
            for event_type, examples in events_received.items()
        }
    }
    
    filename = "websocket_test_results.json"
    with open(filename, "w") as f:
        json.dump(results, f, indent=2)
    
    log(f"✅ Results saved to: {filename}")


async def main():
    """Main function"""
    try:
        success = await test_websocket_connection()
        analyze_events()
        save_results()
        
        if success:
            log("\n✅ WebSocket test completed successfully!")
            return 0
        else:
            log("\n❌ WebSocket test failed!")
            return 1
            
    except KeyboardInterrupt:
        log("\n⚠️  Test interrupted by user")
        analyze_events()
        save_results()
        return 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))

