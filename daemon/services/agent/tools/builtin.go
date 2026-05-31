package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
)

// StateProvider supplies read-only cache snapshots. Satisfied by the API server.
// Each method returns the value and whether it is currently available.
type StateProvider interface {
	SystemJSON() (any, bool)
	ArrayJSON() (any, bool)
	DockerJSON() (any, bool)
}

// DockerActor performs low-risk container actions. Satisfied by controllers.DockerController.
type DockerActor interface {
	Restart(id string) error
}

func marshalState(v any, ok bool, label string) (string, error) {
	if !ok {
		return label + " not available yet", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", label, err)
	}
	return string(b), nil
}

// BuildDefault wires the Phase-1 tool set: read-only reads + low-risk Docker restart.
func BuildDefault(state StateProvider, docker DockerActor) *Registry {
	r := NewRegistry()

	r.Register(Tool{
		Name: "get_system_info", RiskTier: dto.RiskReadOnly,
		Description: "Get current system info: CPU, RAM, temperatures, uptime.",
		Invoke: func(_ context.Context, _ string) (string, error) {
			v, ok := state.SystemJSON()
			return marshalState(v, ok, "system info")
		},
	})
	r.Register(Tool{
		Name: "get_array_status", RiskTier: dto.RiskReadOnly,
		Description: "Get the Unraid array status (state, capacity, parity).",
		Invoke: func(_ context.Context, _ string) (string, error) {
			v, ok := state.ArrayJSON()
			return marshalState(v, ok, "array status")
		},
	})
	r.Register(Tool{
		Name: "list_docker_containers", RiskTier: dto.RiskReadOnly,
		Description: "List Docker containers and their current state.",
		Invoke: func(_ context.Context, _ string) (string, error) {
			v, ok := state.DockerJSON()
			return marshalState(v, ok, "docker containers")
		},
	})

	r.Register(Tool{
		Name: "restart_container", RiskTier: dto.RiskLow,
		Description: "Restart a Docker container by ID. Low-risk, reversible.",
		Schema:      []byte(`{"type":"object","properties":{"container_id":{"type":"string"}},"required":["container_id"]}`),
		Invoke: func(_ context.Context, argsJSON string) (string, error) {
			var a struct {
				ContainerID string `json:"container_id"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
				return "", fmt.Errorf("parse args: %w", err)
			}
			if err := lib.ValidateContainerID(a.ContainerID); err != nil {
				return "", err
			}
			if err := docker.Restart(a.ContainerID); err != nil {
				return "", err
			}
			return fmt.Sprintf("Container %s restarted.", a.ContainerID), nil
		},
	})

	return r
}
