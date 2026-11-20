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
	if !p.Network {
		b.WriteString("(deny network*)\n")
	}
	b.WriteString("(deny file-write*)\n(allow file-write*\n")
	for _, w := range append(append([]string(nil), p.Write...), p.Temp) {
		if w != "" {
			b.WriteString("  (subpath " + sbString(w) + ")\n")
		}
	}
	for _, e := range p.Expose {
		if e.Write && e.Path != "" {
			b.WriteString("  (literal " + sbString(e.Path) + ")\n")
		}
	}
	// The devices a shell writes to whatever it runs.
	b.WriteString("  (literal \"/dev/null\") (literal \"/dev/zero\") (literal \"/dev/tty\") (literal \"/dev/dtracehelper\")\n")
	b.WriteString("  (regex #\"^/dev/fd/[0-9]+$\") (regex #\"^/dev/ttys[0-9]+$\"))\n")
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
	rule("deny file-read*", p.DenyRead)
	rule("allow file-read*", p.AllowRead)
	// A denied path inside one opened again stays denied: the narrower
	// wins, as in Claude Code.
	var again []string
	for _, d := range p.DenyRead {
		for _, r := range p.AllowRead {
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
