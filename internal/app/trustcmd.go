package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/trust"
)

// stateDir is where gnosh keeps what it writes for the user, worked out
// from the environment alone: the directory the config loader calls
// Paths.StateDir. It is for the commands that must work even when the
// configuration has mistakes: listing the trusted folders and forgetting
// one.
func (p *program) stateDir() string {
	if v := p.env["GNOSH_GLOBAL_DATA"]; filepath.IsAbs(v) {
		return filepath.Clean(v)
	}
	if v := p.env["XDG_DATA_HOME"]; filepath.IsAbs(v) {
		return filepath.Join(filepath.Clean(v), "gnosh")
	}
	return filepath.Join(p.home, ".local", "share", "gnosh")
}

// configDir and cacheDir are the user's config directory and gnosh's cache
// directory, worked out from the environment alone as stateDir is: what
// the config loader calls Paths.ConfigDir and Paths.CacheDir.
func (p *program) configDir() string {
	if v := p.env["GNOSH_GLOBAL_CONFIG"]; filepath.IsAbs(v) {
		return filepath.Clean(v)
	}
	if v := p.env["XDG_CONFIG_HOME"]; filepath.IsAbs(v) {
		return filepath.Join(filepath.Clean(v), "gnosh")
	}
	return filepath.Join(p.home, ".config", "gnosh")
}

func (p *program) cacheDir() string {
	if v := p.env["XDG_CACHE_HOME"]; filepath.IsAbs(v) {
		return filepath.Join(filepath.Clean(v), "gnosh")
	}
	return filepath.Join(p.home, ".cache", "gnosh")
}

// trustCommand is `gnosh trust`: it records a folder as trusted, so that
// its own settings are in force from the next start on, and says what
// those settings do; with --list it shows the record, and with --forget
// it takes a folder out of it.
func (p *program) trustCommand(ctx context.Context, inv cli.Invocation) int {
	record := &trust.Store{Dir: filepath.Join(p.stateDir(), "trusted-folders"), Home: p.home, Now: p.now}

	if inv.List {
		entries, notes := record.List()
		if len(entries) == 0 {
			fmt.Fprintln(p.stdout, "No folder is trusted.")
		}
		for _, e := range entries {
			fmt.Fprintf(p.stdout, "%s  (trusted %s)\n", trust.Escape(e.Path), e.Added.Local().Format("2006-01-02"))
		}
		for _, n := range notes {
			p.warnf("%s.", n)
		}
		return cli.ExitOK
	}

	// The folder: the one named, else the one --cwd names, else the one
	// gnosh was started in. A path that was typed is meant from where it
	// was typed.
	dir := p.o.Cwd
	if inv.Cwd != "" {
		dir = p.absFrom(inv.Cwd)
	}
	if len(inv.Args) > 0 {
		dir = p.absFrom(inv.Args[0])
	}

	if inv.Forget {
		// A folder that is gone can still be forgotten, by the path it
		// was recorded under.
		keys := []string{dir}
		if folder, err := trust.Resolve(dir, p.home); err == nil && folder.Key != dir {
			keys = []string{folder.Key, dir}
		}
		for _, key := range keys {
			gone, err := record.Forget(key)
			if err != nil {
				p.errf("%v", err)
				return cli.ExitFailure
			}
			if gone {
				fmt.Fprintf(p.stdout, "Forgotten: %s\nIts own settings are off again from the next start. Sessions that are open keep what they started with.\n", trust.Escape(key))
				return cli.ExitOK
			}
		}
		fmt.Fprintf(p.stdout, "%s is not in the trust record, so nothing changed. gnosh trust --list shows what is.\n", trust.Escape(keys[0]))
		return cli.ExitOK
	}

	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		p.errf("%s is not a directory that is there, so nothing was recorded.", dir)
		return cli.ExitFailure
	}
	// The folder's settings are read as a folder that is not trusted, to
	// learn what trusting it puts in force. Nothing of the folder's is
	// run for that.
	at := inv
	at.Cwd = dir
	at.TrustFolder = false
	st, code := p.prepare(ctx, at, false)
	if st == nil {
		return code
	}
	if st.cfg == nil {
		st.printProblems()
		p.errf("nothing was recorded: gnosh cannot say what trusting %s would put in force while the configuration cannot be read. Put the mistakes right, then run gnosh trust again.", trust.Escape(st.folder.Root))
		return cli.ExitFailure
	}
	if err := record.Trust(st.folder); err != nil {
		if errors.Is(err, trust.ErrSessionOnly) {
			p.errf("%s is your home directory, or holds it, so it is never recorded as trusted: a record of it would cover everything you have. To use its settings for one run, add --trust-folder; in a session, answer the question it asks.", trust.Escape(st.folder.Key))
			return cli.ExitFailure
		}
		p.errf("%v", err)
		return cli.ExitFailure
	}
	fmt.Fprint(p.stdout, trust.Report(st.folder, st.held))
	for _, n := range st.folder.Notes {
		p.warnf("%s", n)
	}
	return cli.ExitOK
}
