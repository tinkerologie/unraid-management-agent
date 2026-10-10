//go:build integration

package controllers

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/moby/moby/client"
)

func skipIfDockerUnavailable(t *testing.T, dc *DockerController) {
	t.Helper()
	if err := dc.initClient(); err != nil {
		t.Skipf("skipping: Docker client unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := dc.client.Ping(ctx, client.PingOptions{}); err != nil {
		t.Skipf("skipping: Docker daemon unavailable: %v", err)
	}
}

func skipIfLibvirtUnavailable(t *testing.T) {
	t.Helper()
	uri, _ := url.Parse(string(libvirt.QEMUSystem))
	l, err := libvirt.ConnectToURI(uri)
	if err != nil {
		t.Skipf("skipping: libvirt unavailable: %v", err)
	}
	_ = l.Disconnect()
}

// ===== Docker Controller - SDK-based methods (requires live Docker daemon) =====

func TestDockerControllerCheckContainerUpdate(t *testing.T) {
	dc := NewDockerController()
	defer dc.Close() //nolint:errcheck
	skipIfDockerUnavailable(t, dc)

	result, err := dc.CheckContainerUpdate(context.Background(), "nonexistent-container-id")
	if err == nil {
		t.Fatal("expected error for nonexistent container ID, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil result on error, got: %+v", result)
	}
}

func TestDockerControllerCheckAllContainerUpdates(t *testing.T) {
	dc := NewDockerController()
	defer dc.Close() //nolint:errcheck
	skipIfDockerUnavailable(t, dc)

	result, err := dc.CheckAllContainerUpdates(context.Background())
	if err != nil {
		t.Fatalf("CheckAllContainerUpdates failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from CheckAllContainerUpdates")
	}
}

func TestDockerControllerGetContainerSize(t *testing.T) {
	dc := NewDockerController()
	defer dc.Close() //nolint:errcheck
	skipIfDockerUnavailable(t, dc)

	result, err := dc.GetContainerSize("nonexistent-container-id")
	if err == nil {
		t.Fatal("expected error for nonexistent container ID, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil result on error, got: %+v", result)
	}
}

func TestDockerControllerUpdateContainer(t *testing.T) {
	dc := NewDockerController()
	defer dc.Close() //nolint:errcheck
	skipIfDockerUnavailable(t, dc)

	result, err := dc.UpdateContainer("nonexistent-container-id", false)
	if err == nil {
		t.Fatal("expected error for nonexistent container ID, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil result on error, got: %+v", result)
	}
}

func TestDockerControllerUpdateAllContainers(t *testing.T) {
	if os.Getenv("TEST_LIVE_DOCKER_UPDATE") != "true" {
		t.Skip("skipping live Docker update test; set TEST_LIVE_DOCKER_UPDATE=true to enable")
	}
	dc := NewDockerController()
	defer dc.Close() //nolint:errcheck
	skipIfDockerUnavailable(t, dc)

	result, err := dc.UpdateAllContainers()
	if err != nil {
		t.Fatalf("UpdateAllContainers failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from UpdateAllContainers")
	}
}

// ===== VM Controller - Snapshot and Clone (requires live libvirt) =====

func TestVMControllerCreateSnapshot(t *testing.T) {
	skipIfLibvirtUnavailable(t)
	ctrl := NewVMController()
	err := ctrl.CreateSnapshot("nonexistent-vm", "test-snap", "test description")
	if err == nil {
		t.Fatal("expected error for nonexistent VM snapshot creation, got nil")
	}
}

func TestVMControllerListSnapshots(t *testing.T) {
	skipIfLibvirtUnavailable(t)
	ctrl := NewVMController()
	result, err := ctrl.ListSnapshots("nonexistent-vm")
	if err == nil {
		t.Fatal("expected error for nonexistent VM snapshot listing, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil result on error, got: %+v", result)
	}
}

func TestVMControllerDeleteSnapshot(t *testing.T) {
	skipIfLibvirtUnavailable(t)
	ctrl := NewVMController()
	err := ctrl.DeleteSnapshot("nonexistent-vm", "test-snap")
	if err == nil {
		t.Fatal("expected error for nonexistent VM snapshot deletion, got nil")
	}
}

func TestVMControllerCloneVM(t *testing.T) {
	skipIfLibvirtUnavailable(t)
	ctrl := NewVMController()
	err := ctrl.CloneVM("nonexistent-vm", "clone-vm")
	if err == nil {
		t.Fatal("expected error for nonexistent VM clone, got nil")
	}
}
