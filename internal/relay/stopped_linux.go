//go:build linux

package relay

import (
	"bytes"
	"os"
	"strconv"
)

// stopped reports whether the process pid is stopped.
func stopped(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// "pid (name) state ...": the name may hold anything, a ) too.
	i := bytes.LastIndexByte(b, ')')
	return i >= 0 && i+2 < len(b) && b[i+2] == 'T'
}
