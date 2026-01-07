package dto

import "time"

// Notification represents a system notification
type Notification struct {
	ID                 string    `json:"id" example:"notification_1234567890"`
	Title              string    `json:"title,omitempty" example:"Array Started"`
	Subject            string    `json:"subject,omitempty" example:"Array Status"`
	Description        string    `json:"description,omitempty" example:"The array has been successfully started"`
	Importance         string    `json:"importance" example:"info"` // "alert", "warning", "info"
	Link               string    `json:"link,omitempty" example:"/Dashboard"`
	Timestamp          time.Time `json:"timestamp"`
	FormattedTimestamp string    `json:"formatted_timestamp" example:"2025-01-15 10:30:00"`
	Type               string    `json:"type" example:"unread"` // "unread", "archive"
}

// NotificationOverview provides notification counts by type and importance
type NotificationOverview struct {
	Unread  NotificationCounts `json:"unread"`
	Archive NotificationCounts `json:"archive"`
}

// NotificationCounts contains counts by importance level
type NotificationCounts struct {
	Info    int `json:"info" example:"5"`
	Warning int `json:"warning" example:"2"`
	Alert   int `json:"alert" example:"0"`
	Total   int `json:"total" example:"7"`
}

// NotificationList groups notifications with overview
type NotificationList struct {
	Overview      NotificationOverview `json:"overview"`
	Notifications []Notification       `json:"notifications"`
	Timestamp     time.Time            `json:"timestamp"`
}
