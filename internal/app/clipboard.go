package app

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// This file puts text on the clipboard for /copy. When the person's
// terminal is on the machine gnosh runs on (gnosh was not reached over
// ssh), the machine's own tool does it: pbcopy on macOS, wl-copy, xclip or
// xsel on a Linux desktop. Apple's Terminal drops the terminal's own way
// (OSC 52), so without this /copy copied nothing there. Over ssh, and
// where no tool is found or the tool fails, the front end sends OSC 52.

// clipboardWait is how long the tool is given.
const clipboardWait = 5 * time.Second

// clipboardTool is the program, with its arguments, that puts text on this
// machine's clipboard: nil when the person's terminal is on another
// machine, or this one has none. goos is runtime.GOOS; look finds a
// program on the PATH.
func clipboardTool(goos string, getenv func(string) string, look func(string) (string, error)) []string {
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

// copyWith runs tool with text on its standard input, in the environment
// gnosh was given.
func (p *program) copyWith(ctx context.Context, tool []string, text string) error {
	ctx, cancel := context.WithTimeout(ctx, clipboardWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool[0], tool[1:]...)
	cmd.Env = p.o.Environ
	cmd.Stdin = strings.NewReader(text)
	// Its output goes nowhere, not into a pipe: wl-copy and xclip leave a
	// process behind to hand the text out, which would hold a pipe open.
	return cmd.Run()
}
