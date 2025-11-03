// Package domain provides core domain models and configuration structures for the Unraid Management Agent.
package domain

// Config holds the application configuration settings.
type Config struct {
	Version string `json:"version"`
	Port    int    `json:"port"`
}
