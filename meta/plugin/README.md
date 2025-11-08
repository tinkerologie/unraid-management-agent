####Unraid Management Agent####
REST API and WebSocket integration for Unraid server monitoring and control

This plugin provides comprehensive system monitoring and control capabilities through REST API and WebSocket interfaces, enabling integration with Home Assistant and other automation platforms.

**Features:**
- Real-time system metrics (CPU, RAM, temperatures, uptime)
- Array status and disk information with SMART data
- Docker container management and monitoring
- VM management and control
- Network interface statistics
- UPS status monitoring (if available)
- GPU metrics (if available)
- User share information
- WebSocket support for real-time updates

**API Endpoint:** http://[server-ip]:8043/api/v1
**WebSocket:** ws://[server-ip]:8043/api/v1/ws

**Configuration:** Settings → Unraid Management Agent

**Support:** https://forums.unraid.net/topic/178262-home-assistant-unraid-integration
**Documentation:** https://github.com/ruaan-deysel/unraid-management-agent

