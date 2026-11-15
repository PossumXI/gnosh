// Package clipboard puts text on the clipboard of the machine gnosh runs
// on, with the machine's own tool: pbcopy on macOS; wl-copy, xclip or
// xsel on a Linux desktop. It is for when the person's terminal is on
// this machine (gnosh was not reached over ssh): /copy uses it, and so
// does gnosh ssh for what the far side copies. Apple's Terminal drops the
// terminal's own way of copying (OSC 52), so without it nothing reached
// the clipboard there.
package clipboard

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// wait is how long the tool is given.
const wait = 5 * time.Second

// Tool is the program, with its arguments, that puts text on this
// machine's clipboard: nil when the person's terminal is on another
// machine, or this one has none. goos is runtime.GOOS; getenv looks up
// the environment gnosh was given, and look finds a program on its PATH.
func Tool(goos string, getenv func(string) string, look func(string) (string, error)) []string {
	if getenv("SSH_CONNECTION") != "" || getenv("SSH_CLIENT") != "" || getenv("SSH_TTY") != "" {
		return nil
	}
	try := func(name string, args ...string) []string {
		if path, err := look(name); err == nil {
			return append([]string{path}, args...)
		}
		return nil
	}
	switch goos {
	case "darwin":
		return try("pbcopy")
	case "windows", "plan9", "js", "wasip1":
		return nil
	}
	if getenv("WAYLAND_DISPLAY") != "" {
		if tool := try("wl-copy"); tool != nil {
			return tool
		}
	}
	if getenv("DISPLAY") != "" {
		if tool := try("xclip", "-selection", "clipboard"); tool != nil {
			return tool
		}
		return try("xsel", "--clipboard", "--input")
	}
	return nil
}

// Copy runs tool with text on its standard input, in the environment env
// (as os.Environ gives it; nil is this process's own).
func Copy(ctx context.Context, tool []string, env []string, text string) error {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool[0], tool[1:]...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(text)
	// Its output goes nowhere, not into a pipe: wl-copy and xclip leave a
	// process behind to hand the text out, which would hold a pipe open.
	return cmd.Run()
}
