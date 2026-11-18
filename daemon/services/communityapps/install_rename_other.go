//go:build !linux && !darwin

package communityapps

import "errors"

func renameExclusive(_, _ string) error {
	return errors.New("exclusive file publication is unavailable on this platform")
}
