// Package dto contains data transfer objects for the MCP (Model Context Protocol) server.
package dto

// MCPEmptyArgs represents tool arguments for tools that require no parameters.
type MCPEmptyArgs struct{}

// MCPDiskArgs represents arguments for disk-related tools.
type MCPDiskArgs struct {
	DiskID       string `json:"disk_id,omitempty" jsonschema:"description=The disk identifier (e.g. 'disk1'\\, 'cache'\\, 'parity')"`
	IncludeSmart bool   `json:"include_smart,omitempty" jsonschema:"description=Include SMART health data in the response"`
}

// MCPContainerArgs represents arguments for container-related tools.
type MCPContainerArgs struct {
	ContainerID string `json:"container_id" jsonschema:"required,description=The Docker container ID or name"`
}

// MCPContainerActionArgs represents arguments for container control actions.
type MCPContainerActionArgs struct {
	ContainerID string `json:"container_id" jsonschema:"required,description=The Docker container ID or name"`
	Action      string `json:"action" jsonschema:"required,enum=start,enum=stop,enum=restart,enum=pause,enum=unpause,description=The action to perform on the container"`
}

// MCPContainerListArgs represents arguments for listing containers.
type MCPContainerListArgs struct {
	State string `json:"state,omitempty" jsonschema:"enum=running,enum=stopped,enum=all,description=Filter containers by state (default: all)"`
}

// MCPVMArgs represents arguments for VM-related tools.
type MCPVMArgs struct {
	VMName string `json:"vm_name" jsonschema:"required,description=The virtual machine name"`
}

// MCPVMActionArgs represents arguments for VM control actions.
type MCPVMActionArgs struct {
	VMName string `json:"vm_name" jsonschema:"required,description=The virtual machine name"`
	Action string `json:"action" jsonschema:"required,enum=start,enum=stop,enum=restart,enum=pause,enum=resume,enum=hibernate,enum=force-stop,description=The action to perform on the VM"`
}

// MCPVMListArgs represents arguments for listing VMs.
type MCPVMListArgs struct {
	State string `json:"state,omitempty" jsonschema:"enum=running,enum=stopped,enum=all,description=Filter VMs by state (default: all)"`
}

// MCPNotificationArgs represents arguments for notification-related tools.
type MCPNotificationArgs struct {
	Type     string `json:"type,omitempty" jsonschema:"enum=alert,enum=warning,enum=normal,enum=all,description=Filter notifications by type (default: all)"`
	Archived bool   `json:"archived,omitempty" jsonschema:"description=Include archived notifications"`
}

// MCPParityCheckArgs represents arguments for parity check operations.
type MCPParityCheckArgs struct {
	Correcting bool `json:"correcting,omitempty" jsonschema:"description=Whether to perform a correcting parity check (writes corrections)"`
}

// MCPSystemActionArgs represents arguments for system control actions.
type MCPSystemActionArgs struct {
	Confirm bool `json:"confirm" jsonschema:"required,description=Must be set to true to confirm the action - this prevents accidental execution"`
}

// MCPArrayActionArgs represents arguments for array control actions.
type MCPArrayActionArgs struct {
	Action  string `json:"action" jsonschema:"required,enum=start,enum=stop,description=The action to perform on the array"`
	Confirm bool   `json:"confirm" jsonschema:"required,description=Must be set to true to confirm the action"`
}

// MCPShareArgs represents arguments for share-related tools.
type MCPShareArgs struct {
	ShareName string `json:"share_name,omitempty" jsonschema:"description=The name of a specific share to retrieve"`
}

// MCPLogArgs represents arguments for log retrieval tools.
type MCPLogArgs struct {
	LogFile string `json:"log_file,omitempty" jsonschema:"description=Specific log file to retrieve (e.g. 'syslog'\\, 'docker.log')"`
	Lines   int    `json:"lines,omitempty" jsonschema:"description=Number of recent lines to retrieve (default: 100\\, max: 1000)"`
}

// MCPZFSPoolArgs represents arguments for ZFS pool operations.
type MCPZFSPoolArgs struct {
	PoolName string `json:"pool_name,omitempty" jsonschema:"description=The name of a specific ZFS pool"`
}

// MCPUserScriptArgs represents arguments for user script execution.
type MCPUserScriptArgs struct {
	ScriptName string `json:"script_name" jsonschema:"required,description=The name of the user script to execute"`
	Confirm    bool   `json:"confirm" jsonschema:"required,description=Must be set to true to confirm script execution"`
}

// MCPCollectorArgs represents arguments for collector-related tools.
type MCPCollectorArgs struct {
	CollectorName string `json:"collector_name,omitempty" jsonschema:"description=The name of a specific collector (e.g. 'system'\\, 'docker'\\, 'vm'\\, 'array'\\, 'disk')"`
}

// MCPCollectorControlArgs represents arguments for collector control actions.
type MCPCollectorControlArgs struct {
	CollectorName string `json:"collector_name" jsonschema:"required,description=The name of the collector to control"`
	Action        string `json:"action" jsonschema:"required,enum=enable,enum=disable,description=The action to perform on the collector"`
}

// MCPCollectorIntervalArgs represents arguments for updating collector intervals.
type MCPCollectorIntervalArgs struct {
	CollectorName string `json:"collector_name" jsonschema:"required,description=The name of the collector to update"`
	Interval      int    `json:"interval" jsonschema:"required,description=The new collection interval in seconds (5-3600)"`
}

// MCPCreateNotificationArgs represents arguments for creating a notification.
type MCPCreateNotificationArgs struct {
	Title       string `json:"title" jsonschema:"required,description=Notification title/event name"`
	Subject     string `json:"subject" jsonschema:"required,description=Notification subject line"`
	Description string `json:"description" jsonschema:"required,description=Notification description/body text"`
	Importance  string `json:"importance" jsonschema:"required,enum=alert,enum=warning,enum=info,description=Notification importance level"`
	Link        string `json:"link,omitempty" jsonschema:"description=Optional link URL for the notification"`
}

// MCPNotificationActionArgs represents arguments for notification actions.
type MCPNotificationActionArgs struct {
	NotificationID string `json:"notification_id" jsonschema:"required,description=The notification ID (filename)"`
	Action         string `json:"action" jsonschema:"required,enum=archive,enum=unarchive,enum=delete,description=The action to perform"`
	IsArchived     bool   `json:"is_archived,omitempty" jsonschema:"description=Set to true if the notification is in the archive (for delete action)"`
}

// MCPSettingsArgs represents arguments for settings-related tools.
type MCPSettingsArgs struct {
	Category string `json:"category,omitempty" jsonschema:"enum=system,enum=docker,enum=vm,enum=disk,description=Settings category to retrieve"`
}

// MCPSearchArgs represents arguments for search/filter operations.
type MCPSearchArgs struct {
	Query string `json:"query" jsonschema:"required,description=Search query or filter text"`
	Type  string `json:"type,omitempty" jsonschema:"enum=containers,enum=vms,enum=shares,enum=disks,enum=logs,description=Type of items to search"`
}
