package config

import (
	"fmt"
	"regexp"
	"strings"
)

// localHostName is the name this machine goes by wherever a machine is
// named: in the first line of a command's result, and as the host of a call
// that means here. No declared host may have it, in any letter case.
const localHostName = "local"

// What the destination of a host may look like. hostTarget is the letters
// it may hold: those of a user name, an address or an alias, and nothing a
// shell or ssh would read as more than a name. hostPort is a destination
// with a port number written after it, which ssh would take for part of
// the machine's name.
var (
	hostTarget = regexp.MustCompile(`^[A-Za-z0-9_.\[\]:%@-]+$`)
	hostPort   = regexp.MustCompile(`^[^:\[\]]+:[0-9]+$`)
)

// checkHosts looks over the declared hosts as they stand once every file
// has been read, and returns what is wrong with them. It is the one check
// of a host declaration, and it reads the declarations as plain values:
// whichever file filled them in, in whichever format, the same mistakes
// are found, and each names the host it is about and the place that
// declared it (the host's Origin).
//
// A declaration is right when its name is a usable one that is not this
// machine's and is not declared twice, its destination is user@address or
// one word (an address, or a Host alias of the person's ssh configuration),
// and what is written about it is one line of plain words. Whether an
// alias exists is ssh's to say; gnosh does not read the ssh configuration.
func checkHosts(hosts []Host) []Problem {
	var out []Problem
	bad := func(h Host, format string, args ...any) {
		p := Problem{Severity: Error, File: h.Origin.File, Line: h.Origin.Line, Message: fmt.Sprintf(format, args...)}
		if h.Origin.File == "" {
			p.Where = h.Origin.Name
		}
		out = append(out, p)
	}
	const form = "A destination is user@address, or a Host alias from your ssh configuration"
	first := map[string]Host{}
	for _, h := range hosts {
		user, address, at := strings.Cut(h.SSH, "@")
		switch {
		case h.Name == "":
			bad(h, "a host is declared without a name. It needs one: the model names the machine by it.")
		case !idPattern.MatchString(h.Name):
			bad(h, "%s is not a usable name for a host: use letters, digits, dots, dashes and underscores, starting with a letter or a digit.", showName(h.Name))
		case strings.EqualFold(h.Name, localHostName):
			bad(h, "a host cannot be named %s: %s is the name of this machine, in every result of a command and as the host of a call that means here. Give the host another name.", h.Name, localHostName)
		case h.SSH == "":
			bad(h, "the host %s has no ssh destination, so there is nowhere to run a command for it. %s.", h.Name, form)
		case strings.HasPrefix(h.SSH, "-"):
			bad(h, "the ssh destination of the host %s starts with a dash, so ssh would read it as an option. %s.", h.Name, form)
		case !hostTarget.MatchString(h.SSH):
			bad(h, "the ssh destination of the host %s holds blanks, or characters a shell would act on. %s; keys, ports and jump hosts belong in ~/.ssh/config.", h.Name, form)
		case at && (user == "" || address == "" || strings.Contains(address, "@")):
			bad(h, "the ssh destination of the host %s, %s, is not a user and an address with one @ between them. %s.", h.Name, show(h.SSH), form)
		case hostPort.MatchString(h.SSH):
			bad(h, "the ssh destination of the host %s, %s, ends in a port number, which ssh would take for part of the machine's name. %s; a port belongs in ~/.ssh/config, under a Host alias (Port 2222).", h.Name, show(h.SSH), form)
		case !printable(h.Description):
			bad(h, "the description of the host %s holds a line break or another control character. A description is one line of plain words: the model is shown it as written.", h.Name)
		default:
			if was, twice := first[h.Name]; twice {
				bad(h, "the host %s is declared twice: here as %s, and at %s as %s. A name stands for one machine, so that a command for it can only go to that one: take one of the two declarations out.",
					h.Name, show(h.SSH), was.Origin, show(was.SSH))
				continue
			}
			first[h.Name] = h
		}
	}
	return out
}
