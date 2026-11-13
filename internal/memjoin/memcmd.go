package memjoin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/gnovm"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// This file is the person's way into mem: /mem inside a session, and
// `gnosh mem` at a shell.
//
// The arguments of /mem are never logged and never put in an error:
// /mem redact carries a secret. What mem answers is said to the person
// and to nobody else.

// redactedMark is what stands in place of a redacted secret, as mem
// writes it.
const redactedMark = "(redacted)"

// minErased is the shortest text that is taken out wherever it stands, as
// mem has it: a shorter one would mangle text that never held it.
const minErased = 6

// memSlash is /mem: the arguments go to mem as the person typed them,
// with the session's id and the names of the project's sessions, and what
// mem answers is shown. When it erased something, gnosh's own copies are
// scrubbed, and the person is told where the text is still shown.
func (j *join) memSlash(ctx context.Context, c *app.SlashContext) error {
	args := strings.TrimSpace(c.Args)
	if args == "help" {
		c.Say("%s", memHelp)
		return nil
	}
	client, err := j.vms.Get()
	if err != nil {
		c.Say("mem cannot be reached: %v", err)
		c.Say("Once the Gno machines can be started, /mem repair builds mem's store and index again from its journal.")
		return errors.New("/mem did nothing: mem cannot be reached")
	}
	text, erased, err := client.MemCommand(ctx, j.project, j.id, args, j.titles(ctx))
	if err != nil {
		// What mem says of a failure may quote what was typed, so it is
		// said to the person and kept out of the error, which is logged.
		c.Say("%v", err)
		if !strings.HasPrefix(args, "repair") {
			c.Say("If mem's own files are damaged, /mem repair builds its store and index again from its journal.")
		}
		return errors.New("/mem failed: what mem said is shown above")
	}
	if text != "" {
		c.Say("%s", text)
	}

	// The person asked for a text to be taken out. gnosh's own copies of
	// it go too, whether or not mem still held it: a session that was
	// open elsewhere the first time is reached by typing the same again.
	sub, rest, _ := strings.Cut(args, " ")
	rest = strings.TrimSpace(rest)
	var e gnovm.MemErasure
	if erased != nil {
		e = *erased
	}
	if sub == "redact" && len(rest) >= minErased {
		if e.Texts == nil {
			e.Texts = map[string]string{}
		}
		if _, ok := e.Texts[rest]; !ok {
			e.Texts[rest] = redactedMark
		}
	}
	done := j.scrub(ctx, e)
	if erased == nil && done.files > 0 {
		c.Say("It was taken out of gnosh's own copies in %d session(s).", done.files)
	}
	if told := done.told(); told != "" {
		c.Say("%s", told)
	}
	return nil
}

// memHelp is what /mem help says.
const memHelp = `/mem                 mem's outline
/mem show <name>     a todo, a fact or a rule in full
/mem find <words>    what in mem matches the words
/mem log [n]         the last n changes (20): when, who, what ran, what it changed
/mem undo [entry]    reverse one change; with no entry, the last is shown first
/mem forget <name>   erase a todo, a fact or a rule, with its history
/mem redact <text>   take a secret's exact text out of mem and its history
/mem repair [vm]     build mem's store and index again from its journal; or
                     repair a broken VM, keeping its entries before the damage
/mem help            this list`

// titles names the project's sessions by their ids, for what mem says of
// who changed what: a session's title, or else the start of its first
// prompt.
func (j *join) titles(ctx context.Context) map[string]string {
	return sessionTitles(ctx, j.s.Sessions, j.s.Dirs.Cwd)
}

func sessionTitles(ctx context.Context, store *transcript.Store, cwd string) map[string]string {
	titles := map[string]string{}
	if store == nil || cwd == "" {
		return titles
	}
	infos, err := store.List(ctx, cwd)
	if err != nil {
		return titles
	}
	for _, info := range infos {
		if name := firstOf(info.Title, clipText(info.FirstPrompt, 60)); name != "" {
			titles[info.ID] = name
		}
	}
	return titles
}

// memCommand is `gnosh mem [show [name…] | find <words> | log [n]]`: the
// project's mem, read from a shell. It needs no session, no model and no
// provider, runs the machines in this process, and only reads: what
// would change mem is refused, with a pointer to /mem. In a project with
// no mem it says so and makes no file.
func (p *process) memCommand(ctx context.Context, c *app.CommandContext) int {
	// Where mem is kept does not depend on the config: the wiring hands
	// over the directories even when the config has mistakes, and mem is
	// read all the same. Reading it needs nothing the config holds.
	if c.Dirs.State == "" {
		fmt.Fprintln(c.Stderr, "gnosh mem: gnosh could not work out where its data is kept, so it cannot tell which mem to read:")
		for _, pr := range c.Problems {
			fmt.Fprintln(c.Stderr, "  "+pr.String())
		}
		return cli.ExitFailure
	}
	dir := DataDir(c.Dirs.State)
	project := c.Dirs.ProjectRoot
	if project == "" {
		project = c.Dirs.Cwd
	}
	// mem's journal is the one file that says a mem is there. Looking for
	// it makes nothing.
	journal := filepath.Join(filepath.Dir(gnovm.MemStatusPath(dir, project)), "journal.jsonl")
	if _, err := os.Stat(journal); err != nil {
		fmt.Fprintln(c.Stdout, "no mem here yet: a session makes one when it first keeps something")
		return cli.ExitOK
	}
	host, err := gnovm.NewHost(gnovm.Options{Dir: dir, Client: gnovm.NewClientID()})
	if err != nil {
		fmt.Fprintf(c.Stderr, "gnosh mem: mem could not be opened: %v\n", err)
		return cli.ExitFailure
	}
	defer host.Close()
	args := strings.Join(c.Invocation.Args, " ")
	text, refused, err := host.MemRead(ctx, project, "gnosh mem", args, sessionTitles(ctx, c.Sessions, c.Dirs.Cwd))
	if err != nil {
		fmt.Fprintf(c.Stderr, "gnosh mem: %v\n", err)
		return cli.ExitFailure
	}
	if text != "" {
		fmt.Fprintln(c.Stdout, strings.TrimRight(text, "\n"))
	}
	if refused {
		// What changes mem is done inside a session. The words after the
		// command are not said back: one of them may be a secret.
		if len(c.Invocation.Args) > 0 {
			switch word := c.Invocation.Args[0]; word {
			case "undo", "forget", "redact", "repair":
				fmt.Fprintf(c.Stdout, "To change mem, type /mem %s … in a session.\n", word)
			}
		}
		return cli.ExitFailure
	}
	return cli.ExitOK
}
