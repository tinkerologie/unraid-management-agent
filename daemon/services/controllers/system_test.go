package controllers

import (
	"errors"
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
	_ = NewSystemController(ctx)

	// Verify the controller has the expected methods
	controllerType := reflect.TypeFor[*SystemController]()

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

func TestNewSystemControllerWithExec(t *testing.T) {
	ctx := &domain.Context{}
	called := false
	mockExec := func(_ string, _ ...string) ([]string, error) {
		called = true
		return []string{}, nil
	}

	controller := NewSystemControllerWithExec(ctx, mockExec)
	if controller == nil {
		t.Fatal("Expected non-nil controller")
	}

	err := controller.Reboot()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !called {
		t.Error("Expected mockExec to be called")
	}
}

func TestSystemControllerReboot_Success(t *testing.T) {
	ctx := &domain.Context{}
	var recordedCommand string
	var recordedArgs []string

	mockExec := func(cmd string, args ...string) ([]string, error) {
		recordedCommand = cmd
		recordedArgs = args
		return []string{"Shutdown scheduled"}, nil
	}

	controller := NewSystemControllerWithExec(ctx, mockExec)
	err := controller.Reboot()
	if err != nil {
		t.Fatalf("Reboot returned unexpected error: %v", err)
	}

	if recordedCommand != "/sbin/shutdown" {
		t.Errorf("Expected command /sbin/shutdown, got %s", recordedCommand)
	}
	if len(recordedArgs) != 2 || recordedArgs[0] != "-r" || recordedArgs[1] != "now" {
		t.Errorf("Expected args [-r now], got %v", recordedArgs)
	}
}

func TestSystemControllerReboot_Error(t *testing.T) {
	ctx := &domain.Context{}
	mockExec := func(_ string, _ ...string) ([]string, error) {
		return nil, errors.New("command failed")
	}

	controller := NewSystemController(ctx)
	controller.SetExec(mockExec)

	err := controller.Reboot()
	if err == nil {
		t.Fatal("Expected error from Reboot when exec fails, got nil")
	}
}

func TestSystemControllerShutdown_Success(t *testing.T) {
	ctx := &domain.Context{}
	var recordedCommand string
	var recordedArgs []string

	mockExec := func(cmd string, args ...string) ([]string, error) {
		recordedCommand = cmd
		recordedArgs = args
		return []string{"Shutdown scheduled"}, nil
	}

	controller := NewSystemControllerWithExec(ctx, mockExec)
	err := controller.Shutdown()
	if err != nil {
		t.Fatalf("Shutdown returned unexpected error: %v", err)
	}

	if recordedCommand != "/sbin/shutdown" {
		t.Errorf("Expected command /sbin/shutdown, got %s", recordedCommand)
	}
	if len(recordedArgs) != 2 || recordedArgs[0] != "-h" || recordedArgs[1] != "now" {
		t.Errorf("Expected args [-h now], got %v", recordedArgs)
	}
}

func TestSystemControllerShutdown_Error(t *testing.T) {
	ctx := &domain.Context{}
	mockExec := func(_ string, _ ...string) ([]string, error) {
		return nil, errors.New("command failed")
	}

	controller := NewSystemControllerWithExec(ctx, mockExec)
	err := controller.Shutdown()
	if err == nil {
		t.Fatal("Expected error from Shutdown when exec fails, got nil")
	}
}
