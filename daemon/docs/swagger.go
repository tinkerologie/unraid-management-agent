// Package docs provides Swagger/OpenAPI documentation for the Unraid Management Agent API.
package docs

// General API Info
//
//	@title						Unraid Management Agent API
//	@version					2025.12.1
//	@description				REST API and WebSocket interface for comprehensive Unraid system monitoring and control.
//	@description				This is a third-party community plugin providing an alternative/complement to the official Unraid GraphQL API.
//
//	@contact.name				GitHub Issues
//	@contact.url				https://github.com/ruaan-deysel/unraid-management-agent/issues
//
//	@license.name				MIT
//	@license.url				https://github.com/ruaan-deysel/unraid-management-agent/blob/main/LICENSE
//
//	@host						localhost:8043
//	@BasePath					/api/v1
//	@schemes					http https
//
//	@tag.name					System
//	@tag.description			System monitoring and control endpoints (CPU, RAM, temps, reboot/shutdown)
//	@tag.name					Array
//	@tag.description			Unraid array status and parity check control
//	@tag.name					Disks
//	@tag.description			Disk information and SMART data
//	@tag.name					Docker
//	@tag.description			Docker container monitoring and lifecycle control
//	@tag.name					VMs
//	@tag.description			Virtual machine monitoring and lifecycle control
//	@tag.name					Network
//	@tag.description			Network interface information and access URLs
//	@tag.name					Shares
//	@tag.description			User share information and configuration
//	@tag.name					Hardware
//	@tag.description			Hardware information (BIOS, CPU, Memory) via DMI
//	@tag.name					UPS
//	@tag.description			UPS/NUT status monitoring
//	@tag.name					GPU
//	@tag.description			GPU metrics (NVIDIA/AMD)
//	@tag.name					ZFS
//	@tag.description			ZFS pool, dataset, snapshot, and ARC statistics
//	@tag.name					Notifications
//	@tag.description			System notification management
//	@tag.name					Configuration
//	@tag.description			Configuration endpoints for shares and system settings
//	@tag.name					Logs
//	@tag.description			Log file access
//	@tag.name					User Scripts
//	@tag.description			User script management and execution
//	@tag.name					Collectors
//	@tag.description			Runtime collector management (enable/disable/interval)
//	@tag.name					WebSocket
//	@tag.description			Real-time event streaming via WebSocket
//	@tag.name					Unassigned Devices
//	@tag.description			Unassigned devices and remote shares
//	@tag.name					Plugins
//	@tag.description			Installed plugin list and versions
//	@tag.name					Updates
//	@tag.description			Unraid OS and plugin update availability
