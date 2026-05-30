package dto

import "time"

const (
	// OSUpdateStatusUpToDate indicates the OS is at the latest known version.
	OSUpdateStatusUpToDate = "up_to_date"
	// OSUpdateStatusAvailable indicates a newer OS version is available locally.
	OSUpdateStatusAvailable = "update_available"
	// OSUpdateStatusUnknown indicates no local latest-version data was found.
	OSUpdateStatusUnknown = "unknown"
)

// OSUpdateStatus represents the result of a best-effort local OS update check.
// All data is sourced from local files only — no outbound network calls are made.
type OSUpdateStatus struct {
	// CurrentVersion is the running Unraid OS version (e.g. "7.2.0").
	CurrentVersion string `json:"current_version" example:"7.2.0"`
	// LatestVersion is the newest version found in a local candidate file, if any.
	LatestVersion string `json:"latest_version,omitempty" example:"7.2.1"`
	// UpdateAvailable is true when LatestVersion != "" and LatestVersion != CurrentVersion.
	UpdateAvailable bool `json:"update_available" example:"false"`
	// Status is one of: "up_to_date", "update_available", "unknown".
	Status string `json:"status" example:"unknown"`
	// Timestamp records when the check was performed.
	Timestamp time.Time `json:"timestamp"`
}
