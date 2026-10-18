//go:build linux

package lib

import (
	"errors"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestOpenUevents_CloseUnblocksRead(t *testing.T) {
	conn, err := OpenUevents()
	if err != nil {
		t.Skipf("uevent socket unavailable: %v", err)
	}

	readErr := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := conn.Read(buf); err != nil {
				readErr <- err
				return
			}
		}
	}()
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-readErr:
		if err == nil {
			t.Error("Read after Close returned no error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not unblock Read")
	}
}

func TestOpenUevents_Errors(t *testing.T) {
	socket, bind := ueventSocket, ueventBind
	defer func() { ueventSocket, ueventBind = socket, bind }()

	ueventSocket = func(int, int, int) (int, error) { return -1, syscall.EPERM }
	if _, err := OpenUevents(); err == nil || !errors.Is(err, syscall.EPERM) || !strings.Contains(err.Error(), "creating") {
		t.Errorf("socket failure: err = %v, want a wrapped EPERM", err)
	}

	// Bind fails on a real socket.
	ueventSocket = unix.Socket
	ueventBind = func(int, unix.Sockaddr) error { return syscall.EADDRINUSE }
	if _, err := OpenUevents(); err == nil || !errors.Is(err, syscall.EADDRINUSE) || !strings.Contains(err.Error(), "binding") {
		t.Errorf("bind failure: err = %v, want a wrapped EADDRINUSE", err)
	}
}
