//go:build !linux

package lib

import (
	"errors"
	"io"
)

// OpenUevents is only implemented on Linux.
func OpenUevents() (io.ReadCloser, error) {
	return nil, errors.ErrUnsupported
}
