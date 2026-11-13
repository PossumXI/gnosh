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

## From the harness spec (`spec/gnosh/harness-open-questions.md`)

10. **Every recommended reading in that file is adopted**, with the one
    change in item 11. The ones that shape the build most:
    - `gnoshrc` is a list of settings, not a script; a shell construct in it
      is a startup error that names the replacement; `--base-url` may be
      given more than once and the first that answers wins (OQ-1). Jae's
      three old-named files are read in place until he moves them.
    - `-c` and `-C` both mean continue; `--cwd` has no short form (OQ-9).
    - For a local model server there is no limit before the first piece of
      a reply and 900 seconds between pieces; for a remote one, ten minutes
      to start and 60 seconds between pieces; `request-timeout` overrides
      both (OQ-13).
    - Thinking is on by default and `/no_think` sticks (OQ-18).
    - No general list of banned commands; with hosts declared, a command
      that starts with `ssh` or `scp` is refused with a pointer to hosts
      (OQ-5).
    - `/loop` keeps Jae's meaning, "keep working until done" (OQ-17).
    - No counter ends work on empty replies: gnosh keeps asking and tells
      the person at every third one (OQ-7).
    - The JSON settings file has Claude Code's shape only (`permissions`,
      `hooks`, `env`); everything gnosh-specific is in `gnoshrc`; an unknown
      key is an error (OQ-8).
    - Claude Code's tool names everywhere. gnosh's deliverables tool is
      called `Deliver`, never `Artifact` (OQ-10).
    - Hook variables are `GNOSH_*` plus Claude Code's two; `AGENT=gnosh`
      (OQ-21). Of the inherited hook-protocol extras only exit 3 is kept
      (OQ-24).
    - No tool reaches a site without asking; no analytics; no client/server
      mode; no import of old state or old sessions (OQ-6, 22, 23, 25).
    - The colours are gnosh's own purple and hot pink, not the earlier
      program's; Jae picks from rendered options later (OQ-26).
11. **A non-interactive run does not approve its own tool calls** (OQ-2;
    this overrides `harness-cli.md` CLI-31). `gnosh -p` and `gnosh run`
    refuse what would have asked, as Claude Code does, unless the run is
    given `-y` / `--dangerously-skip-permissions`, a permission mode, or
    allow rules. gnosh starts closed (PRIN-1) and follows Claude Code
    (PRIN-3); the two agree here. Jae's unattended runs on the AI box keep
    working without touching each script by setting the mode once in that
    machine's own user settings, at switch-over and with his say.

## From the core spec (`spec/gnosh/open-questions.md`)

12. **Every reading taken in that file is adopted: build it as it landed.**
    Holds are soft for plans and hard for leaves (Q1); a pop spares the
    prompts of the turn it is made in (Q2); the slide is built as specified
    and tested against a server whose window really is 32K (Q3); the root
    lists todos in the order made and `mem.Next()` works there (Q13, Q14);
    stock sub-agents have no gnovm tools (Q15); the goal judge reads the
    full conversation (Q16); the mem reminder is not muted by a goal (Q18);
    a reviewed close is not judged twice (Q19); reminders go in as
    user-side rows (Q20); the fresh guard stays (Q21). The swarm bridge and
    frames on gno.land are not built in this rewrite (Q5, Q6). The questions
    marked "For Jae" there stay his.
13. **How the earlier harness's rows map onto the new transcript.** Where
    the core spec says "row" it means a transcript entry; "flag the rows"
    means appending an entry that names the entries it marks, since nothing
    in a transcript is rewritten; "hidden parts of a prompt" are user-side
    entries of their own, stored in the order sent and joined into one
    message when the request is built, so what is rebuilt from the
    transcript is byte-identical to what went out.
