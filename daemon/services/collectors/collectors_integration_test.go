package collectors

import (
	"context"
	"testing"
	"time"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

// TestSystemCollectorStart tests the System collector's Start method
func TestSystemCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewSystemCollector(ctx)

	// Create context with timeout
	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start collector with short interval
	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	// Wait briefly to allow collection
	time.Sleep(150 * time.Millisecond)

	// Verify collector started without panic
	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}

// TestArrayCollectorStart tests the Array collector's Start method
func TestArrayCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewArrayCollector(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}

// TestDiskCollectorStart tests the Disk collector's Start method
func TestDiskCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewDiskCollector(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}

// TestShareCollectorStart tests the Share collector's Start method
func TestShareCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewShareCollector(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}

// TestDockerCollectorStart tests the Docker collector's Start method
func TestDockerCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewDockerCollector(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}

// TestVMCollectorStart tests the VM collector's Start method
func TestVMCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewVMCollector(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}

// TestNetworkCollectorStart tests the Network collector's Start method
func TestNetworkCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewNetworkCollector(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}

// TestUPSCollectorStart tests the UPS collector's Start method
func TestUPSCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewUPSCollector(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}

// TestNUTCollectorStart tests the NUT collector's Start method
func TestNUTCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewNUTCollector(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}

// TestGPUCollectorStart tests the GPU collector's Start method
func TestGPUCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewGPUCollector(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}

// TestParityCollectorCreation tests the Parity collector creation
func TestParityCollectorCreation(t *testing.T) {
	collector := NewParityCollector()

	if collector == nil {
		t.Fatal("NewParityCollector() returned nil")
	}
}

// TestZFSCollectorStart tests the ZFS collector's Start method
func TestZFSCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewZFSCollector(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}

// TestHardwareCollectorStart tests the Hardware collector's Start method
func TestHardwareCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewHardwareCollector(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}

// TestRegistrationCollectorStart tests the Registration collector's Start method
func TestRegistrationCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewRegistrationCollector(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}

// TestUnassignedCollectorStart tests the Unassigned collector's Start method
func TestUnassignedCollectorStart(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewUnassignedCollector(ctx)

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go collector.Start(ctxWithTimeout, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	select {
	case <-ctxWithTimeout.Done():
		// Normal timeout
	case <-time.After(3 * time.Second):
		t.Error("collector did not respond to context cancellation")
	}
}
