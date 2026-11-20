package app

import (
	"slices"

	"github.com/gnosh-ai/gnosh/internal/envscrub"
)

// With CLAUDE_CODE_SUBPROCESS_ENV_SCRUB set to 1, the programs gnosh starts
// that are not gnosh itself (the shell's commands, a skill's !`commands`,
// hooks, stdio MCP servers, LSP servers, the status line and the file
// suggestion command, gnosh's own git) start without the credentials
// gnosh was started with; gnosh keeps them for its own requests, and a
// gnosh it starts again (/restart, a work tree, the background) has them
// (envscrub; spec/DECISIONS.md 384).

// scrubbing reports whether the scrub is on.
func (p *program) scrubbing() bool { return envscrub.On(p.o.Environ) }

// childEnv is env as a program gnosh starts is given it: without its
// credentials when the scrub is on.
func (p *program) childEnv(env []string) []string {
	if p.scrubbing() {
		return envscrub.Env(env)
	}
	return env
}

// childEnviron is the environment gnosh was started with, as a program it
// starts is given it.
func (p *program) childEnviron() []string { return p.childEnv(slices.Clone(p.o.Environ)) }
