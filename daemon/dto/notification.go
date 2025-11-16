package dto

import "time"

// Notification represents a system notification
type Notification struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title,omitempty"`
	Subject            string    `json:"subject,omitempty"`
	Description        string    `json:"description,omitempty"`
	Importance         string    `json:"importance"` // "alert", "warning", "info"
	Link               string    `json:"link,omitempty"`
	Timestamp          time.Time `json:"timestamp"`
	FormattedTimestamp string    `json:"formatted_timestamp"`
	Type               string    `json:"type"` // "unread", "archive"
}

// NotificationOverview provides notification counts by type and importance
type NotificationOverview struct {
	Unread  NotificationCounts `json:"unread"`
	Archive NotificationCounts `json:"archive"`
}

// NotificationCounts contains counts by importance level
type NotificationCounts struct {
	Info    int `json:"info"`
	Warning int `json:"warning"`
	Alert   int `json:"alert"`
	Total   int `json:"total"`
}

// NotificationList groups notifications with overview
type NotificationList struct {
	Overview      NotificationOverview `json:"overview"`
	Notifications []Notification       `json:"notifications"`
	Timestamp     time.Time            `json:"timestamp"`
}
