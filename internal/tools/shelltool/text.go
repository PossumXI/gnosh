package shelltool

import (
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

// seconds writes a duration the way results show it: "120 s", "3.2 s",
// "0.05 s".
func seconds(d time.Duration) string {
	s := d.Seconds()
	digits := 3
	switch {
	case s >= 100:
		digits = 0
	case s >= 10:
		digits = 1
	case s >= 1:
		digits = 2
	}
	text := strconv.FormatFloat(s, 'f', digits, 64)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	return text + " s"
}

// byteSize writes a number of bytes the way results show it: "812 bytes",
// "64 MiB", "1.5 GiB".
func byteSize(n int64) string {
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	if n < 1024 {
		return strconv.FormatInt(n, 10) + " bytes"
	}
	v := float64(n)
	unit := ""
	for _, u := range units {
		v /= 1024
		unit = u
		if v < 1024 {
			break
		}
	}
	text := strconv.FormatFloat(v, 'f', 1, 64)
	return strings.TrimSuffix(text, ".0") + " " + unit
}

// plural writes a count with its noun: "1 line", "2 lines".
func plural(n int64, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.FormatInt(n, 10) + " " + noun + "s"
}

// signalName names a signal the way people write it, such as "SIGKILL".
func signalName(sig syscall.Signal) string {
	switch sig {
	case syscall.SIGHUP:
		return "SIGHUP"
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGQUIT:
		return "SIGQUIT"
	case syscall.SIGILL:
		return "SIGILL"
	case syscall.SIGTRAP:
		return "SIGTRAP"
	case syscall.SIGABRT:
		return "SIGABRT"
	case syscall.SIGBUS:
		return "SIGBUS"
	case syscall.SIGFPE:
		return "SIGFPE"
	case syscall.SIGKILL:
		return "SIGKILL"
	case syscall.SIGUSR1:
		return "SIGUSR1"
	case syscall.SIGSEGV:
		return "SIGSEGV"
	case syscall.SIGUSR2:
		return "SIGUSR2"
	case syscall.SIGPIPE:
		return "SIGPIPE"
	case syscall.SIGALRM:
		return "SIGALRM"
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGXCPU:
		return "SIGXCPU"
	case syscall.SIGXFSZ:
		return "SIGXFSZ"
	}
	return "signal " + strconv.Itoa(int(sig))
}

// firstLine returns the first line of s that is not blank, without the
// space around it, cut to limit characters.
func firstLine(s string, limit int) string {
	for line := range strings.Lines(s) {
		if line = strings.TrimSpace(line); line != "" {
			return shorten(line, limit)
		}
	}
	return ""
}

// oneLine puts s on one line: every run of white space becomes one space.
func oneLine(s string, limit int) string {
	return shorten(strings.Join(strings.Fields(s), " "), limit)
}

// shorten cuts s to limit characters, ending it with "…" when it was cut.
func shorten(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return firstRunes(s, limit-1) + "…"
}
