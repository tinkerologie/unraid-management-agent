// Package cmd provides command implementations for the Unraid Management Agent.
package cmd

import (
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services"
)

// Boot represents the boot command that starts the Unraid Management Agent.
type Boot struct{}

// Run executes the boot command by creating and running the orchestrator.
func (b *Boot) Run(ctx *domain.Context) error {
	return services.CreateOrchestrator(ctx).Run()
}
