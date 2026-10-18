//go:build !linux

package lib

import (
	"errors"
	"testing"
)

func TestOpenUevents_Unsupported(t *testing.T) {
	if _, err := OpenUevents(); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("OpenUevents error = %v, want ErrUnsupported", err)
	}
}
