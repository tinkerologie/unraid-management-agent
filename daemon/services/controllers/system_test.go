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
		t.Error("Expected non-nil controller")
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

// Note: We don't test actual Reboot/Shutdown execution as they would
// affect the system. The methods are tested for existence only.
// Integration tests should be run in a controlled environment.
