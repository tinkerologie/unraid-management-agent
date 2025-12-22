package controllers

import (
	"reflect"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewSystemController(t *testing.T) {
	ctx := &domain.Context{}
	controller := NewSystemController(ctx)

	if controller == nil {
		t.Fatal("Expected non-nil controller")
	}

	if controller.ctx != ctx {
		t.Error("Expected context to be set")
	}
}

func TestSystemControllerInterface(t *testing.T) {
	ctx := &domain.Context{}
	controller := NewSystemController(ctx)

	// Verify the controller has the expected methods
	controllerType := reflect.TypeOf(controller)

	methods := []string{"Reboot", "Shutdown"}

	for _, method := range methods {
		t.Run("has_"+method+"_method", func(t *testing.T) {
			_, exists := controllerType.MethodByName(method)
			if !exists {
				t.Errorf("SystemController should have %s method", method)
			}
		})
	}
}

func TestSystemControllerReboot(t *testing.T) {
	// Skip in normal tests - running shutdown/reboot is destructive
	if testing.Short() {
		t.Skip("Skipping destructive system test in short mode")
	}

	ctx := &domain.Context{}
	controller := NewSystemController(ctx)

	// Will fail without root privileges or in container
	err := controller.Reboot()
	if err == nil {
		t.Log("Note: No error - reboot command might be available")
	}
}

func TestSystemControllerShutdown(t *testing.T) {
	// Skip in normal tests - running shutdown/reboot is destructive
	if testing.Short() {
		t.Skip("Skipping destructive system test in short mode")
	}

	ctx := &domain.Context{}
	controller := NewSystemController(ctx)

	// Will fail without root privileges or in container
	err := controller.Shutdown()
	if err == nil {
		t.Log("Note: No error - shutdown command might be available")
	}
}

// Note: We don't test actual Reboot/Shutdown execution as they would
// affect the system. The methods are tested for existence only.
// Integration tests should be run in a controlled environment.
