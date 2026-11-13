# Decisions

Rulings on questions the specs left open. Each follows from a rule Jae has
already given; none is new policy. Jae can overrule any of them. Builders
follow this file where `spec/cc/` or `spec/gnosh/` leaves a choice.

**Which spec wins.** `spec/gnosh/` records what Jae asked his harness to do
and beats `spec/cc/` where the two differ. `spec/cc/` records Claude Code's
public conventions and is followed wherever `spec/gnosh/` is silent. Where
both are silent, the builder chooses, and says so in its report.

## From the Claude Code conventions spec (`spec/cc/INDEX.md`)

1. **gnosh reads its own settings files, not Claude Code's.** The files have
   Claude Code's JSON shape (`permissions`, `hooks`, `env`, ...), so one can
   be copied or linked across, but gnosh does not look in `~/.claude` or
   `.claude` by itself. Another program's hooks would otherwise run inside
   gnosh: Jae's own Claude Code prompt hook adds about 10 KB to every prompt,
   which suits Claude and not a local model with a 64K window. Approvals a
   person saves go to gnosh's own project-local file. (Exact file names and
   how they sit beside `gnoshrc`: `spec/gnosh/harness-config.md`.)
2. **An untrusted folder gets nothing of its own, in every mode.** Until a
   folder is trusted, its project settings, hooks, `env`, skills and rule
   files do not apply, in print mode as much as interactively.
   `--trust-folder` trusts it for one run. This is stricter than Claude Code
   and is what the earlier gnosh's trust gate already did. (Rule: security
   first.)
3. **`Glob` and `Grep` stay in the default tool set.** They are gnosh's own
   code with no outside program behind them, and a local model does better
   with a search tool than with a shell pipeline.
4. **No cap on Stop-hook continuations.** A count never ends the work. After
   a run of forced continuations with no tool call between them (8 by
   default, an option), the model is asked to judge whether there is a real
   reason to stop; if it stops, the person is shown that it did and why.
   Jae, 22 October 2026: "no, if it's not done, it's not done. after 10 not
   dones, say, the harness should have ai think and see if there is a good
   reason to stop, before stopping, but otherwise it should generally keep
   going." `stop_hook_active` is still sent to hooks.
5. **The `init` line says `gnosh_version`**, and lists the sub-agent tool as
   `Agent`.
6. **Subprocesses get `GNOSH=1`** and not `CLAUDECODE=1`. gnosh does not
   pass itself off as another program.
7. **No commit attribution by default.** gnosh adds no trailer lines to
   commits or pull requests unless a setting asks for them.

## From the orchestrating session

8. **Directories.** Config in `$XDG_CONFIG_HOME/gnosh` (default
   `~/.config/gnosh`), data (sessions, mem) in `$XDG_DATA_HOME/gnosh`
   (default `~/.local/share/gnosh`), cache in `$XDG_CACHE_HOME/gnosh`
   (default `~/.cache/gnosh`). A project's own files are in `.gnosh/` at its
   root. Tests point all three variables and `HOME` at temp dirs.
9. **Messages are gnosh's own words.** Error lines, notices, prompts and tool
   descriptions are written fresh, in plain English. What gnosh takes from
   Claude Code is names: flags, tools, fields, values, file names.
