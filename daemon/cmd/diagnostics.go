package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/diagnostics"
)

// Diagnostics collects system diagnostic information and creates a ZIP archive for troubleshooting.
type Diagnostics struct {
	OutputDir string `default:"." help:"directory to write the diagnostic archive"`
}

// Run executes the diagnostics command by collecting system data and creating a ZIP bundle.
func (d *Diagnostics) Run(ctx *domain.Context) error {
	_, _ = fmt.Fprintln(os.Stderr, "Collecting diagnostic information...")

	svc := diagnostics.NewBundleService(ctx)
	bundle, err := svc.CollectDiagnostics(context.Background())
	if err != nil {
		return fmt.Errorf("collecting diagnostics: %w", err)
	}

	_, _ = fmt.Fprintln(os.Stderr, "Creating diagnostic archive...")

	outputPath, err := diagnostics.CreateArchive(bundle, d.OutputDir)
	if err != nil {
		return fmt.Errorf("creating archive: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stderr, "Diagnostic bundle created: %s\n", outputPath)
	return nil
}
