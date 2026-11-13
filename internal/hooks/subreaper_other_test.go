//go:build !linux

package hooks

import "errors"

// becomeSubreaper is something only Linux can do.
func becomeSubreaper() error {
	return errors.ErrUnsupported
}
