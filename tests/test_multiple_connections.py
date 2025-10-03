#!/usr/bin/env python3
"""
Test Multiple WebSocket Connections

This script tests the Unraid Management Agent's ability to handle multiple
concurrent WebSocket connections. It verifies:
- Maximum concurrent connections (10 clients)
- Connection stability under load
- Event broadcasting to all clients
- Connection rejection when limit exceeded
- Graceful handling of client disconnects

Usage:
    python test_multiple_connections.py ws://192.168.20.21:8043/api/v1/ws [num_clients] [duration]

Arguments:
    url         - WebSocket URL (required)
    num_clients - Number of concurrent clients to test (default: 12)
    duration    - Test duration in seconds (default: 60)
"""

import asyncio
import aiohttp
import sys
import json
from datetime import datetime
from collections import defaultdict
from typing import Dict, List, Any


class WebSocketClient:
    """Individual WebSocket client for testing"""
    
    def __init__(self, client_id: int, url: str):
        self.client_id = client_id
        self.url = url
        self.connected = False
        self.connection_time = None
        self.disconnect_time = None
        self.events_received = 0
        self.event_types = defaultdict(int)
        self.errors = []
        self.session = None
        self.ws = None
        
    async def connect(self):
        """Connect to WebSocket"""
        try:
            self.session = aiohttp.ClientSession()
            self.ws = await self.session.ws_connect(self.url)
            self.connected = True
            self.connection_time = datetime.now()
            print(f"[Client {self.client_id}] ✅ Connected at {self.connection_time.strftime('%H:%M:%S')}")
            return True
        except Exception as e:
            self.errors.append(f"Connection failed: {e}")
            print(f"[Client {self.client_id}] ❌ Connection failed: {e}")
            return False
    
    async def listen(self, duration: int):
        """Listen for events for specified duration"""
        if not self.connected:
            return
        
        try:
            end_time = asyncio.get_event_loop().time() + duration
            
            async for msg in self.ws:
                if asyncio.get_event_loop().time() > end_time:
                    break
                
                if msg.type == aiohttp.WSMsgType.TEXT:
                    try:
                        data = json.loads(msg.data)
                        event_type = self.identify_event_type(data.get('data', {}))
                        self.events_received += 1
                        self.event_types[event_type] += 1
                    except json.JSONDecodeError as e:
                        self.errors.append(f"JSON decode error: {e}")
                
                elif msg.type == aiohttp.WSMsgType.ERROR:
                    self.errors.append(f"WebSocket error: {msg.data}")
                    break
                
                elif msg.type == aiohttp.WSMsgType.CLOSED:
                    break
        
        except Exception as e:
            self.errors.append(f"Listen error: {e}")
        
        finally:
            await self.disconnect()
    
    async def disconnect(self):
        """Disconnect from WebSocket"""
        if self.ws and not self.ws.closed:
            await self.ws.close()
        if self.session:
            await self.session.close()
        
        self.connected = False
        self.disconnect_time = datetime.now()
        print(f"[Client {self.client_id}] 🔌 Disconnected at {self.disconnect_time.strftime('%H:%M:%S')}")
    
    def identify_event_type(self, data: Any) -> str:
        """Identify event type from data structure"""
        if isinstance(data, list):
            if not data:
                return "empty_list"
            data = data[0]
        
        if not isinstance(data, dict):
            return "unknown"
        
        # System update
        if "hostname" in data and "cpu_usage_percent" in data:
            return "system_update"
        
        # Array status
        if "state" in data and "parity_check_status" in data and "num_disks" in data:
            return "array_status_update"
        
        # UPS status
        if "connected" in data and "battery_charge_percent" in data:
            return "ups_status_update"
        
        # GPU metrics
        if "available" in data and "driver_version" in data and "utilization_gpu_percent" in data:
            return "gpu_update"
        
        # Network interface
        if "mac_address" in data and "bytes_received" in data:
            return "network_list_update"
        
        # Container
        if "image" in data and "ports" in data:
            return "container_list_update"
        
        # VM
        if "state" in data and "vcpus" in data:
            return "vm_list_update"
        
        # Disk
        if "device" in data and "mount_point" in data:
            return "disk_list_update"
        
        # Share
        if "name" in data and "path" in data and "size_bytes" in data:
            return "share_list_update"
        
        return "unknown"
    
    def get_stats(self) -> Dict[str, Any]:
        """Get client statistics"""
        duration = None
        if self.connection_time and self.disconnect_time:
            duration = (self.disconnect_time - self.connection_time).total_seconds()
        
        return {
            "client_id": self.client_id,
            "connected": self.connected,
            "connection_time": self.connection_time.isoformat() if self.connection_time else None,
            "disconnect_time": self.disconnect_time.isoformat() if self.disconnect_time else None,
            "duration_seconds": duration,
            "events_received": self.events_received,
            "event_types": dict(self.event_types),
            "errors": self.errors
        }


async def test_multiple_connections(url: str, num_clients: int, duration: int):
    """Test multiple concurrent WebSocket connections"""
    
    print(f"\n{'='*80}")
    print(f"MULTIPLE WEBSOCKET CONNECTIONS TEST")
    print(f"{'='*80}")
    print(f"URL: {url}")
    print(f"Number of Clients: {num_clients}")
    print(f"Test Duration: {duration} seconds")
    print(f"Max Allowed Clients: 10 (per server configuration)")
    print(f"{'='*80}\n")
    
    # Create clients
    clients = [WebSocketClient(i + 1, url) for i in range(num_clients)]
    
    # Phase 1: Connect all clients
    print(f"📡 PHASE 1: Connecting {num_clients} clients...\n")
    connect_tasks = [client.connect() for client in clients]
    connect_results = await asyncio.gather(*connect_tasks)
    
    successful_connections = sum(connect_results)
    failed_connections = num_clients - successful_connections
    
    print(f"\n✅ Successfully connected: {successful_connections}/{num_clients}")
    print(f"❌ Failed connections: {failed_connections}/{num_clients}\n")
    
    # Phase 2: Listen for events
    print(f"👂 PHASE 2: Listening for events ({duration} seconds)...\n")
    listen_tasks = [client.listen(duration) for client in clients if client.connected]
    await asyncio.gather(*listen_tasks)
    
    # Phase 3: Analyze results
    print(f"\n📊 PHASE 3: Analyzing results...\n")
    
    # Collect statistics
    all_stats = [client.get_stats() for client in clients]
    
    # Calculate aggregates
    total_events = sum(stats['events_received'] for stats in all_stats)
    avg_events = total_events / successful_connections if successful_connections > 0 else 0
    
    # Event type distribution
    event_type_totals = defaultdict(int)
    for stats in all_stats:
        for event_type, count in stats['event_types'].items():
            event_type_totals[event_type] += count
    
    # Print summary
    print(f"{'='*80}")
    print(f"TEST RESULTS SUMMARY")
    print(f"{'='*80}\n")
    
    print(f"Connection Statistics:")
    print(f"  - Total clients attempted: {num_clients}")
    print(f"  - Successful connections: {successful_connections}")
    print(f"  - Failed connections: {failed_connections}")
    print(f"  - Connection success rate: {(successful_connections/num_clients)*100:.1f}%\n")
    
    print(f"Event Statistics:")
    print(f"  - Total events received: {total_events}")
    print(f"  - Average events per client: {avg_events:.1f}")
    print(f"  - Events per second (avg): {total_events/duration:.2f}\n")
    
    print(f"Event Type Distribution:")
    for event_type, count in sorted(event_type_totals.items(), key=lambda x: x[1], reverse=True):
        percentage = (count / total_events * 100) if total_events > 0 else 0
        print(f"  - {event_type}: {count} ({percentage:.1f}%)")
    
    # Per-client details
    print(f"\n{'='*80}")
    print(f"PER-CLIENT DETAILS")
    print(f"{'='*80}\n")
    
    for stats in all_stats:
        status = "✅ Connected" if stats['connection_time'] else "❌ Failed"
        print(f"Client {stats['client_id']}: {status}")
        if stats['connection_time']:
            print(f"  - Duration: {stats['duration_seconds']:.1f}s")
            print(f"  - Events received: {stats['events_received']}")
            if stats['errors']:
                print(f"  - Errors: {len(stats['errors'])}")
                for error in stats['errors'][:3]:  # Show first 3 errors
                    print(f"    • {error}")
        print()
    
    # Save detailed results
    results = {
        "test_info": {
            "url": url,
            "num_clients": num_clients,
            "duration": duration,
            "timestamp": datetime.now().isoformat()
        },
        "summary": {
            "successful_connections": successful_connections,
            "failed_connections": failed_connections,
            "total_events": total_events,
            "avg_events_per_client": avg_events,
            "events_per_second": total_events / duration
        },
        "event_type_distribution": dict(event_type_totals),
        "client_stats": all_stats
    }
    
    output_file = "multiple_connections_test_results.json"
    with open(output_file, 'w') as f:
        json.dump(results, f, indent=2)
    
    print(f"{'='*80}")
    print(f"✅ Detailed results saved to: {output_file}")
    print(f"{'='*80}\n")
    
    # Verification
    print(f"{'='*80}")
    print(f"VERIFICATION")
    print(f"{'='*80}\n")
    
    if num_clients <= 10:
        if successful_connections == num_clients:
            print(f"✅ PASS: All {num_clients} clients connected successfully (within limit)")
        else:
            print(f"⚠️  WARNING: Only {successful_connections}/{num_clients} clients connected")
    else:
        if successful_connections == 10:
            print(f"✅ PASS: Exactly 10 clients connected (max limit enforced)")
            print(f"✅ PASS: {failed_connections} clients rejected as expected")
        elif successful_connections < 10:
            print(f"⚠️  WARNING: Only {successful_connections}/10 clients connected")
        else:
            print(f"❌ FAIL: {successful_connections} clients connected (exceeds limit of 10)")
    
    if total_events > 0:
        print(f"✅ PASS: Events received and distributed to clients")
    else:
        print(f"❌ FAIL: No events received")
    
    # Check event consistency
    if successful_connections > 1:
        event_counts = [stats['events_received'] for stats in all_stats if stats['events_received'] > 0]
        if event_counts:
            min_events = min(event_counts)
            max_events = max(event_counts)
            variance = max_events - min_events
            variance_percent = (variance / max_events * 100) if max_events > 0 else 0
            
            print(f"\nEvent Distribution Consistency:")
            print(f"  - Min events: {min_events}")
            print(f"  - Max events: {max_events}")
            print(f"  - Variance: {variance} ({variance_percent:.1f}%)")
            
            if variance_percent < 10:
                print(f"  ✅ PASS: Events distributed evenly across clients")
            else:
                print(f"  ⚠️  WARNING: Significant variance in event distribution")
    
    print(f"\n{'='*80}\n")


def main():
    """Main entry point"""
    if len(sys.argv) < 2:
        print("Usage: python test_multiple_connections.py <websocket_url> [num_clients] [duration]")
        print("Example: python test_multiple_connections.py ws://192.168.20.21:8043/api/v1/ws 12 60")
        sys.exit(1)
    
    url = sys.argv[1]
    num_clients = int(sys.argv[2]) if len(sys.argv) > 2 else 12
    duration = int(sys.argv[3]) if len(sys.argv) > 3 else 60
    
    asyncio.run(test_multiple_connections(url, num_clients, duration))


if __name__ == "__main__":
    main()

