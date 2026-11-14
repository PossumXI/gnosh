# harness-ledger.md — every other behaviour change, one line each

*Since 14 November 2026 gnosh is configured by one JSON file, `gnosh.json`, and no longer by `gnoshrc` (`spec/DECISIONS.md`, items 17 and 23 to 28). Where this file speaks of `gnoshrc`, of its directives or of its dashed option names, it records what the build was first given.*

What this file is: the long tail. Every change to the earlier gnosh's harness
behaviour that the sources record, one line each, so that nothing Jae had is
dropped by accident. Things that have a full write-up elsewhere are here too,
as a line that points to it.

How to read it:

- **Behaviour** says what the user or the model sees, in plain words.
- **Source** is a date (2026, month-day) and the short ID of the change in
  the earlier gnosh's history, for the spec side to trace. The build side
  does not open that history (PROVENANCE.md); the words here are the spec.
- **Pri**: P0, P1, P2 as in `harness-00-principles.md`. "n/a" = belongs to
  something the new gnosh does not have; kept so the decision is on record.
- **See** points to the requirement that carries the detail.
- "as CC" = Claude Code does this, and `spec/cc/` has its rules.

Not in this ledger: the GnoVM machines, `mem`, frames, the close check, the
goal judge, peers and the swarm (the other spec set); documentation, brand
art for the website, licences and the repository's own housekeeping.

Counts: 872 changes were made to the fork between 2026-10-18 and 2026-11-11.
About 470 of them touched harness behaviour; they are grouped into the rows
below.

---

## A. Command line and print mode

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| A1 | `-p` / `--print` answers one prompt and exits; it is the same thing as `gnosh run`, so `run`'s flags work after it | 10-22 2bc558eb | P0 | CLI-2 |
| A2 | `-r` / `--resume` and `--dangerously-skip-permissions` work beside `-s` and `-y`, on the interactive command and on `run` | 10-22 903b4375, ba86c907 | P0 | CLI-4, CLI-6 |
| A3 | On `run`, the skip-permissions flags are accepted, change nothing, and the help says so | 10-22 6df80961 | P1 | CLI-6 |
| A4 | A bare `-r` shows the sessions to pick from; a session named on the command line opens before any first-run tip | 10-23 1d0bd223, 41706747, 195c5c99 | P1 | CLI-4 |
| A5 | `-r` and `/resume` take a session's **name**; in a run, several matches are an error listing their IDs | 10-24 12efe5cf, 836c0476 | P1 | CLI-4 |
| A6 | `--fork-session` goes on in a copy of the session resumed | 10-25 7acd0dfa | P1 | CLI-4 |
| A7 | `-n` / `--name` names the session from its start, on resume, or for the fork copy | 10-25 9394d1dd | P1 | CLI-4 |
| A8 | `-m` / `--model` on the interactive command, in memory only; a resumed session does not switch back | 10-25 10daac6c | P0 | CLI-5 |
| A9 | `gnosh "prompt"` opens the session with the prompt sent | 10-25 e521e8c5 | P0 | CLI-1 |
| A10 | `--output-format json`: one result object at the end, also on failure | 10-22 e1116608 | P1 | CLI-40 |
| A11 | `--output-format stream-json`: a line per message and tool result, then the result | 10-23 741ba52d | P1 | CLI-40 |
| A12 | The stream's first line says what the run really uses (model after a resume, mode, tools, MCP servers) | 10-23 bb2a2318 | P1 | CLI-40 |
| A13 | The result has `num_turns`, counted from the run's own replies only | 10-23 35f5f81c | P1 | CLI-40 |
| A14 | Text output separates its messages; the JSON result is the answer alone | 10-23 2ebb9b00 | P1 | CLI-40 |
| A15 | A row cut from the context by a pop is not printed again in a later run's stream | 11-04 (frames QA fix C4) | P1 | CLI-40 |
| A16 | `--verbose`: one stderr line per tool call; failures marked; a shell call's exit code; a sub-agent's calls shown under their parent; always one line each; no progress spinner | 10-22 d426c200, 9710f78d, dcff7c6f, c6fb6e25, 928bd776, 697a58ce, 682cb385 | P1 | CLI-41 |
| A17 | A run prints hook warnings (and an AI-chosen stop with its reason) on stderr | 10-22 eb8069f0 | P0 | CLI-42 |
| A18 | A failed turn is reported as a failed turn, not as a failure to start | 10-22 d862df2b | P1 | CLI-40 |
| A19 | A run does not wait for ever on a piped stdin that stays silent | 10-22 c79b350f | P0 | CLI-3 |
| A20 | `--allowedTools`, `--disallowedTools` (both spellings), with CC's tool names | 10-22 5782d0e6 | P1 | CLI-6 |
| A21 | `--tools` gives the model only the tools named; `""` gives none | 10-23 734108ed | P1 | CLI-6 |
| A22 | `Bash(pattern)` rules are accepted in the tool flags | 10-24 60bf6f08, 976988d4 | P1 | TOOL-31, TOOL-32 |
| A23 | `--system-prompt`, `--append-system-prompt`, and their `-file` forms; the environment, skills and context files are kept | 10-23 30fae57b | P1 | CLI-7 |
| A24 | `--permission-mode` with `default`, `plan`, `acceptEdits`, `bypassPermissions`; then `dontAsk` | 10-23 572f1a5b; 10-24 631ebf58 | P1 | CLI-6, CLI-31 |
| A25 | `--add-dir`, and `/add-dir` while a session runs; a sub-agent's prompt names the added directories | 10-23 d228ae2f; 10-25 556abbbc, a295f087 | P1 | CLI-6 |
| A26 | `--add-dir` approves only reading and listing files there | 10-23 7e2d1dcf | P1 | SAFE-14 |
| A27 | `--mcp-config`, `--strict-mcp-config` | 10-24 5b70e37d | P2 | CLI-8 |
| A28 | `--safe-mode` starts with hooks, MCP servers, context files, skills and custom commands off, and says so | 10-25 764dd471 | P1 | CLI-20 |
| A29 | `gnosh trust`, `--trust-folder` | 11-01 7fa38a65 | P1 | SAFE-7 |
| A30 | `gnosh doctor`: config, providers, models, hooks, tools; exit 1 when a turn cannot run | 10-23 dddf8961 | P1 | CFG-60 |
| A31 | `doctor` checks the scripts and programs hooks run, and calls a hook that cannot run an error where it would block | 10-23 1dcb6ca7, 84665f3f, cb5ba6e6 | P1 | CFG-60 |
| A32 | `gnosh models` says which configured providers it could not use, and why | 10-23 8a9a8858 | P1 | CLI-10 |
| A33 | `gnosh prompt show`, `path`, `export`: the instructions as a file the user can replace | 10-18 44fbc863 | P1 | CLI-12 |
| A34 | `gnosh prompt tools`: what the model is told about each tool, with sizes | 10-19 39de8edb | P1 | CLI-12 |
| A35 | Tool descriptions as files the user can edit, and a list of tools to turn off | 10-19 85f5b479 | P1 | CLI-12 |
| A36 | An exported description changes nothing until it is edited | 10-22 c00821d2 | P1 | UI-41 |
| A37 | `gnosh prompt context`: what fills the context window, as CC's `/context` | 10-24 51db1455 | P1 | CLI-12 |
| A38 | `gnosh session web`: a local page to read and edit a session | 10-19 1796dee1, 12d85090 | P2 | UI-40 |
| A39 | That page refuses other sites (name rebinding, cross-site posts and frames, writes with no origin) | 11-04 aa35fba0, 24cc09c2 | P2 | SAFE-23 |
| A40 | That page's port is not the deliverables server's; a taken port falls back to a free one unless the user named it | 11-04 6a32500c; 11-05 eddb26f8 | P2 | UI-40 |
| A41 | `gnosh mem` and `gnosh session` start no provider | 10-30 bea2a12d | P1 | CFG-70 |
| A42 | `gnosh ssh`: copies made on the far side of ssh reach this computer's clipboard | 11-09 a4ba355c; 11-10 5b9ce3e3 | P2 | UI-12 |
| A43 | Stopped by SIGTERM: hooks, tools and jobs stop with it; exit 143 | 10-25 eb828a0c, 7f0a903f, 58737b14 | P0 | CLI-43 |
| A44 | Stopped by SIGINT: exit 130 with nothing printed | 10-25 199ebcdd | P0 | CLI-43 |
| A45 | An error that begins with something the user typed prints it as typed | 10-22 3293d50b | P1 | CLI-33 |
| A46 | `run --goal`: a goal found met is printed, and is not a failure | 10-30 8c1be91a | P1 | CLI-9 |
| A47 | The version printed is the running program's own build | 10-30 | P0 | CLI-10 |
| A48 | Custom slash commands are read from the user's commands folder | 10-22 4fea3f8a | P2 | `spec/cc` |
| A49 | An ignore file of gnosh's own (same form as `.gitignore`) beside `.gitignore`, for searches and listings | 10-22 bc9a0e4d | P2 | |

## B. Sessions and data

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| B1 | A reply left unfinished by a crash or a kill is marked interrupted when the session is opened | 10-20 0b003aa9 | P0 | LOOP-6 |
| B2 | A project uses the closest data folder on the way up to its root | 10-22 c87ecbac | P1 | CFG-2 |
| B3 | A session is named after its prompt at once and titled by the model after its first turn | 10-30 c45d57f3 | P1 | LOOP-9 |
| B4 | Titles work on thinking models (the title request does not think) | 10-22 306870d0 | P0 | LOOP-42 |
| B5 | A title that cannot be made is a warning, not an error | 10-22 41fcc87e | P1 | LOOP-9 |
| B6 | Sub-agent sessions get no generated title | 10-22 d5b3f09d | P0 | LM-14 |
| B7 | A rename survives a summary and a late automatic title | 10-24 0a501b21 | P1 | LOOP-9 |
| B8 | Whatever updates a session's record writes only its own fields, so two updates at once never undo each other (usage, title, summary) | 10-25 150d0e11, da1b19c0 | P1 | LOOP-9 |
| B9 | A data folder removed while gnosh runs is put back, and the user is told; it is locked while being restored; a second removal is restored too; one made just before exit is restored | 11-03 516bf609; 11-04 (ten follow-ups) | P1 | CFG-3 |
| B10 | A new or restored data folder has its own `.gitignore` | 11-03 3a624399 | P1 | CFG-3 |
| B11 | The log file is reopened if it was removed, and appends to the file now at its path | 11-04 be5f052c | P2 | CFG-3 |
| B12 | `/branch`: copy the conversation into a new session and go on there | 10-25 a7837a92 | P1 | UI-30 |
| B13 | `/rewind`: restore the files the edit tools changed, the conversation, or both, to before a prompt; the whole conversation is kept as a session of its own | 10-25 3c5a6c3b | P1 | UI-30 |
| B14 | A rewind forgets the reads and the on-demand context files of the part it cut | 10-25 4724cd78 | P1 | CFG-23 |
| B15 | `/export`: save the conversation as text or copy it | 10-24 542d84e0 | P2 | UI-30 |
| B16 | Leaving a session, coming back and leaving again ends it twice; switching away and then quitting ends it once | 10-23 7b08e6a1 | P2 | HOOK-18 |

## C. Interface

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| C1 | `/think` and `/no_think` switch a model's thinking for every later request | 10-18 e19c1846 | P0 | LOOP-40 |
| C2 | A typed command name selects exactly that command, not the nearest fuzzy match | 10-18 70273327 | P1 | UI-30 |
| C3 | A thinking **level** per model, set in `/model` with the arrow keys | 10-25 653e5ec2 | P1 | LOOP-41 |
| C4 | The status shows how hard the current model thinks | 10-25 eed611d8 | P0 | UI-20 |
| C5 | Prompts queued while a turn runs can be recalled with Up | 10-21 bbef3bbb | P0 | LOOP-7 |
| C6 | Up in an empty input box edits queued prompts in place; none is ever lost; attachments come back; Esc still stops gnosh's own follow-ups | 10-22 0e786aec, 9d735911, 68b104ba, 09f9b898 | P0 | LOOP-7, LOOP-5 |
| C7 | History never offers a copy of a queued prompt, nor a prompt gnosh sent itself | 10-22 c6c70ccf, 064de1d2 | P1 | LOOP-7 |
| C8 | Enter with only an attachment sends it | 10-22 c04a877c | P1 | UI-31 |
| C9 | A prompt refused before it was sent comes back into the input box; one refused during a turn leaves that turn running; errors from the agent are shown | 10-23 a3b7f930, d4b7ef40 | P1 | HOOK-15 |
| C10 | CC's slash names find gnosh's commands | 10-23 7a1bf975 | P1 | UI-30 |
| C11 | A slash command runs wherever it is typed, takes what is typed after its name, and keeps what was attached | 10-25 d1fced60, 7eb15e97, d6fb02f1 | P1 | UI-30 |
| C12 | `/rename` names the session; with no name it makes one from the conversation | 10-24 786f63dc, 12e2fa27 | P1 | UI-30 |
| C13 | `/compact <instructions>` | 10-24 f013a37e | P1 | LOOP-63 |
| C14 | `/autocompact` | 10-25 0b267db3 | P1 | LOOP-65 |
| C15 | `/context` | 10-24 2ec0b929 | P1 | UI-20 |
| C16 | `/hooks`, `/permissions` (read-only), `/status` (names both models), `/doctor`, `/diff`, `/tasks` | 10-24 5c2c7504, f7415a2b, 53d45ef8, 7fc52188, f80ae956, 8cc65059; 10-25 8e2220eb | P1 | UI-20 |
| C17 | `/copy` copies a response, or one code block of it | 10-24 0eda16e3; 10-25 a11e08c4 | P1 | UI-11 |
| C18 | Esc twice on an empty prompt opens the rewind list | 10-25 0c50666e | P1 | UI-31 |
| C19 | Esc twice clears a prompt with text and keeps it for Up | 10-25 9ab9ac4a | P1 | UI-31 |
| C20 | A project never initialized gets a tip to run `/init`, shown 30 s, not a question that takes the first keys typed | 10-25 65c00845, 7e578c12, bf95b16e | P2 | UI-30 |
| C21 | A background job's end shows as one quiet line | 10-25 37438ace | P1 | TOOL-16 |
| C22 | A tool call's input is shown as the model writes it; a Gno call is headed by its code | 10-30 850a180e, 3ce4091f | P0 | LOOP-4 |
| C23 | Deliverables are listed apart from changed files, latest version of each | 10-24 757ecc76 | P1 | UI-33 |
| C24 | A deliverable's label is one colour, its path or URL is shown whole, and it can be copied or opened | 10-30 b0f9e57a | P1 | UI-33 |
| C25 | A payment's receipt and errors wrap instead of being cut | 11-02 3a656f25 | P2 | UI-52 |
| C26 | Counts of added and removed lines are right for lines that begin with `--` or `++` | 11-01 e85a504d | P2 | |
| C27 | While `/loop` is on the interface says so, through the final turn | 10-21 bdd0eded, e686ce59 | P1 | LOOP-22 |
| C28 | The terminal's title names gnosh | 10-22 006b2238 | P2 | |
| C29 | The look: purple and hot pink kept, with green and gold; fades run green, purple, hot pink, gold | 10-24 97957c38; 10-25 f144ea54, 20ad7d11 | P2 | UI-50 |
| C30 | No stars in the wordmark, no stretched letter, the site and version under the word | 10-25 5ee5d25c, 17685a08 | P2 | UI-51, UI-53 |
| C31 | "Gno" is one green where the terminal lacks full colour, and the full blend where it has it | 10-30 f2dc3a5d, 36c0214e | P2 | UI-53 |
| C32 | The header's wordmark blends evenly, in bold, and the lines beside it fade | 11-09 cf23cd82 | P2 | UI-53 |
| C33 | Text drawn on the brand colour is readable everywhere | 10-24 e6a23599 | P2 | |

## D. Hooks

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| D1 | An unknown hook event, and a hook answer gnosh would otherwise ignore, are refused | 10-22 ca02493f | P1 | HOOK-2 |
| D2 | `Stop` hooks that keep the agent working until the work is done; a stop marker for checkpoints | 10-22 613fb3b1, a8c199a6 | P1 | HOOK-21 |
| D3 | `PostToolUse`; `PostToolUseFailure` | 10-22 5834be40, 80627118 | P1 | HOOK-12 |
| D4 | CC's `"continue": false` ends the turn; top-level fields count beside the nested ones | 10-22 fb184c45 | P1 | HOOK-32 |
| D5 | The payload names its event as `hook_event_name`; tool hooks get the call's ID as `tool_use_id` | 10-22 40dd4aca, 66ff2208 | P1 | HOOK-31 |
| D6 | A bad field in a hook's answer is named, and a deny given beside it holds | 10-22 99884328 | P1 | HOOK-32 |
| D7 | A changed input that is not an object makes the hook a failed one, and voids the approval given with it | 10-22 302f156f, a82c5e8c | P1 | HOOK-11 |
| D8 | `UserPromptSubmit`; then: Esc works during the hook, pasted text is in the payload, a hook added mid-session behaves | 10-22 d46218cb; 10-23 f47e3765 | P1 | HOOK-15 |
| D9 | `PreCompact` (with `custom_instructions`), `PostCompact` | 10-23 01233b18, 5203f68e; 10-24 69441074 | P2 | HOOK-17 |
| D10 | `SessionStart`, before the prompt's own hooks; four bugs fixed (run once per start; context kept when the first prompt is blocked; delivered on gnosh's own prompts; one record of started sessions) | 10-23 ad1a746c, baeaa40c, 815a6172, a19e506e, 799eaae3, b0236833 | P1 | HOOK-16 |
| D11 | `SessionStart` can set the environment of the session's commands (`CLAUDE_ENV_FILE`) | 10-24 79797d87 | P2 | HOOK-23 |
| D12 | `SubagentStop`, with CC's input; `SubagentStart` | 10-23 66181241, 959ae396; 10-24 7cf511ba | P2 | HOOK-1 |
| D13 | Tool hooks run on sub-agents' tool calls | 10-23 0a7d59a8 | P1 | HOOK-13 |
| D14 | A sub-agent's halt ends the parent's turn | 10-23 e7f08fb9 | P1 | HOOK-13 |
| D15 | Matchers read as CC's, with CC's tool names; CC's grouped hook block loads | 10-23 45ab508e, 2be0993f | P1 | HOOK-4, HOOK-5 |
| D16 | `WebFetch` and `WebSearch` matchers reach gnosh's fetch tools | 10-23 b1de3104 | P1 | HOOK-14 |
| D17 | A guard copied from CC also covers the gnosh tools CC has no match for, for blocking only | 10-23 732264ab | P1 | HOOK-14 |
| D18 | `CLAUDE_PROJECT_DIR` is set for hooks | 10-23 8814d5b2 | P1 | HOOK-30 |
| D19 | Hook timeouts default to CC's: 600 s, and 30 s for `UserPromptSubmit` | 10-23 abe608ac | P1 | HOOK-7 |
| D20 | Hooks are told the permission mode | 10-23 92a83553 | P1 | HOOK-31 |
| D21 | **A hook that could not check blocks** | 10-23 ef806204 | P1 | HOOK-10 |
| D22 | A non-zero exit that says why blocks; a silent one is no opinion; `systemMessage` warns | 10-23 f6a8b31c | P1 | HOOK-9 |
| D23 | `SessionEnd`; it runs on SIGINT too; a session left by a signal ends with reason `other` | 10-23 de918c43; 10-25 35111679, 159b1a4e | P2 | HOOK-18 |
| D24 | A hook's `ask` puts the call to the user even when it would run unasked; once per call; refused where nobody can answer | 10-24 3907abde, aa152cd9 | P1 | HOOK-11 |
| D25 | `StopFailure` | 10-24 ad2a08a2 | P2 | HOOK-31 |
| D26 | `InstructionsLoaded` | 10-24 bb164daa, 23b7ec82 | P2 | HOOK-1 |
| D27 | `PermissionRequest` | 10-24 eade13de | P2 | HOOK-19 |
| D28 | `Notification`; its timers exist only when such a hook is configured; the idle notice names the session whose turn finished | 10-25 842f19f1, a35ad539 | P2 | HOOK-20 |
| D29 | A file a formatter hook changed after an edit counts as freshly read, and the model is told | 10-22 5834be40 | P1 | HOOK-12 |
| D30 | `GNOSH_MEM_STATUS` names a file with what `mem` holds, for hooks | 10-30 cd233381 | P2 | HOOK-30 |
| D31 | The config command and the settings loader accept and refuse the same hooks | 10-23 3f357086 | P1 | CFG-41 |

## E. Permissions, trust and safety

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| E1 | A sub-agent inherits its parent's standing to act without asking | 10-22 9c12ad2f | P1 | LOOP-81 |
| E2 | `acceptEdits` lets edits inside the project run, nothing else | 10-24 bed5b84d | P1 | SAFE-12 |
| E3 | A change to a protected file always asks; names match in any case; a grant covers a file where it really is | 10-24 c1b194f4, 62368c78 | P1 | TOOL-33 |
| E4 | No "allow for this session" for a request that is asked every time | 10-24 3604a22a | P1 | TOOL-34 |
| E5 | CC's `Bash(pattern)` deny rules | 10-24 60bf6f08 | P1 | TOOL-32 |
| E6 | A shell command runs unasked only if every part of it only reads | 10-24 c52cbac2 | P1 | TOOL-30 |
| E7 | CC's `Bash(pattern)` allow rules, with gnosh asking in more cases | 10-24 976988d4 | P1 | TOOL-31 |
| E8 | A `cd` into the project is read-only | 10-24 dae556a3 | P1 | TOOL-30 |
| E9 | Searches ask before looking outside the working directory | 10-25 f68b9578 | P1 | TOOL-14 |
| E10 | A folder's own settings wait until it is trusted | 11-01 7fa38a65 | P1 | SAFE-1 |
| E11 | The gate holds against a planted `.git`, and shows exactly what trust turns on | 11-01 74a93251 | P1 | SAFE-8 |
| E12 | The trust question says what could go wrong | 11-01 25ad1412 | P1 | SAFE-4 |
| E13 | A link out of the project is read only after asking, and the question says where it leads | 11-01 9f30681b | P1 | SAFE-10 |
| E14 | Read-only commands run unasked only when they read the project | 11-01 bd0dc909 | P1 | SAFE-11 |
| E15 | A repository's own git settings run nothing until trust | 11-01 cd0f8ba6 | P1 | SAFE-15 |
| E16 | A project's stored state starts no MCP server until trust | 11-01 4c84f868 | P1 | SAFE-8 |
| E17 | Read-only git runs no program under a shortened option; a date command reads the project only | 11-01 c2e6fa8f | P1 | SAFE-8 |
| E18 | A project settings file owned by someone else stops startup with whose it is | 10-22 1c3b4d6a | P1 | CFG-40 |
| E19 | The web-reading sub-agent's file tools open only the pages it fetched | 10-23 ae4d60cd | P1 | SAFE-21 |
| E20 | Its own fetches reach public sites only; an approved URL cannot redirect into the user's network | 10-23 981d64ab, 4d16069f | P1 | SAFE-21 |
| E21 | A model's control tokens in untrusted text are defused | 10-20 c7574c57 | P0 | SAFE-20 |
| E22 | Nothing goes to the vendor's servers unless asked: analytics, catalog, update check | 10-18 f48fdbb2; 10-22 b75b14c9 | P0 | SAFE-30 |
| E23 | gnosh's own endpoints for analytics and the update check, never the vendor's | 10-23 92846ec1; 10-24 7def60a0 | P2 | SAFE-30 |
| E24 | The analytics question: once, on the first run, Enter declines | 10-23 f783aca5 | P2 | SAFE-32 |
| E25 | Tools that change files through a language server need a session, as the edit tool does | 10-24 fa4f2028 | P2 | |

## F. Reading and editing files

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| F1 | A read returns 2,000 lines by default | 10-22 1b8146d4 | P0 | TOOL-10 |
| F2 | A read over the size limit is cut at the last line that fits, with where to go on from | 10-22 d640d226 | P0 | TOOL-10 |
| F3 | `offset` is the line number to start at | 10-23 dd6def28 | P0 | TOOL-10 |
| F4 | One read returns at most 25,000 tokens (`CLAUDE_CODE_FILE_READ_MAX_OUTPUT_TOKENS`) | 10-25 8b1f6a02 | P0 | TOOL-10 |
| F5 | `allow_large` reads a section over the limit, up to what fits | 10-25 1fe18d5c | P2 | TOOL-10 |
| F6 | An unchanged file's lines are given once | 10-25 0ed89f52 | P1 | TOOL-10 |
| F7 | A large image is scaled down to fit; a model that cannot see images is told what the image is | 10-25 410dbd20, 8f67a915 | P1 | TOOL-12 |
| F8 | A screenshot's name is tried with the other kind of space before AM or PM | 10-25 1a79d452 | P2 | TOOL-12 |
| F9 | An edit refused for a stale read says what to do | 10-22 1a7ee271 | P0 | TOOL-13 |
| F10 | A write to a file never read says so | 10-23 a0bb4415 | P1 | TOOL-13 |
| F11 | A file written back as it was is not stale | 10-23 327b6b56 | P1 | TOOL-11 |
| F12 | The agent is told when a file it read changes on disk; a resumed session is not told everything changed | 10-24 15911a4e, 07a70ed7 | P1 | TOOL-11 |
| F13 | A file shown by `cat`, `head`, `sed -n` or `grep` in the shell counts as read: only the one file shown | 10-24 19d644bc, 856217b2 | P0 | TOOL-11 |
| F14 | An edit of a file changed since its read is made where it matches exactly, and the file is then unread | 10-24 c0ea3b17, 3737d6f8 | P1 | TOOL-13 |
| F15 | A shell command's result names the read files it changed | 10-23 fa2a71cf | P1 | TOOL-11 |
| F16 | A change of the same size in one clock tick is still a change | 10-30 17acb522 | P1 | TOOL-11 |
| F17 | An edit finds `old_string` written with other quotes or escapes | 10-25 28cbfffe | P1 | TOOL-13 |
| F18 | An edit or write that went in whole says the file need not be read back | 10-25 9dc5bc20 | P1 | TOOL-13 |
| F19 | Several edits at once: each failed edit says why | 10-22 643c4822 | P1 | TOOL-13 |
| F20 | A path that is not there is answered with the working directory and a likely intended path | 10-25 887602b4 | P1 | TOOL-7 |
| F21 | A path under a closed directory says which directory refused, for reading, listing, writing and editing; it does not end the turn | 10-26 9e632e16, 154885b1 | P0 | TOOL-7, TOOL-3 |
| F22 | The model is steered from shell commands to the file tools for file work, and told why | 10-22 aab61a33 | P1 | LOOP-103 |
| F23 | A subdirectory's context files come with the first read, write or edit there | 10-23 586d52f4; 10-24 912e5267, d2971375 | P1 | CFG-20 |

## G. Running commands

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| G1 | Tool output is capped by its real length, not its width on screen | 10-20 beae8e6c | P0 | TOOL-5 |
| G2 | A running command's output is not copied whole ten times a second | 10-24 35fca848 | P1 | TOOL-5 |
| G3 | A wait for a background job is bounded | 10-21 9b01f6ae | P0 | TOOL-17 |
| G4 | A model asking for a job that does not exist is told how jobs come about, and which jobs there are | 10-22 5a17be87; 10-25 bc0adae1 | P1 | TOOL-15 |
| G5 | A job the shell refused to start is not reported as a bare exit code | 11-06 7e4c63a0 | P1 | TOOL-15 |
| G6 | The shell tool takes CC's `timeout` in milliseconds | 10-23 a1a07105 | P0 | TOOL-15 |
| G7 | CC's `BASH_*` limits apply | 10-24 f8981415 | P1 | CFG-51 |
| G8 | A command runs 2 minutes before moving to the background, and says in CC's terms that it did | 10-25 e47789f5, 362bd4ed | P1 | TOOL-15 |
| G9 | The agent is told when a background job ends | 10-25 bac74ac3 | P1 | TOOL-16 |
| G10 | A long foreground `sleep` is refused, with what to do instead | 10-25 99fc7132 | P1 | TOOL-18 |
| G11 | A gnosh tool typed as a shell command is answered with "call the tool" | 10-23 1dfef170 | P1 | TOOL-19 |
| G12 | A command that is not installed, or a lookup that finds nothing, gets a note: use another way or ask; do not search the disk | 10-30 be2f322f, 6d0a1350, 2d66576d | P1 | TOOL-19 |
| G13 | A shell call's exit code is recorded with its result | 10-22 c6fb6e25 | P0 | TOOL-19 |
| G14 | A shell call names the machine it runs on, and every result says where it ran | 10-27 cb4346f9 | P1 | TOOL-20 |
| G15 | A word that is a config directive is an ordinary command when typed in the shell tool (`host example.com` is the lookup program) | 10-27 7f701eb2 | n/a | True by construction: the new gnosh has no shell of its own. |
| G16 | The shell refuses gnokey's password-on-stdin flag | 10-22 ef3412cc | P1 | TOOL-41 |

## H. Searching

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| H1 | An empty result for a pattern with brackets says why | 10-22 9f7c99a3 | P0 | TOOL-14 |
| H2 | A search is bounded by its timeout, and says when it was cut short | 10-22 2d4d178d | P0 | TOOL-14, TOOL-17 |
| H3 | A search keeps its matches when it could not read some file | 10-22 bd5338b4 | P1 | TOOL-14 |
| H4 | Case-insensitive search | 10-22 4fef00d4 | P0 | TOOL-14 |
| H5 | A pattern that starts with a dash is searched for | 10-22 c1d99394 | P0 | TOOL-14 |
| H6 | Grep takes CC's parameters (context lines, file filter, output mode, head limit) | 10-23 c6618032 | P0 | TOOL-14 |
| H7 | Grep shows 250 lines or entries at a time, paged with `offset` | 10-25 38e30655 | P1 | TOOL-14 |
| H8 | Glob searches where an absolute pattern points; newest files first; says when its timeout cut it short | 10-25 0a72d548, e4ab1bfa, 6b211098, 3446a139 | P1 | TOOL-14 |

## I. Web

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| I1 | Web search falls back to a second source while the first rate-limits | 10-25 ee02aa92 | P2 | TOOL-17 |
| I2 | The code-search tool waits as long as the timeout it was given | 10-25 0d15e8fa | P2 | TOOL-17 |
| I3 | Fetching something that is not text says what it is | 10-25 3dde8bd1 | P1 | TOOL-70 |
| I4 | A page longer than the read limit is cut on a character boundary | 11-05 986eb3ad | P1 | TOOL-70 |

## J. MCP and language servers

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| J1 | An MCP tool's text is cut at the output limit | 10-25 f8e69944 | P2 | harness-tools §10 |
| J2 | A binary MCP resource is saved to a file; a long one is shown by its start; embedded resources and links are described; images and audio are saved, and a model that cannot see an image gets the text and the file's place | 10-25 59a1dea1, b7adf683, da6c9815, 27210b37 | P2 | harness-tools §10 |
| J3 | An MCP sign-in token issued without an expiry is not treated as expired in 1970 | 11-05 ad6d587b | P2 | |
| J4 | A language server that fails to start is retried with a back-off; one that is not installed is reported once; its program is looked up the way it will be run | 10-22 85feea22, 087bc7a9, 55e1951a | P2 | CFG-60 |
| J5 | gnosh knows the Gno language server | 10-22 aafcc9b9 | P2 | CFG-17 |
| J6 | A language server's edit that deletes up to an empty line removes that line too | 11-02 e2583ca6 | P2 | |

## K. The turn

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| K1 | An agent caught repeating itself is told so; the turn is not silently ended | 10-22 0a7bac22 | P0 | LOOP-10 |
| K2 | The repetition guard looks only at a round the model will be asked again after | 10-22 32282609 | P0 | LOOP-10 |
| K3 | The guard sees loops of `mem` calls, and two calls taking turns; a loop of refused closes gets its own words | 11-04 (frames QA fixes) | P0 | LOOP-10 |
| K4 | A stream gets a longer budget for its first piece | 10-20 7f8b4d69 | P0 | LM-2 |
| K5 | gnosh waits as long as a local model server takes to start answering | 10-24 594335be | P0 | LM-1 |
| K6 | `API_TIMEOUT_MS=0` means no limit | 10-24 47b6ab28 | P1 | CFG-51 |
| K7 | A model server restart is ridden out instead of losing the work | 10-22 746fcd40 | P0 | LM-3 |
| K8 | A model server that never answered is not waited on for twenty minutes | 10-25 500e89e2 | P0 | LM-3 |
| K9 | A timeout error is never made its own cause | 10-30 3742523a | P0 | LOOP-70 |
| K10 | A model that answers with nothing is asked again | 10-26 b84cef9a | P0 | LOOP-11 |
| K11 | gnosh does not die when tools that run at once finish together | 11-10 14c6b6dd | P0 | LOOP-3 |
| K12 | What was queued behind a failed summary or a failed turn still runs | 10-25 d82838a6, a6aa0829 | P0 | LOOP-7 |
| K13 | A prompt folded into a running turn stays in every later round of that turn | 10-25 505b376f | P0 | LOOP-7, LM-10 |
| K14 | A request's cache mark stays on that request | 10-23 8234766a | P1 | LM-10 |
| K15 | The environment block is sent as the first message, not inside the system prompt | 10-30 39f98ab4 | P0 | LM-11 |
| K16 | `CLAUDE_CODE_MAX_OUTPUT_TOKENS` limits a reply | 10-25 4f7ddc70 | P1 | CFG-51 |
| K17 | `/loop`: keep working until done; no stacking; ends on a session switch; pill; optional cap | 10-21 2af526a2, 94acd115, e70077af, bdd0eded | P1 | LOOP-22 |
| K18 | `/loop` checks in every 25 turns instead of giving up; the check-in asks for a decision first; it reads its own answer; it stops on a denial, a halt or a failed send; it asks its own session before starting | 10-22 cbfdd2bb, 08b34bab, 2e7f0661, 60b49b3e, cbe3d7c2 | P1 | LOOP-22 |
| K19 | `/loop` asks a claim of "done" to show its work before believing it | 10-26 4b117157 | P1 | LOOP-22 |
| K20 | `/goal` keeps the agent working until a check finds the goal met; a goal goes on after a summary | 10-25 c831104c; 10-30 29ddb0e5, aeaa96af | P1 | LOOP-23 |
| K21 | The model is reminded when it has not used its task list for ten replies (now a reminder about `mem`) | 10-25 a4e54946; 10-30 f6e71b31 | P1 | `mem.md` |
| K22 | One task list: the separate todo tool is gone | 10-30 f6e71b31 | P0 | TOOL-1 |

## L. The context limit

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| L1 | A model with an unknown window is compacted as if it had 32,768 tokens | 10-21 7e0663c5 | P0 | LOOP-51 |
| L2 | A model listed without a window gets it from its server | 10-25 4330bea5 | P0 | LM-20 |
| L3 | A summary request begins as the session's own requests do | 10-23 e72cbb2a | P1 | LOOP-61 |
| L4 | CC's auto-compaction variables (earlier only, never later) | 10-24 8af0f08a | P1 | CFG-51 |
| L5 | A summary that does not help is not repeated on every round | 10-24 c5f74fc4 | P1 | LOOP-62 |
| L6 | A conversation past the limit is compacted **before** the prompt goes out, leaving the summary room; a summary the server cut short is not accepted | 10-25 8bb10ec7 | P0 | LOOP-50, LOOP-61 |
| L7 | A long conversation left idle is compacted | 10-24 e131ff40 | P2 | LOOP-64 |
| L8 | The summary is asked for in CC's form, with every message of the user's and the next step in their words | 10-25 18a36e59 | P1 | LOOP-63 |
| L9 | After a summary: the model is told to go on; the replaced conversation is in a file it is told of; recent files, running jobs and loaded skills are given again | 10-25 eb2a5b12, 6c40cdcb, b9610672, 9d204411, b6ec5eaa | P1 | LOOP-63 |
| L10 | A conversation too long to summarize leaves out its earliest turns, cut at one of the agent's messages | 10-25 934f248c, 512bb377 | P1 | LOOP-61 |
| L11 | In frames, the oldest rounds slide out instead of a summary | 10-31 6f75cae1, fd00f508, 2389520b, f74a13a9 | P1 | `frames.md` |

## M. Sub-agents

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| M1 | The search sub-agent is told what CC tells its exploring agent, and the tool asks for a search's breadth | 10-25 363036b4 | P1 | LOOP-80 |
| M2 | The plan agent plans with the project's instructions and environment | 10-25 8bda2072 | P1 | LOOP-80 |
| M3 | Leaving plan mode to carry out the plan puts the plan in `mem` first | 10-30 200deea2 | P1 | `mem.md` |

## N. Config

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| N1 | Config that would be silently ignored is refused: unknown hook events, unknown model slots, unknown MCP types, unknown tool names, wrong-shaped files | 10-22 ca02493f (and the sweep's 18 steps) | P0 | CFG-40 |
| N2 | One validation for first load, reload and every config-writing command | 10-22 (sweep) | P0 | CFG-41 |
| N3 | Warnings raised before logging is set up are kept | 10-22 8c4259a4 | P1 | CFG-43 |
| N4 | When no provider can be used, gnosh says which configured ones were left out and why, and still starts | 10-22 69dcf745; 10-23 34a81129 | P0 | CFG-12 |
| N5 | Every option the settings have can be set from `gnoshrc` | 10-30 7b20c1e0 | P1 | CFG-18 |
| N6 | Context files are read in their configured order, each once | 10-22 60bcd4a9 | P0 | CFG-20 |
| N7 | Context files import with `@path`; they are read from the directories above the working directory | 10-23 e42295b6, aa2b05e1 | P1 | CFG-20, CFG-21 |
| N8 | Context files lose their HTML comments; one over 4 MiB is skipped | 10-23 9f76a5a9 | P1 | CFG-22 |
| N9 | A local server's address works with or without a trailing `/v1` | 10-24 4794e344; 11-05 8b24294d | P0 | LM-20 |
| N10 | With `GNOSH_PROFILE`, a gnosh whose debugging port is taken uses a free one | 10-23 af3fc5ab | P1 | LOOP-70 |
| N11 | Skills kept under gnosh's own folder name are found | 10-25 344d89b7 | P2 | TOOL-61 |
| N12 | No warnings about names from before a rename | 10-23 b8cb6b41 | P0 | PRIN-5 |

## O. What the model is told

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| O1 | The security rule is Jae's: help with defensive work, do not refuse, ask when in doubt and proceed | 10-30 0749d6f6 | P0 | LOOP-100 |
| O2 | Search where things belong, never the whole filesystem | 10-30 ef0d0c87 | P1 | LOOP-103 |
| O3 | Today's date is in ISO form | 10-30 200deea2 | P1 | LOOP-103 |
| O4 | The data folder is never deleted or moved | 11-03 0bdb2abd; 11-04 459758ad | P0 | CFG-3 |
| O5 | A project with Gno code audits its new Gno code against the two guides | 11-06 670d977a | P1 | LOOP-102 |
| O6 | The line about `mem` and Gno goes last on every typed prompt | 10-30 8b9941a9 | P1 | `mem.md`, LM-10 |
| O7 | Which machine the agent is on, and the declared hosts | 10-27 cb4346f9 | P1 | TOOL-20 |

## P. Keys, paying, deliverables

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| P1 | A masked answer to a question, for passwords | 10-22 d60746cf | P1 | TOOL-45 |
| P2 | `gnokey` runs gno.land transactions with a password the model never sees; the password flag's place is fixed by gnosh | 10-22 04e26fc0, 5fd62bf0 | P1 | TOOL-40 |
| P3 | A built-in Gno skill; it carries the two audit guides | 10-22 e1ec93bd; 11-06 670d977a | P1 | TOOL-60 |
| P4 | `x402_fetch` pays for a URL on a test chain, at the user's prompt | 10-22 0b82963b | P2 | TOOL-46 |
| P5 | The network fee is capped; payments that could never settle are refused; a node that answers nothing is an error | 10-23 784ab18f, eb409796 | P2 | TOOL-46 |
| P6 | It says which key store lacked the key, and when to pass the session's owner | 10-23 d1594a6d | P2 | TOOL-46 |
| P7 | After a payment has left, it says only what it knows | 10-28 18d74471 | P2 | TOOL-46 |
| P8 | The payment is built with the public x402 library for gno.land, not a private copy | 10-27 a13ae282, 804c5ec1 | P2 | TOOL-46 |
| P9 | A deliverable is named before it is made, and served at a URL | 10-29 e9537033 | P1 / P2 | TOOL-51, TOOL-52 |
| P10 | Deliverables are served on this machine only by default; a run does not hold the port | 10-30 217e9d79 | P2 | TOOL-52 |

## Q. Diagnosing

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| Q1 | When the crash handler cannot write its crash file it still does its cleanup (the terminal is restored) | 11-01 7838c19d | P1 | LOOP-3 |
| Q2 | Nothing is left running after shutdown | 11-02 b8ea8c1c | P2 | CLI-43 |
| Q3 | Each new trouble with the session store is logged once | 11-04 6e84b850 | P2 | |

## R. Rules for gnosh's own tests

| # | Behaviour | Source | Pri | See |
| :- | :- | :- | :- | :- |
| R1 | A whole print-mode turn is tested against a stand-in model server | 10-23 e04247fa | P1 | PRIN-15 |
| R2 | Tests never fetch from the network, never read the caller's `GNOSH_*` settings, each has a directory and a config home of its own | 10-22 bae85ef0, 7ca48346, a57b73aa; 10-23 (two more) | P0 | SAFE-34 |
| R3 | No real host, address or login in test data | 10-30 (standing rule) | P0 | SAFE-34 |
| R4 | Concurrent code is run under the race detector before it is merged | 11-10 (lesson) | P0 | LOOP-3 |
| R5 | A test that depends on the agent choosing to stop has its own bound | 10-22 (standing rule) | P0 | PRIN-4 |

## S. Asked for or noted, never built

Leads from the same sources. Each would be new work.

| # | Lead | Source | Pri |
| :- | :- | :- | :- |
| S1 | The file tools take `host`, so a file on a declared machine is read without `cat` over ssh | 10-27 hosts follow-ups | P2 |
| S2 | `user` and `container` as further coordinates of a call | 10-27 | P2 |
| S3 | Hold an identical command that failed three times until the model states a different approach | 10-27 | P2 |
| S4 | "Permission denied" says who the agent is running as and where it can write | 10-27 | P2 |
| S5 | The `DirectoryAdded` hook event; `transcript_path` in every hook payload | 10-25 | P2 / P1 |
| S6 | When the server refuses a request as too large, read the real limit from its answer, compact and retry (with a guard against going round) | 10-21 | P2 |
| S7 | The notice that a background command seems to be waiting for typed input | 10-25 | P2 |
| S8 | Binary content from a fetch saved to a file and named, as CC does | 10-25 | P2 |
| S9 | `/model` marks a local model whose server is not answering | 10-25 | P1 |
| S10 | Say when a config file or the program on disk is newer than the running session | 10-26 (seen three times) | P2 |
| S11 | Read the fixed prompt into the server's cache once after an install, so the first new session is not the one that pays | 10-30 | P2 |
| S12 | A shorter form for summaries | 11-01 | P1 |
| S13 | A leaner fixed prompt for small windows | 11-07 benchmark fix "G" | P2 |
| S14 | Pattern-check questions so the model cannot ask for a password in a plain question | 10-22 | P2 |
| S15 | Unattended signing with a session key | 10-22 (designed, deliberately not built) | not planned |
| S16 | Language servers held back in an untrusted folder | 11-01 | P2 |
| S17 | A count, shown to the user, of what side requests cost | 11-07 | P2 |
