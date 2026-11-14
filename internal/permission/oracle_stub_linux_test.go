//go:build linux

package permission

// stubSource is the one program the oracle builds. Hard-linked under the
// name of every command in the oracle's vocabulary it is a stub: it writes
// down who it is, where it runs, what its first three descriptors are and
// what it was given, and then either exits or becomes the real program of
// that name. Under the name oracle-serve it is the sandbox: a process that
// sits in namespaces of its own, makes everything but the jail read-only,
// and runs one shell at a time for the test, as a user with no rights.
//
// It is kept free of backquotes so that it can sit in this raw string.
const stubSource = `package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

func main() {
	exe, err := os.Readlink("/proc/self/exe")
	if err != nil {
		os.Exit(125)
	}
	name := filepath.Base(exe)
	if name == "oracle-serve" {
		serve()
		return
	}
	stub(exe, name)
}

// realTools are the programs a stub turns into when the mode says so.
var realTools = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "wc": true, "nl": true, "grep": true,
	"which": true, "diff": true, "stat": true, "du": true, "sort": true, "uniq": true, "cut": true,
	"tr": true, "basename": true, "dirname": true, "realpath": true, "readlink": true, "file": true,
	"whoami": true, "uname": true, "seq": true, "find": true, "sed": true, "date": true, "git": true,
	"env": true, "xargs": true, "timeout": true, "nice": true, "nohup": true, "stdbuf": true,
	"setsid": true, "time": true,
}

// editTools join them when the mode is one that lets files be changed.
var editTools = map[string]bool{"rm": true, "rmdir": true, "mv": true, "cp": true, "mkdir": true, "touch": true}

func stub(exe, name string) {
	dir := filepath.Dir(exe)
	mode := filepath.Base(dir)
	root := filepath.Dir(filepath.Dir(dir))
	cwd, _ := os.Readlink("/proc/self/cwd")
	fd := func(n string) string {
		s, _ := os.Readlink("/proc/self/fd/" + n)
		return s
	}
	ppid := os.Getppid()
	// The programs above this one, nearest first, up to the sandbox.
	var chain []string
	for pid, n := ppid, 0; pid > 1 && n < 24; n++ {
		link, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
		if err != nil {
			break
		}
		chain = append(chain, filepath.Base(link))
		stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			break
		}
		f := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
		if len(f) < 2 {
			break
		}
		pid, _ = strconv.Atoi(f[1])
	}
	fd0 := fd("0")
	rec := []string{exe, cwd, fd0, fd("1"), fd("2"), strings.Join(chain, "<")}
	rec = append(rec, os.Args...)
	var b strings.Builder
	for i, f := range rec {
		if i > 0 {
			b.WriteByte('\t')
		}
		b.WriteString(strconv.Quote(f))
	}
	b.WriteByte('\n')
	if f, err := os.OpenFile(root+"/log/stubs", os.O_WRONLY|os.O_APPEND, 0); err == nil {
		f.WriteString(b.String())
		if st, err := f.Stat(); err == nil && st.Size() > 1<<18 {
			// A loop that never ends: stop the shell that runs it.
			syscall.Kill(ppid, syscall.SIGKILL)
			os.Exit(137)
		}
		f.Close()
	}
	if len(mode) != 2 {
		os.Exit(125)
	}
	if strings.IndexByte("reqf", mode[0]) >= 0 {
		if real := realPath(name, mode[0] == 'e' || mode[0] == 'f', fd0); real != "" {
			// In the q and f modes the real tools are told to keep to
			// POSIX, which makes GNU's stop reading flags at the first
			// operand, as the tools of macOS and the BSDs do. A shell is
			// never told so: it would read its lines another way.
			var env []string
			for _, kv := range os.Environ() {
				if !strings.HasPrefix(kv, "POSIXLY_CORRECT=") {
					env = append(env, kv)
				}
			}
			if (mode[0] == 'q' || mode[0] == 'f') && name != "sh" && name != "bash" && name != "dash" {
				env = append(env, "POSIXLY_CORRECT=1")
			}
			syscall.Exec(real, os.Args, env)
		}
	}
	os.Exit(int(mode[1] - '0'))
}

// realPath says which real program a stub should become, or "".
func realPath(name string, edits bool, fd0 string) string {
	switch name {
	case "sh", "bash", "dash":
		// A real shell only for a script that is text in the line: -c, or
		// a pipe or here-document on its standard input. A script file is
		// a program of its own, which no rule reads.
		script, operand := false, false
		for _, a := range os.Args[1:] {
			if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "c") {
				script = true
				break
			}
			if !strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "+") {
				operand = true
				break
			}
		}
		if !script && (operand || !(strings.HasPrefix(fd0, "pipe:") || strings.Contains(fd0, "(deleted)"))) {
			return ""
		}
		if name == "bash" {
			return "/bin/bash"
		}
		return "/bin/dash"
	}
	if !realTools[name] && !(edits && editTools[name]) {
		return ""
	}
	for _, d := range []string{"/usr/bin/", "/bin/"} {
		if st, err := os.Stat(d + name); err == nil && st.Mode().IsRegular() {
			return d + name
		}
	}
	return ""
}

type request struct {
	Argv      []string
	Env       []string
	Dir       string
	UID, GID  uint32
	TimeoutMS int
	Out, Err  string
}

type response struct {
	Exit     int
	Signal   int
	TimedOut bool
	Err      string
}

func serve() {
	root := os.Args[1]
	setupErr := ""
	if err := lockDown(root); err != nil {
		setupErr = err.Error()
	}
	// Process 1 of a namespace gets only the signals it has handlers for,
	// and the Go runtime has handlers for all of them: the ones a shell
	// might send are ignored. SIGCHLD is left alone, or nothing could be
	// waited for.
	signal.Ignore(syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGUSR1,
		syscall.SIGUSR2, syscall.SIGPIPE, syscall.SIGALRM, syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU,
		syscall.SIGVTALRM, syscall.SIGPROF, syscall.SIGWINCH, syscall.SIGIO, syscall.SIGXCPU, syscall.SIGXFSZ)
	// Every shell this starts runs as a user of its own, so this counts
	// only that user's processes.
	syscall.Setrlimit(6, &syscall.Rlimit{Cur: 400, Max: 400})
	in := bufio.NewReaderSize(os.Stdin, 1<<20)
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(response{Err: setupErr})
	if setupErr != "" {
		os.Exit(1)
	}
	for {
		line, err := in.ReadBytes('\n')
		if err != nil {
			os.Exit(0)
		}
		var rq request
		if err := json.Unmarshal(line, &rq); err != nil {
			enc.Encode(response{Err: "bad request: " + err.Error()})
			continue
		}
		enc.Encode(run(rq))
	}
}

// lockDown gives this process a proc of its own (it is process 1 of a new
// process namespace) and makes every mount but the jail read-only, in a
// mount namespace nobody else sees.
func lockDown(root string) error {
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return wrap("making the mounts private", err)
	}
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return wrap("mounting /proc", err)
	}
	if err := syscall.Mount(root, root, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return wrap("binding the jail", err)
	}
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return wrap("reading the mounts", err)
	}
	rootRO := false
	for _, ln := range strings.Split(string(data), "\n") {
		f := strings.Fields(ln)
		if len(f) < 5 {
			continue
		}
		mp := strings.NewReplacer("\\040", " ", "\\011", "\t", "\\012", "\n", "\\134", "\\").Replace(f[4])
		if mp == root || strings.HasPrefix(mp, root+"/") {
			continue
		}
		err := syscall.Mount("", mp, "", syscall.MS_REMOUNT|syscall.MS_BIND|syscall.MS_RDONLY, "")
		if mp == "/" && err == nil {
			rootRO = true
		}
	}
	if !rootRO {
		return wrap("making / read-only", syscall.EPERM)
	}
	return nil
}

func wrap(what string, err error) error {
	return &os.PathError{Op: what, Path: "sandbox", Err: err}
}

func prlimit(pid int, res int, cur, max uint64) {
	lim := syscall.Rlimit{Cur: cur, Max: max}
	syscall.RawSyscall6(syscall.SYS_PRLIMIT64, uintptr(pid), uintptr(res), uintptr(unsafe.Pointer(&lim)), 0, 0, 0)
}

func run(rq request) response {
	if len(rq.Argv) == 0 {
		return response{Err: "no command"}
	}
	out, err := os.OpenFile(rq.Out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_APPEND, 0o666)
	if err != nil {
		return response{Err: err.Error()}
	}
	defer out.Close()
	errf, err := os.OpenFile(rq.Err, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return response{Err: err.Error()}
	}
	defer errf.Close()
	cmd := exec.Command(rq.Argv[0], rq.Argv[1:]...)
	cmd.Env = rq.Env
	cmd.Dir = rq.Dir
	cmd.Stdout, cmd.Stderr = out, errf
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:     true,
		Credential: &syscall.Credential{Uid: rq.UID, Gid: rq.GID, Groups: []uint32{rq.GID}},
	}
	if err := cmd.Start(); err != nil {
		return response{Err: err.Error()}
	}
	pid := cmd.Process.Pid
	prlimit(pid, syscall.RLIMIT_CPU, 2, 3)
	prlimit(pid, syscall.RLIMIT_FSIZE, 8<<20, 8<<20)
	prlimit(pid, syscall.RLIMIT_AS, 1<<30, 1<<30)
	var timedOut atomic.Bool
	timer := time.AfterFunc(time.Duration(rq.TimeoutMS)*time.Millisecond, func() {
		timedOut.Store(true)
		syscall.Kill(-1, syscall.SIGKILL)
	})
	werr := cmd.Wait()
	timer.Stop()
	// Whatever the shell left running goes with it, and is reaped here:
	// this process is the init of its namespace.
	for {
		syscall.Kill(-1, syscall.SIGKILL)
		var ws syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &ws, syscall.WNOHANG, nil)
		if err != nil {
			break
		}
		if pid == 0 {
			time.Sleep(200 * time.Microsecond)
		}
	}
	resp := response{TimedOut: timedOut.Load()}
	if ee, ok := werr.(*exec.ExitError); ok {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			resp.Signal = int(ws.Signal())
			resp.Exit = 128 + resp.Signal
		} else {
			resp.Exit = ee.ExitCode()
		}
	} else if werr != nil {
		resp.Err = werr.Error()
	}
	return resp
}
`
