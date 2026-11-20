//go:build !linux && !darwin

package sandbox

// check says there is no sandbox here.
func check() Status { return Status{Why: ErrUnsupported.Error()} }

// Wrap refuses: this system has no sandbox gnosh can use.
func Wrap(p Policy, dir string, argv []string) ([]string, error) {
	return nil, ErrUnsupported
}
