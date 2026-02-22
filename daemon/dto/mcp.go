// Package dto contains data transfer objects for the MCP (Model Context Protocol) server.
package dto

// MCPEmptyArgs represents tool arguments for tools that require no parameters.
type MCPEmptyArgs struct{}

// MCPDiskArgs represents arguments for disk-related tools.
type MCPDiskArgs struct {
	DiskID       string `json:"disk_id,omitempty" jsonschema:"The disk identifier (e.g. disk1, cache, parity)"`
	IncludeSmart bool   `json:"include_smart,omitempty" jsonschema:"Include SMART health data in the response"`
}

// MCPContainerArgs represents arguments for container-related tools.
type MCPContainerArgs struct {
	ContainerID string `json:"container_id" jsonschema:"The Docker container ID or name"`
}

// MCPContainerActionArgs represents arguments for container control actions.
type MCPContainerActionArgs struct {
	ContainerID string `json:"container_id" jsonschema:"The Docker container ID or name"`
	Action      string `json:"action" jsonschema:"The action to perform: start, stop, restart, pause, or unpause"`
}

// MCPContainerListArgs represents arguments for listing containers.
type MCPContainerListArgs struct {
	State string `json:"state,omitempty" jsonschema:"Filter containers by state: running, stopped, or all (default: all)"`
}

// MCPVMArgs represents arguments for VM-related tools.
type MCPVMArgs struct {
	VMName string `json:"vm_name" jsonschema:"The virtual machine name"`
}

// MCPVMActionArgs represents arguments for VM control actions.
type MCPVMActionArgs struct {
	VMName string `json:"vm_name" jsonschema:"The virtual machine name"`
	Action string `json:"action" jsonschema:"The action to perform: start, stop, restart, pause, resume, hibernate, or force-stop"`
}

// MCPVMListArgs represents arguments for listing VMs.
type MCPVMListArgs struct {
	State string `json:"state,omitempty" jsonschema:"Filter VMs by state: running, stopped, or all (default: all)"`
}

// MCPNotificationArgs represents arguments for notification-related tools.
type MCPNotificationArgs struct {
	Type     string `json:"type,omitempty" jsonschema:"Filter notifications by type: alert, warning, normal, or all (default: all)"`
	Archived bool   `json:"archived,omitempty" jsonschema:"Include archived notifications"`
}

// MCPParityCheckArgs represents arguments for parity check operations.
type MCPParityCheckArgs struct {
	Correcting bool `json:"correcting,omitempty" jsonschema:"Whether to perform a correcting parity check (writes corrections)"`
}

// MCPSystemActionArgs represents arguments for system control actions.
type MCPSystemActionArgs struct {
	Confirm bool `json:"confirm" jsonschema:"Must be set to true to confirm the action - prevents accidental execution"`
}

// MCPArrayActionArgs represents arguments for array control actions.
type MCPArrayActionArgs struct {
	Action  string `json:"action" jsonschema:"The action to perform on the array: start or stop"`
	Confirm bool   `json:"confirm" jsonschema:"Must be set to true to confirm the action"`
}

// MCPShareArgs represents arguments for share-related tools.
type MCPShareArgs struct {
	ShareName string `json:"share_name,omitempty" jsonschema:"The name of a specific share to retrieve"`
}

// MCPLogArgs represents arguments for log retrieval tools.
type MCPLogArgs struct {
	LogFile string `json:"log_file,omitempty" jsonschema:"Specific log file to retrieve (e.g. syslog, docker.log)"`
	Lines   int    `json:"lines,omitempty" jsonschema:"Number of recent lines to retrieve (default: 100, max: 1000)"`
}

// MCPZFSPoolArgs represents arguments for ZFS pool operations.
type MCPZFSPoolArgs struct {
	PoolName string `json:"pool_name,omitempty" jsonschema:"The name of a specific ZFS pool"`
}

// MCPUserScriptArgs represents arguments for user script execution.
type MCPUserScriptArgs struct {
	ScriptName string `json:"script_name" jsonschema:"The name of the user script to execute"`
	Confirm    bool   `json:"confirm" jsonschema:"Must be set to true to confirm script execution"`
}

// MCPCollectorArgs represents arguments for collector-related tools.
type MCPCollectorArgs struct {
	CollectorName string `json:"collector_name,omitempty" jsonschema:"The name of a specific collector (e.g. system, docker, vm, array, disk)"`
}

// MCPCollectorControlArgs represents arguments for collector control actions.
type MCPCollectorControlArgs struct {
	CollectorName string `json:"collector_name" jsonschema:"The name of the collector to control"`
	Action        string `json:"action" jsonschema:"The action to perform on the collector: enable or disable"`
}

// MCPCollectorIntervalArgs represents arguments for updating collector intervals.
type MCPCollectorIntervalArgs struct {
	CollectorName string `json:"collector_name" jsonschema:"The name of the collector to update"`
	Interval      int    `json:"interval" jsonschema:"The new collection interval in seconds (5-3600)"`
}

// MCPCreateNotificationArgs represents arguments for creating a notification.
type MCPCreateNotificationArgs struct {
	Title       string `json:"title" jsonschema:"Notification title/event name"`
	Subject     string `json:"subject" jsonschema:"Notification subject line"`
	Description string `json:"description" jsonschema:"Notification description/body text"`
	Importance  string `json:"importance" jsonschema:"Notification importance level: alert, warning, or info"`
	Link        string `json:"link,omitempty" jsonschema:"Optional link URL for the notification"`
}

// MCPNotificationActionArgs represents arguments for notification actions.
type MCPNotificationActionArgs struct {
	NotificationID string `json:"notification_id" jsonschema:"The notification ID (filename)"`
	Action         string `json:"action" jsonschema:"The action to perform: archive, unarchive, or delete"`
	IsArchived     bool   `json:"is_archived,omitempty" jsonschema:"Set to true if the notification is in the archive (for delete action)"`
}

// MCPSettingsArgs represents arguments for settings-related tools.
type MCPSettingsArgs struct {
	Category string `json:"category,omitempty" jsonschema:"Settings category to retrieve: system, docker, vm, or disk"`
}

// MCPSearchArgs represents arguments for search/filter operations.
type MCPSearchArgs struct {
	Query string `json:"query" jsonschema:"Search query or filter text"`
	Type  string `json:"type,omitempty" jsonschema:"Type of items to search: containers, vms, shares, disks, or logs"`
}

// MCPContainerUpdateArgs represents arguments for container update operations.
type MCPContainerUpdateArgs struct {
	ContainerID string `json:"container_id,omitempty" jsonschema:"The Docker container ID or name (omit to check/update all containers)"`
	Force       bool   `json:"force,omitempty" jsonschema:"Force update even if no update is detected"`
	Confirm     bool   `json:"confirm" jsonschema:"Must be set to true to confirm the update operation"`
}

// MCPContainerSizeArgs represents arguments for container size operations.
type MCPContainerSizeArgs struct {
	ContainerID string `json:"container_id" jsonschema:"The Docker container ID or name"`
}

// MCPVMSnapshotArgs represents arguments for VM snapshot operations.
type MCPVMSnapshotArgs struct {
	VMName       string `json:"vm_name" jsonschema:"The virtual machine name"`
	SnapshotName string `json:"snapshot_name,omitempty" jsonschema:"Name for the snapshot (auto-generated if empty)"`
	Description  string `json:"description,omitempty" jsonschema:"Optional description for the snapshot"`
}

// MCPVMCloneArgs represents arguments for VM clone operations.
type MCPVMCloneArgs struct {
	VMName    string `json:"vm_name" jsonschema:"The source virtual machine name to clone"`
	CloneName string `json:"clone_name" jsonschema:"Name for the cloned virtual machine"`
	Confirm   bool   `json:"confirm" jsonschema:"Must be set to true to confirm the clone operation"`
}

// MCPServiceStatusArgs represents arguments for read-only service status queries.
type MCPServiceStatusArgs struct {
	ServiceName string `json:"service_name" jsonschema:"The service name: docker, libvirt, smb, nfs, ftp, sshd, nginx, syslog, ntpd, avahi, or wireguard"`
}

// MCPServiceActionArgs represents arguments for service control actions.
type MCPServiceActionArgs struct {
	ServiceName string `json:"service_name" jsonschema:"The service name: docker, libvirt, smb, nfs, ftp, sshd, nginx, syslog, ntpd, avahi, or wireguard"`
	Action      string `json:"action" jsonschema:"The action to perform: start, stop, or restart"`
	Confirm     bool   `json:"confirm" jsonschema:"Must be set to true to confirm the action"`
}

// MCPPluginUpdateArgs represents arguments for plugin update operations.
type MCPPluginUpdateArgs struct {
	PluginName string `json:"plugin_name,omitempty" jsonschema:"Specific plugin name to update (omit to update all with available updates)"`
	Confirm    bool   `json:"confirm" jsonschema:"Must be set to true to confirm the update"`
}

// MCPProcessListArgs represents arguments for listing processes.
type MCPProcessListArgs struct {
	SortBy string `json:"sort_by,omitempty" jsonschema:"Sort by: cpu, memory, pid, or name (default: cpu)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of processes to return (default: 50, max: 500)"`
}

// MCPContainerLogsArgs represents arguments for retrieving container logs.
type MCPContainerLogsArgs struct {
	ContainerID string `json:"container_id" jsonschema:"The Docker container ID or name"`
	Tail        int    `json:"tail,omitempty" jsonschema:"Number of recent log lines to retrieve (default: 100, max: 5000)"`
	Since       string `json:"since,omitempty" jsonschema:"Only return logs since this timestamp (RFC3339 format, e.g. 2026-02-17T00:00:00Z)"`
	Timestamps  bool   `json:"timestamps,omitempty" jsonschema:"Include timestamps in log output"`
}

// MCPVMSnapshotRestoreArgs represents arguments for restoring a VM snapshot.
type MCPVMSnapshotRestoreArgs struct {
	VMName       string `json:"vm_name" jsonschema:"The virtual machine name"`
	SnapshotName string `json:"snapshot_name" jsonschema:"Name of the snapshot to restore"`
	Confirm      bool   `json:"confirm" jsonschema:"Must be set to true to confirm the restore operation - this will revert the VM to the snapshot state"`
}

// MCPAlertRuleIDArgs represents arguments for operations on a specific alert rule.
type MCPAlertRuleIDArgs struct {
	RuleID string `json:"rule_id" jsonschema:"The unique identifier of the alert rule"`
}

// MCPCreateAlertRuleArgs represents arguments for creating a new alert rule.
type MCPCreateAlertRuleArgs struct {
	ID              string   `json:"id" jsonschema:"Unique identifier for the alert rule (e.g. high-cpu, disk-temp-warn)"`
	Name            string   `json:"name" jsonschema:"Human-readable name for the alert rule"`
	Expression      string   `json:"expression" jsonschema:"expr-lang boolean expression evaluated against system metrics (e.g. CPU > 90, MaxDiskTemp > 55)"`
	Severity        string   `json:"severity,omitempty" jsonschema:"Alert severity level: info, warning, or critical (default: warning)"`
	DurationSeconds int      `json:"duration_seconds,omitempty" jsonschema:"Seconds the expression must be true before firing (0 = immediate)"`
	CooldownMinutes int      `json:"cooldown_minutes,omitempty" jsonschema:"Minutes between re-fires of the same alert (default: 5)"`
	Channels        []string `json:"channels,omitempty" jsonschema:"Notification channels: shoutrrr URLs or 'unraid' for local notification"`
	Enabled         bool     `json:"enabled,omitempty" jsonschema:"Whether the rule is enabled (default: true when created)"`
}

// MCPDeleteAlertRuleArgs represents arguments for deleting an alert rule.
type MCPDeleteAlertRuleArgs struct {
	RuleID  string `json:"rule_id" jsonschema:"The unique identifier of the alert rule to delete"`
	Confirm bool   `json:"confirm" jsonschema:"Must be set to true to confirm deletion"`
}

// MCPHealthCheckIDArgs represents arguments for operations on a specific health check.
type MCPHealthCheckIDArgs struct {
	CheckID string `json:"check_id" jsonschema:"The unique identifier of the health check"`
}

// MCPCreateHealthCheckArgs represents arguments for creating a new health check.
type MCPCreateHealthCheckArgs struct {
	ID              string `json:"id" jsonschema:"Unique identifier for the health check (e.g. plex-http, nginx-tcp)"`
	Name            string `json:"name" jsonschema:"Human-readable name for the health check"`
	Type            string `json:"type" jsonschema:"Probe type: http, tcp, or container"`
	Target          string `json:"target" jsonschema:"Probe target: URL for http, host:port for tcp, container ID/name for container"`
	IntervalSeconds int    `json:"interval_seconds,omitempty" jsonschema:"Check interval in seconds (min 10, default 30)"`
	TimeoutSeconds  int    `json:"timeout_seconds,omitempty" jsonschema:"Probe timeout in seconds (default 5)"`
	SuccessCode     int    `json:"success_code,omitempty" jsonschema:"Expected HTTP status code for http probes (default 200)"`
	OnFail          string `json:"on_fail,omitempty" jsonschema:"Remediation action: notify, restart_container:<id>, or webhook:<url>"`
	Enabled         bool   `json:"enabled,omitempty" jsonschema:"Whether the health check is enabled (default: true when created)"`
}

// MCPDeleteHealthCheckArgs represents arguments for deleting a health check.
type MCPDeleteHealthCheckArgs struct {
	CheckID string `json:"check_id" jsonschema:"The unique identifier of the health check to delete"`
	Confirm bool   `json:"confirm" jsonschema:"Must be set to true to confirm deletion"`
}

// MCPRunHealthCheckArgs represents arguments for manually running a health check.
type MCPRunHealthCheckArgs struct {
	CheckID string `json:"check_id" jsonschema:"The unique identifier of the health check to run"`
}
