package dto

import "time"

// DockerNetworkInfo represents a single Docker network with its configuration and connected containers.
type DockerNetworkInfo struct {
	ID             string            `json:"id"`
	Name           string            `json:"name" example:"bridge"`
	Driver         string            `json:"driver" example:"bridge"`
	Scope          string            `json:"scope" example:"local"`
	Internal       bool              `json:"internal"`
	Attachable     bool              `json:"attachable"`
	Subnet         string            `json:"subnet,omitempty" example:"172.17.0.0/16"`
	Gateway        string            `json:"gateway,omitempty" example:"172.17.0.1"`
	ContainerNames []string          `json:"container_names"`
	Created        string            `json:"created,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	Timestamp      time.Time         `json:"timestamp"`
}

// DockerNetworkList is the envelope published on TopicDockerNetworksUpdate and
// served by the REST / MCP network endpoints.
type DockerNetworkList struct {
	Networks  []DockerNetworkInfo `json:"networks"`
	Count     int                 `json:"count"`
	Timestamp time.Time           `json:"timestamp"`
}
