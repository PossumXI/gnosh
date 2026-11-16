# harness-ui.md — what Jae needs from the interface

What this file is: what Jae liked, disliked and asked for in the earlier
gnosh's interface, written as **needs**. The earlier interface was a
full-screen application with a sidebar, dialogs and mouse capture. The new
one is inline, in the terminal's own scrollback, like Claude Code's
(ARCHITECTURE.md decision 3). So nothing here describes the old layout to be
rebuilt; each item says what he must be able to see or do.

In one paragraph: the first need is the one he asked for three times: text
on screen must be selectable and copyable with the terminal's own selection,
and scrolling back must be the terminal's own. After that: he must always be
able to see which model is answering, how hard it is thinking, how full the
context is, what is queued, and what the agent has produced. Commands are
typed as Claude Code's are. The look is a night sky in purple and hot pink
with a little green and gold, **crisp**: he reads anything soft as blur.

Read first: `harness-00-principles.md` (PRIN-12: function before bling).
Claude Code's slash commands and keys are in `spec/cc/`.

---

## 1. Selecting, copying, scrolling

### UI-1 The terminal's own selection and scrollback work (P0)

Jae asked on 2026-10-20, on 2026-10-21, and again on 2026-11-09: "i have
crush running in the apple default terminal just fine... when i select text
it says "Selected text copied to clipboard" but it doesn't actually happen.
please fix this."

What was happening: the program captured the mouse and drew its own
selection, then asked the terminal to set the clipboard with an escape
sequence. He works in Apple's Terminal, through ssh, inside tmux, on the AI
box. Apple's Terminal ignores that sequence. So the program said "copied"
and nothing arrived. The answer he was given on 2026-10-21 ("switch
terminals, or hold a key while dragging") did not satisfy him.

The requirement:

- gnosh does **not** take over the screen and does **not** capture the
  mouse. Dragging selects text the way it does in any shell, in Apple's
  Terminal, in Ghostty, locally, over ssh and inside tmux.
- Finished output is written **once**, into the terminal's own scrollback,
  and never redrawn. The terminal's scrollbar, its search and tmux's copy
  mode all see it.
- Only a small region at the bottom is live (the reply being written, the
  input, a status line).
- **What is selected is what was said.** A code block copies as the code:
  no border characters, gutters or line numbers inside the selectable text.
  Prose is not broken into hard lines where the terminal could wrap it, so a
  copied paragraph pastes as one paragraph. (Derived from the need; not
  something Jae spelled out.)
- When gnosh ends, or dies, what was on screen stays on screen. (A fatal
  error's message once vanished because the restarted program drew over
  it; LOOP-3.)

### UI-10 Never claim a copy gnosh cannot know happened (P1)

"It says copied but it doesn't actually happen." Where gnosh copies
something itself (UI-11), it uses the machine's own clipboard when it is
running on the user's machine. Where it can only ask the terminal (over
ssh), it says that it asked the terminal, and that some terminals ignore the
request. It never prints a flat "copied".

### UI-11 `/copy` (P1)

As CC's: copies the last response, and when the response holds code blocks,
asks whether to copy all of it or one block.

### UI-12 `gnosh ssh`: a clipboard relay (P2)

The earlier fix for Apple's Terminal, built 2026-11-09. With native
selection (UI-1) it is no longer needed for dragging; it still makes `/copy`
and other programs' copies arrive from the far side of ssh.

- `gnosh ssh <ssh arguments>` runs `ssh` with the user's own terminal:
  ssh reads the keys, asks for passwords, writes its messages and is
  stopped and continued with the shell's job, as it is alone. Only its
  output passes through gnosh, untouched, and ssh's exit status (or the
  signal that ended it) is gnosh's.
- Each "set the clipboard" sequence it sees in the output is put on **this**
  computer's clipboard.
- A request to **read** or **clear** the clipboard is never acted on.
- Copies are made one at a time, the newest winning, and a stuck clipboard
  never freezes the screen.
- Without a terminal, or with `-f`, gnosh hands itself over to plain `ssh`.
- One line in the user's shell startup file sends interactive `ssh` typed in
  Apple's Terminal through it.

It was checked without ever touching Jae's real clipboard, per his rule that
the clipboard is touched only when he says so.

### UI-13 Colour depth is found, not assumed (P1)

Inside tmux, his terminal's 24-bit colours arrived rounded to 256, because
tmux passes true colour only to a client it knows supports it, whatever the
environment says. Then every gnosh colour was off and a five-letter gradient
came out in three colours.

- gnosh works out what the terminal can show (asking tmux when inside it)
  and draws for that.
- With fewer colours, gradients are replaced by one plain colour (UI-52),
  never by a rounded gradient.
- Links are real terminal hyperlinks where the terminal shows them, and are
  always also printed as text.

---

## 2. What must always be visible

### UI-20 Status (P0 for the first four; P1 the rest)

At a glance, without a command:

| What | Why he needs it |
| :- | :- |
| **Which model is answering** | He switches between two; a session once ran for hours on the wrong one. |
| **How hard it thinks**, in the model's own words ("Thinking Off", "Reasoning Low") | Jae, 2026-10-25: "the right sidebar doesn't show 'low/med/high' effort level. likewise should show for think/nothink for qwen, a general solution please." |
| **How full the context is**, as a percentage of the model's window | The earlier compact header showed this; on a local model it predicts how slow the next round will be. |
| **What state the turn is in**, with the time it has taken: waiting for the model to start answering; the model is thinking; writing; running a tool (which); waiting for a model to load | A local model can be silent for minutes, and he must be able to tell "slow" from "stuck" (LOOP-70). **One** line only: two "working" lines at once was a bug he reported twice (LOOP-6). Nothing in that line may look like data: an animated scramble of characters beside the timer once read as an ID. |
| What is queued (LOOP-7) | He asked. |
| The session's name | He resumes by it. |
| The working directory, and the machine | PRIN-11. |
| The permission mode when it is not the default (plan, skipped, safe mode) | So that a mode is never on by surprise. |
| `/loop` on; a goal set | LOOP-22. |
| The open work in `mem` | The other spec set. |
| Background jobs running | TOOL-15. |

And, on demand, as CC's commands of the same names: `/status`, `/context`,
`/hooks`, `/permissions`, `/doctor`, `/tasks`, `/diff` (what the session
changed), `/mem`.

### UI-21 `/model` (P1)

Lists the models of every usable provider; says which providers were left
out and why; marks a local model whose server is not answering. Left and
right arrows change the highlighted model's thinking level, as in CC's
picker. Choosing saves the choice for later sessions (CFG-4); `-m` does not.

### UI-22 The model's thinking is shown (P1)

When a model sends its reasoning, gnosh shows it as it streams, set apart
from the answer, and it can be read back afterwards. Jae asked on 2026-11-12
how to see "the internal dialogue" (of another tool); with a local model the
reasoning is all there, and it is the best clue to why a turn went wrong.
Ctrl+O, the transcript view, shows every thought of the session, with the
rest of the conversation whole (DECISIONS 49); the verbose option writes
reasoning out as it comes.

### UI-23 Tool calls on screen (P1)

- A call is headed by **what it will do, as the model writes it** (the
  command; the file; for a Gno call, the code, highlighted), not by a name
  and a spinner (LOOP-4).
- A command on another machine leads with the machine's name.
- A failed call is marked as failed.
- The hooks that ran on a call and what each said are on the call's line
  (HOOK-40).
- An edit shows as a diff.
- Long output is shortened on screen with a count of what was left out; the
  model's copy has its own limit (TOOL-5).

### UI-24 Warnings (P0)

One plain line each (PRIN-7, PRIN-8), in the conversation, where it will be
seen and stay seen. A warning is never only in a log.

---

## 3. Commands and keys

### UI-30 Slash commands are typed, as in Claude Code (P1)

Jae, 2026-10-19: "i can't type '/nothink'.. it opens a dialog". In the
earlier interface "/" opened a chooser, and its fuzzy matching once picked
"Thinking On" for the word "nothink".

- A slash command is **text typed into the input box**, with completion as
  you type. An exact name or alias always wins over a fuzzy match.
- A command works **wherever it is typed**: mid-line after a space, pasted,
  while a turn is running where that makes sense.
- A command takes **what is typed after its name** (`/rename nightly check`,
  `/compact keep the API decisions`).
- A command **keeps what was attached** with it, and a custom command's
  prompt receives it. (Attachments used to vanish with every command.)
- An unknown command puts the text back in the box.
- CC's names for commands find gnosh's.

The commands he has, each "as in Claude Code" unless noted. Details of CC's
behaviour are in `spec/cc/`; gnosh's differences are in the file named.

| Command | Does | Notes |
| :- | :- | :- |
| `/think`, `/no_think` (`/nothink`, `/no-think`) | Thinking on or off, and it sticks | gnosh's own. LOOP-40. P0. |
| `/model` | Pick the model and its thinking level | UI-21. |
| `/loop`, `/noloop` | Keep working until done | gnosh's own meaning. LOOP-22. |
| `/goal <condition>`, `/goal`, `/goal clear` | Work until a check finds the condition met | `mem.md`. |
| `/mem ...` | The user's own view of `mem` | The other spec set. |
| `/rename [name]` (`/name`) | Name the session; with no name, make one from the conversation | Jae, 2026-10-24: "i should be able to name the session with /rename". A failed naming shows the real error. |
| `/resume [id or name]` (`/continue`) | Go to another session | CLI-4. |
| `/branch` | Copy the conversation into a new session and go on there | SessionStart source `fork`. |
| `/rewind` (`/checkpoint`, `/undo`), or Esc twice on an empty prompt | Go back to before one of the user's messages: restore the files the edit tools changed since, the conversation, or both | The whole conversation is first kept as a session of its own. Changes made through the shell are not undone, and it says so. Reads and context files given in the cut part are forgotten. The chosen prompt returns to the input box. |
| `/compact [instructions]` | Summarize now | LOOP-63. |
| `/autocompact [150k \| auto]` | How full the context may get | LOOP-65. |
| `/context` | What fills the context window | CLI-12. |
| `/status`, `/hooks`, `/permissions`, `/doctor`, `/tasks`, `/diff` | Reports | UI-20. `/hooks` and `/permissions` are read-only, unlike CC's. |
| `/export [file]` | Save the conversation as text, or copy it | A name with no extension gets `.txt`. |
| `/copy` | Copy a response or a code block | UI-11. |
| `/add-dir <dir>` | Let the agent read another directory | CLI-6. |
| `/init` | Write the project's context file | A project never initialized gets a **tip** to run it, shown for 30 seconds. It is never a question that swallows the first keys typed. |
| New session (`/clear`) | Start a fresh session | SessionEnd reason `clear`. |

### UI-31 Keys (P0 Esc and Up; P1 the rest)

| Key | Does |
| :- | :- |
| Esc | Stops the turn, at once, in every state (LOOP-5). Inside tmux, tmux's escape delay must be short or Esc feels dead; the setup notes say so. |
| Esc twice within about 0.8 s, input empty | Opens the rewind list (as CC). |
| Esc twice, input has text | Clears it, and keeps it in history so Up brings it back. The first press says that a second Esc will clear (as CC). |
| Up, input empty, prompts queued | Brings the queued prompts back for editing (LOOP-7). |
| Up, otherwise | History: the user's own prompts only. |
| Enter with only an attachment | Sends the attachment. (It used to be dropped.) |
| A long paste (over ten lines) | Becomes an attachment, and hooks see its text (HOOK-15). |
| `!` at the start of a line | Runs the rest as a shell command of the user's own (as CC). |
| Ctrl-C | As CC's (`spec/cc`). On the way out gnosh prints how to resume the session. |

Keys of the earlier interface that he used, and what replaces them:

| Earlier key | Did | Now |
| :- | :- | :- |
| Ctrl-L | Opened the model chooser | `/model` |
| Ctrl-S | Opened the session list | `/resume` |
| Tab | Moved between the input, the conversation and the sidebar, so the sidebar could be scrolled from the keyboard | Not needed: everything is in the terminal's own scrollback (UI-1). |
| Ctrl-B | Showed or hid the sidebar | Not needed. **Do not put anything he needs on Ctrl-B alone:** he works inside tmux, where Ctrl-B is tmux's own prefix key and has to be pressed twice. He asked why it took two presses. (Claude Code uses Ctrl-B to send a running command to the background; gnosh must offer that another way as well.) |

### UI-32 Questions for the user (P1)

Several questions in one call are shown together, one tab each, with a
confirmation at the end; Jae used this for answering batches. A secret
question is a masked line (TOOL-45). A permission question shows exactly
what will run (TOOL-35). No is the default for anything dangerous
(SAFE-4).

### UI-33 Deliverables (P1)

TOOL-50. The need: at any time he can see **the latest version of each
thing the agent produced for him**, apart from and above the list of files
it merely changed. In an inline interface that means: a line when one is
claimed, a line when it is delivered or replaced, and a command that lists
them.

How an entry is shown, from his reaction on 2026-10-30 ("i don't like how
'hebrew ad video' is multicolored... and the file name is clipped with
'...', and it's not even a URL... is it possible to make the sidebar
artifact url clickable? or at least click url to copy to clipboard"):

- the label in **one plain colour**; at most a small coloured mark beside it
  to say file or link;
- the path or URL **whole**, wrapped if need be, never cut with "…";
- a URL where there is one (TOOL-52), as a real link where the terminal
  shows links, and always as text he can select.

### UI-34 Notifications (P2; OQ-27)

When the terminal is not in front: a desktop notification when a permission
question waits and when a turn finishes. `option notifications` chooses how
(`auto`, `native`, `osc`, `bell`, `disabled`). Jae has it `disabled`.

---

## 4. Seeing and changing what the model is sent

### UI-40 The session page (P2; OQ-14)

Jae, 2026-10-19, wanted a web interface "to click/type to modify the chat
log in my local AI instance… to learn the ropes".

`gnosh session web` served a page on this machine: the sessions on the left
(one opens from its address, `?session=<id>`); each message's parts shown
and editable in place (text, thinking, a tool call's input, a tool result's
content); save and delete per message; add a message at the end; and a
panel "what the model gets" with the instructions in effect, each tool's
description, and rough token counts.

For the new gnosh: the transcript is append-only and the model is sent a
*view* of it (ARCHITECTURE.md decision 4), so "edit a message" becomes "add
a correction the view honours". Recommended: build the read-only page with
the "what the model gets" panel first, and decide editing with Jae. Security
rules for the page: SAFE-23. Its port must not be the deliverables server's,
and when the port is taken it uses a free one unless the user named it.

### UI-41 Replacing the instructions and tool descriptions (P1)

CLI-12, LOOP-101. Rules from the mistakes:

- A file the user exported and never edited changes nothing (it is
  recognized as an unedited export).
- A file the user did edit is in force, and every place that shows the
  instructions says so, and says when it is older than the build.
- Prefer overriding one named piece over replacing the whole.

---

## 5. The look

Function comes first (PRIN-12), but Jae cares about this and corrected it
more than a dozen times in two weeks. These are his directions, in order.

### UI-50 Direction (P2)

- 2026-10-24: "in the likeness of crush (lots of purple fade bling) but
  better, in our own colors, more green and yellow i suppose like the
  cosmos."
- The same evening: "keep the original colors too, just *add* the green and
  yellow hints on top", and "use green, yellow sparingly, more purple, hot
  pink, like the cosmos nebula, and not solid colors but like crush, faded
  colors, make it look like outer space. scintillating sparkling... as for
  links and such... use best judgement based on what people would expect."
- 2026-10-25, after a pass with green and gold only as accents: "now it's
  just purple and pink again i wanted the fade to include green and yellow
  too."

So: **a night sky; purple and hot pink carry it; every fade runs green,
purple, hot pink, gold; green and gold are otherwise used sparingly.** The
green is a mint (`#00FFB2`) and the gold a warm yellow (`#FBD237`), both
gnosh's own. The purple and the pink of the earlier look were its
upstream's brand colours; the new gnosh picks a purple and a hot pink of its
own (OQ-26).

### UI-51 Crisp. Nothing soft. (P2)

Three corrections in a row, each removing a soft effect:

- 2026-10-28: "mmm i don't like the blur effect on the banner. make it more
  neat."
- Then: "it's still blurry, unblur plz, no solar wind effect."
- Then, when everything soft had been removed: "you removed the purple haze
  entirely... add that back in, just not interacting with the word mark."
- Finally: "uh, it was better before. just do a top bottom fade, purple haze
  on top."

Rules:

- **He reads any blur, glow or haze as unclean.** Edges are crisp; colour
  lives in gradients, not in soft focus.
- Nothing soft behind letters.
- A haze he does like: one plain fade from top to bottom, purple at the top.
  Offer the simplest form first.
- **Everything is in open space.** No drop shadows ("in space doesn't make
  sense"), no horizon, no planet, nothing that implies a ground or a light
  from below.
- No stars inside the wordmark, and no letter stretched for effect ("the L
  and S look stretched and weird").
- When he objects to one effect, remove that effect where he objects, not
  the whole element; and when he says "blur", remove every soft thing at
  once rather than dimming it.
- When he names a visual element, check every place it is drawn before
  calling it done.

### UI-52 Words the user reads are one colour (P1)

Jae, 2026-10-30, of a label drawn through the brand gradient: "i don't like
how 'hebrew ad video' is multicolored".

- A gradient is for the brand: the wordmark, the lines beside it, art.
- A label, a file name, a path, a URL, a model's name, a status word: **one
  plain colour**, with at most a small coloured marker beside it.
- Nothing the user reads is cut with "…". It wraps.
- Links look the way people expect: underlined, in a colour that fits.

### UI-53 The wordmark (P2)

- It reads **GNOSH**, bold.
- In a terminal with full colour it fades through the whole range, spread
  **evenly** over its five letters, so all four colours show (green G, gold
  H). Jae, 2026-11-09, of a header where the letters came out green,
  purple, purple, pink, pink beside a flat line: "the GNOSH /////// looks
  lame... the color of the letters GNOSH should blend better; and there's no
  fade in the //////// at all, even though clearly it is possible".
- Any line or rule beside it fades too, hot pink next to the word cooling to
  purple away from it.
- In a terminal without full colour, "GNO" is one green and the rest runs
  purple to pink. Jae, 2026-10-30: "at least make 'gno' one color, green i
  suppose", then: "i meant in basic terminals like apple default terminal...
  since it doesn't support the full color spectrum. in ghostty ... i want
  the original blend which looked better."
- A second line with the site and the version sits **under** the word, not
  above it.
- `--version` and the line printed at exit use the same mark.

### UI-54 Words (P2)

The headline, Jae's choice on 2026-10-31: "An AI harness that thinks in
Gno." The tone of gnosh's own messages is plain (PRIN-7).
