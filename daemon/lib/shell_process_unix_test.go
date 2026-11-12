//go:build darwin || linux

package lib

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCanceledCommandClosesDescendantOutputPipe(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ready := filepath.Join(t.TempDir(), "started")
	result := make(chan error, 1)
	go func() {
		_, err := ExecCommandOutputWithContext(ctx, "sh", "-c", `sleep 30 & : > "$1"; wait`, "fixture", ready)
		result <- err
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("command did not start its child")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("canceled command succeeded")
		}
	case <-time.After(950 * time.Millisecond):
		t.Fatal("descendant still held the canceled command's output pipe")
	}
}
