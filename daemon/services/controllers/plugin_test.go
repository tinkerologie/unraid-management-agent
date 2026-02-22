package controllers

import (
	"testing"
)

func TestNewPluginController(t *testing.T) {
	pc := NewPluginController()
	if pc == nil {
		t.Fatal("NewPluginController returned nil")
	}
}
