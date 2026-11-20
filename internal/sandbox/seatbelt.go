package sandbox

import (
	"strings"
)

// seatbeltProfile is the Seatbelt profile (macOS's sandbox language) for
// the policy: everything is allowed but writing outside the folders the
// policy names and a few devices, writing the read-only paths, reading
// the denied ones (but those opened again inside them), and the network.
// A later rule wins over an earlier one, which is what makes the order
// the rule. The paths must be the system's own spelling of them, links
// resolved (/private/tmp, not /tmp): Seatbelt matches what it is asked
// about, which is that.
func seatbeltProfile(p Policy) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n")
	// What would start a program outside the sandbox: an Apple Event to
	// another application, a launchd job, opening a file or an app through
	// Launch Services (the sandbox review; Claude Code blocks these too).
	b.WriteString("(deny appleevent-send)\n(deny job-creation)\n")
	b.WriteString("(deny mach-lookup (global-name \"com.apple.coreservices.launchservicesd\") (global-name \"com.apple.coreservices.quarantine-resolver\"))\n")
	if !p.Network {
		b.WriteString("(deny network*)\n")
		if p.Proxy != "" {
			// The one way out: the proxy, on this machine.
			b.WriteString("(allow network-outbound (remote ip " + sbString(strings.Replace(p.Proxy, "127.0.0.1:", "localhost:", 1)) + "))\n")
		}
	}
	b.WriteString("(deny file-write*)\n(allow file-write*\n")
	for _, w := range append(append([]string(nil), p.Write...), p.Temp) {
		if w != "" {
			b.WriteString("  (subpath " + sbString(w) + ")\n")
		}
	}
	// The devices a shell writes to whatever it runs; no terminal (a
	// command has none of its own, and another tab's is not its to read
	// or to draw on: the sandbox review).
	b.WriteString("  (literal \"/dev/null\") (literal \"/dev/zero\") (literal \"/dev/dtracehelper\")\n")
	b.WriteString("  (regex #\"^/dev/fd/[0-9]+$\"))\n")
	b.WriteString("(deny file-read* file-write* (literal \"/dev/tty\") (regex #\"^/dev/ttys[0-9]+$\"))\n")
	rule := func(what string, paths []string) {
		var some []string
		for _, s := range paths {
			if s != "" {
				some = append(some, "  (subpath "+sbString(s)+")")
			}
		}
		if len(some) > 0 {
			b.WriteString("(" + what + "\n" + strings.Join(some, "\n") + ")\n")
		}
	}
	rule("deny file-write*", p.ReadOnly)
	// gnosh's own files after the read-only paths, which may hold them.
	var writable []string
	for _, e := range p.Expose {
		if e.Write && e.Path != "" {
			writable = append(writable, "  (literal "+sbString(e.Path)+")")
		}
	}
	if len(writable) > 0 {
		b.WriteString("(allow file-write*\n" + strings.Join(writable, "\n") + ")\n")
	}
	rule("deny file-read*", p.DenyRead)
	// Opened again: allowRead, and the folders it writes in and gnosh's
	// own files, which a denied path must not take from it.
	opened := append(append([]string(nil), p.AllowRead...), p.Write...)
	for _, e := range p.Expose {
		opened = append(opened, e.Path)
	}
	if len(p.DenyRead) > 0 {
		rule("allow file-read*", opened)
	}
	// A denied path inside one opened again stays denied: the narrower
	// wins, as in Claude Code.
	var again []string
	for _, d := range p.DenyRead {
		for _, r := range opened {
			if d != "" && r != "" && d != r && under(d, r) {
				again = append(again, d)
				break
			}
		}
	}
	rule("deny file-read*", again)
	return b.String()
}

// sbString is a string in Seatbelt's language: in double quotes, with a
// backslash before a quote or a backslash.
func sbString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
