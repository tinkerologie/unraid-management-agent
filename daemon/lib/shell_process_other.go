//go:build !darwin && !linux

package lib

import "os/exec"

func cancelProcessGroup(_ *exec.Cmd) {}
