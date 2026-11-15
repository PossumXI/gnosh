package clipboard

import (
	"os/exec"
	"strings"
	"testing"
)

// Tool picks the machine's own tool only where the terminal is on
// this machine, and the one the desktop takes.
func TestTool(t *testing.T) {
	has := func(names ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			for _, n := range names {
				if n == name {
					return "/bin/" + name, nil
				}
			}
			return "", exec.ErrNotFound
		}
	}
	env := func(kv ...string) func(string) string {
		return func(name string) string {
			for i := 0; i+1 < len(kv); i += 2 {
				if kv[i] == name {
					return kv[i+1]
				}
			}
			return ""
		}
	}
	all := has("pbcopy", "wl-copy", "xclip", "xsel")
	for _, c := range []struct {
		goos string
		env  func(string) string
		look func(string) (string, error)
		want string
	}{
		{"darwin", env(), all, "/bin/pbcopy"},
		{"darwin", env("SSH_CONNECTION", "203.0.113.7 1 203.0.113.8 22"), all, ""},
		{"darwin", env("SSH_TTY", "/dev/ttys001"), all, ""},
		{"darwin", env(), has(), ""},
		{"linux", env(), all, ""},
		{"linux", env("WAYLAND_DISPLAY", "wayland-0", "DISPLAY", ":0"), all, "/bin/wl-copy"},
		{"linux", env("WAYLAND_DISPLAY", "wayland-0", "DISPLAY", ":0"), has("xclip"), "/bin/xclip -selection clipboard"},
		{"linux", env("DISPLAY", ":0"), has("xsel"), "/bin/xsel --clipboard --input"},
		{"linux", env("DISPLAY", ":0", "SSH_CLIENT", "203.0.113.7 1 22"), all, ""},
		{"windows", env(), all, ""},
	} {
		if got := strings.Join(Tool(c.goos, c.env, c.look), " "); got != c.want {
			t.Errorf("%s: Tool = %q, want %q", c.goos, got, c.want)
		}
	}
}
