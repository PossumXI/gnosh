# open-questions.md — what the sources contradict or leave unsettled

What this file is: every place where the sources for this set disagree, stop
short, or wait on Jae, each with the reading this set took so that nobody
has to stop and ask. "Needs Jae" marks the ones only he can settle.

Order: the ones that change what gets built come first.

---

## A. Needs Jae

### Q1. A todo with nothing under it, entered by a session that then ended, stays held for good

- **What the sources say.** Jae, 2026-10-31, on the held-frame lock: "just
  leave it as is, we will solve it differently later." On 2026-11-05 ("ok do
  both") the hold on a todo *with todos under it* became soft: it stands
  only while its session works. "A todo with nothing under it stays held
  (the 2026-10-31 ruling, for leaves)."
- **What it costs.** "In one run of the four-session plan, the last session
  could not take the step the one before it had entered." Every
  non-interactive run is a new session, so a leaf one run stepped into and
  left is stuck until the user resumes that session by id.
- **Reading taken.** Build it as landed: soft for plans, hard for leaves.
  The refusal tells the model to go on with what is free or tell the user,
  and not to look for the session.
- **For Jae.** Make every hold soft (a hold stands only while its session
  works)? It would end the stuck leaf, at the cost that a session idle
  between turns can lose the todo it is in to a peer.

### Q2. Should a pop spare everything the user said while the model stood in the frame, or only this turn's words?

- **Jae's ruling** (2026-10-31): "a pop removes everything, the words
  included; what matters is kept as a thought".
- **Landed 2026-11-04** (A18): a pop spares the prompt rows of the turn it is
  made in. QA argued further: "the pop's sparing should cover every prompt
  that arrived while the model stood in the frame, not only this turn's. The
  user said them there, but they were never part of the todo's work."
- **Reading taken.** As landed: this turn's prompts stay; earlier turns'
  words go with the frame. The receipts hand a frame's thoughts back word
  for word, which covers the case the ruling leans on.
- **For Jae.** Confirm, or widen the sparing.

### Q3. The 32K overflow

- **Seen** (bench, 2026-11-07): stock gnosh "ran its prompt to 40.5K at a 32K
  window, after four slides. On a server with a real 32K slot that request
  fails." Jae: "make note to fix it later".
- **Not diagnosed**: "either the slide threshold is tuned for big windows,
  or the window it discovered was not 32K."
- **Reading taken.** The slide as specified (`frames.md` §8). When it is
  rebuilt, test it against a server whose slot really is 32K, check that the
  window the harness believes is the server's, and check that the fixed
  prefix (about 14–15K tokens in the earlier build: system prompt, 29 tool
  schemas, reminders) leaves the body room. At a 24K window that prefix left
  about 4K of room and the session slid almost every round.
- **For Jae.** Nothing to decide; a known defect to carry.

### Q4. One step per reply: the gap to Crush on short tasks

- **Measured** (2026-11-07, 64K window, same model): both harnesses passed
  every task; Crush was 1.3–4× faster on each. "crush fixed the six bugs in
  4 replies: 12 views in one, 6 edits in one. gnosh's runs took 16 to 40
  replies, most with a single tool call." Causes named: the verification
  discipline, one step per reply, and a fixed prompt about 3.3K tokens
  larger. "Prompt variants cannot override" the discipline.
- **Proposed, not built**: make the discipline proportional in code (H: no
  review when every file the result names has an edit in the window and a
  test passed; the mem reminder only once the task has grown); a leaner
  fixed prompt for small windows (G); a push of a new plan also entering its
  first todo (D).
- **Reading taken.** Build the rules as landed. Keep the fixed prompt as
  small as the new harness allows; the memory section alone was a quarter of
  the old prompt.
- **For Jae.** Whether to trade some of the checking for speed, and how.

### Q5. Swarm step 4: syncing a session's mem with the realm

- **State.** The realm, the chain client and demo A are done. The bridge
  (a session's shared calls as transactions, the watcher, the renewer) is
  recorded as "STOPPED (a safety classifier; not to be resumed or
  reworded)". After Jae said "do it yourself please", one sending function
  was written, tested live and landed on 2026-11-01; it was removed on
  2026-11-05 because nothing called it, and the Gno-side hooks were parked
  on a local branch marked never to land.
- **Reading taken.** Carry what exists. Do not build the bridge as part of
  this rewrite. `swarm.md` §6 keeps the recorded design.
- **For Jae.** Whether and how to take it up; demos B and C wait on it; and
  whether a `run --swarm N` flag should exist for local peers.

### Q6. Frames on gno.land (a local, unpushed branch)

- **State.** A frames realm, `/mem frames` and `gnosh frames export`, three
  local commits of 2026-11-06, verified on a dev chain, held back until the
  provisional patent was filed (it was, that day). "Jae decides when."
- **Reading taken.** Not part of main, so not part of what is carried by
  default; described in `swarm.md` §9.
- **For Jae.** Land it or hold it; carry the branch into the new repository
  or not; which key deploys the realm.

### Q7. Secrets

- "The recipe line 'Never keep a secret, or offer to' loses to an explicit
  'remember this key': the value went into mem 3 times in 5." The model has
  done all three on its own: refused; written the key to an unasked
  plaintext file with a fact pointing there; kept it with a warning.
- **Reading taken.** Tell the model never to keep one; provide Redact and
  Forget; no scanner.
- **For Jae.** "Refuse a secret even when asked to keep it, keep it with a
  warning, or leave" it.

### Q8. Does an erasure also scrub the visible messages?

- Landed: an erasure scrubs the harness's own copies of mem's text (hidden
  parts, summaries, bindings, task lists) and **names** the sessions whose
  visible messages still hold a redacted secret; it leaves what the user saw.
- **For Jae** (open since 2026-10-30): scrub those too?

### Q9. Where Jae's existing mems are, and under which name

- The gnovm data directory is `gnovm/` beside the global data file. The
  earlier program read older product names as legacy (`crush`, `klash`); a
  QA home kept mem under the `crush` name, and Jae's AI-box lane runs as a
  user named after the first of them.
- **Reading taken.** The new harness writes `<XDG data>/gnosh/gnovm/`. Before
  the switch, find each existing mem on the AI box and the Mac, and either
  read the legacy location in place or move it once. Do not start an empty
  mem beside a full one.
- **For Jae.** Nothing unless a mem is to be left behind.

### Q10. Licence

The carried directories are under the GNO Network GPL v6 with a Strong
Attribution clause, and name Jae as licensor. The earlier binary also held
code under another licence, which conflicted. The rewrite removes that
conflict. What licence the new harness as a whole takes, and what the
attribution Designation reads, are Jae's.

### Q11. Things still waiting from the mem v2 notes

- The mem line's wording: with thinking off, one-off answers were still kept
  as facts about 4 times in 10. This set keeps the line and the "not what
  you worked out for a one-off question" clause.
- Journal backups to the backup machine.
- Turning recall on in his own lane ("his call because his prompts go to
  it").

---

## B. Contradictions between sources, and the reading taken

### Q12. Machine options that were designed and never wired

DESIGN §13 specifies `options.gnovm` (`memory_budget_mb`, `default_memory_mb`,
`default_gas`, `ns_per_gas`, `goals`, `in_process`), honoured only from the
user's global config. At main the harness passed only the data directory and
the embedding URL; the only two options in the config are
`mem_embeddings_url` and `mem_review_closes`. **Reading:** the two options
are required; the rest is P2, and if built, a project file must never raise
the budget or set `in_process`.

### Q13. Does `mem.Next()` work at the unbound root?

FRAMES.md draft 13: "At the unbound root it is nil: a todo is entered by id
or made and pushed, never wandered into." Draft 15 (same day) and layout 6:
`mem.Next()` at the unbound root steps into the next free todo, "so a
session taking up another's work needs no name". **Reading (newest):** it
works, in creation order. `Frame.NextTodo()` on the root frame still returns
nil; only `mem.Next()` walks the top level.

### Q14. In what order does the root list its todos?

MEMPLAN and FRAMES.md: "newest first". Layout 6 (2026-11-04): "in the order
made, as Next takes them", because "listed newest first, a model following
the outline by name would still run steps backwards". **Reading (newest):**
order made, in Show, the digest and the JSON summary. Closed todos and facts
are still newest first.

### Q15. Sub-agents and mem

MEMPLAN: "sub-agents get no mem". PARALLEL.md handles a sub-agent that calls
mem ("Sub-agents never wait"; "a sub-agent is a session of its own, so it
may take a free todo under one its parent holds"). At main the stock
sub-agent's tool set had no `gnovm_run`, and the tool carried a sub-agent
flag for agents that did. **Reading:** stock sub-agents have no gnovm tools
and get none of mem's context; the flag and the never-wait rule are kept for
any agent that is given the tool.

### Q16. What does the goal judge read?

The design: "the goal judge, a model, also reads the full list: a frame
popped inside a goal has its evidence there." The landed judge reads the
whole session, clipped to its last 24,000 characters. But a QA note of
2026-11-04 says the judge "said 'Goal met' because it sees only the cut
conversation". Those cannot both be so; the likeliest explanation is that
the user's instruction had fallen outside the 24,000-character tail
(unconfirmed). **Reading:** the full list, as designed. Since the judge is
also shown `mem.Show(name)`, thoughts kept on the todo reach it.

### Q17. Thought addresses

Jae, 2026-10-31: "todo#thought --> I don't get this... why do we need the
'todo#' at all?" Yet Ops still name a thought `item#name`, and
`mem.Show("x#thought")` was made to work on 2026-11-04 because a model asked
for it and then read the files again. **Reading:** a thought is its name;
the address is storage, never printed as something to type, and accepted by
Show as a courtesy.

### Q18. The mem reminder while a goal is bound

MEMPLAN: the reminder "is muted while a goal is bound". The landed reminder
did not check the binding (as read). **Reading:** not muted (as landed);
unconfirmed which is wanted. A bound session gets goal continuations at each
turn's end anyway.

### Q19. Every goal judged, against a reviewed close not being judged again

Jae, 2026-10-30: "all goals should be judged before deemed complete". Since
2026-11-05 a top-level todo with nothing under it whose close the review
approved is not judged again at the turn's end. **Reading:** the review *is*
the judging for that case (it sees more: the window, the files changed, the
commands run). With the review turned off the turn-end judge runs.

### Q20. Hidden reminders in the middle of a turn, and the prefix cache

The mem reminder, the slide's line and the large-step line are inserted as
hidden user-side rows. On two chat templates (GLM, Gemma) that makes the
server re-read the whole turn (16,746 and 19,727 tokens against 121 and
227); on Qwen and gpt-oss it costs nothing. Putting them inside the tool
result fixes it but "crosses gnosh's tool-output trust line". **Reading:**
as landed (user-side rows). The new harness carries tool results in
user-side messages already; whether a reminder appended to the same message
keeps the cache on those templates is worth one measurement before choosing.

### Q21. The fresh guard was never ruled on by Jae

The guard (2026-11-01) "is a new rule in Jae's frames design: tell him, he
may want it different." It was then tightened three times from real false
closes, and Jae's "do both" (2026-11-05) and "do it" (2026-11-11) approved
the checks built on it. **Reading:** as landed (`frames.md` §9).

### Q22. Leftovers in the mem API

- `mem.Start` (a hold with no frame): "a leftover"; models take it for
  "enter". Kept working, not taught.
- `mem.Note` beside `mem.Think`; `mem.Find` beside `mem.Recall`: two ways
  each. Kept.
- The package comment of the mem library still shows `mem.Goal(…)` in its
  example list, though the call is gone.

**Reading:** carry as is; these are the carried code's to clean, not the
harness's.

---

## C. Proposed in the sources, not built (a backlog, in rough order of evidence)

1. **A13: check a pop's result against its todo's title.** A model did the
   API reference inside the migration-guide todo and recorded the guide
   done. The prompt rule "enter the todo the user names" took true records
   from 1 of 5 to 3 of 3; the review's "other work" rule now catches some.
2. **A14: a todo made twice at two levels with no word from mem.**
3. **A9: forget a thought by its name** (today only Redact removes one).
4. **B4: the digest's `next:` says how to step in** ("step in with
   mem.Next()").
5. **A3 / A2: one search, one way to keep a thought.**
6. **A17: Drop notices a reason that says the work is done** and points at
   `mem.Done`.
7. **Drop the echo of the result in the pop receipt** (about 80 tokens a
   pop).
8. **B2: a shorter memory section**: one worked example in place of three
   bullets of prose; a ~600-word draft exists in IMPROVEMENTS.md.
9. **Recall by words alone** when there is no embedder.
10. **A29: local mem's fixed cost per call** (about 0.2–0.37 s), which caps
    many peers at about 2.7 calls a second.
11. **A fold or marker for popped rows in the chat** (the stored marker is
    there for it; Jae ruled none in v1).
12. **A way to un-mark `.Parallel()`.**
13. **Self-trimming of a stack**; offering todos left by ended sessions to
    Next (subsumed by Q1).

---

## D. Unconfirmed details (marked so in the other files)

- The exact compaction point between "small" and "large" windows. The
  sources give: the window minus 20,000 for a large window (262,144 →
  242,144); 80% for small ones (4,000 → 3,200; 24,576 → 19,661; 32,768 →
  26,214). Where the two meet, and the user overrides, are the harness
  specs'.
- The stop marker's text, and how many continuations make "every tenth".
  MEMPLAN says tenth; the constant is the harness's.
- How the earlier UI showed a `gnovm_run` call waiting on peers.
- The timeout of the goal judge call.
- Whether the earlier tree still had `klash/gnomem/v1` reserved anywhere
  beyond the name (its code is gone from main).
- A pop event for a frame that is not on the derived stack. The earlier
  code recorded no range for it when deriving from rows, yet on the live
  path still flagged the rows since the last summary. `frames.md` §3 says to
  record nothing. The case cannot arise while the harness passes its stack
  to mem on every call, so neither path was ever exercised (unconfirmed).
- There is **no "mem board" view** in the earlier UI: `/mem` prints text,
  the task list shows mem's todos, and the status area shows notices. If a
  board or tab view is wanted it is new work.

---

## E. Things the clean side should know about the sources

- **MEMPLAN.md was never committed.** It exists only as an untracked file in
  a local worktree. Everything of it that still holds is restated in
  `mem.md`; drafts after 7 were overtaken by the frames work.
- **FRAMES.md names functions of the earlier harness** in its sections on
  where the state lives. Those names are not carried into this set; the
  behaviour is.
- **The docs are tests.** GNOMEM.md's examples and the README's mem examples
  were replayed byte for byte by a test, and each row of GNOMEM's scenario
  table was tied to its scenario file. If the receipts' wording changes, the
  docs change with it, or the landing fails.
- **Prompt text is A/B'd before it lands** (mem v2 notes): two binaries,
  interleaved runs, thinking off. A clause that sounded right scored 3 of 5
  against 5 of 5 without it.
- **When replaying a judge, send what the harness sends**, thinking switch
  included.
- **Runs go on the devbox or the AI box, never the Mac**, and concurrency
  tests run with the race detector before landing (a crash on 2026-11-10
  came from parallel tools finishing together; it was in the harness, not in
  mem, but it took the lane down).
