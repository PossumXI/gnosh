# harness-local-models.md — running against llama.cpp and Qwen on the AI box

What this file is: everything four weeks of daily use taught about driving a
model that runs on the user's own machine, written as requirements for the
part of gnosh that talks to model servers and for everything that decides
what a request holds. Almost all of it was learned from an incident: a
"Provider Error" after exactly ten minutes, a session that re-read 180,000
tokens for twenty minutes, a server that crashed every hour, a second model
loaded by accident that threw out the first.

In one paragraph: a local model server has a few slots, and each slot
remembers one conversation. A request that begins exactly like what a slot
already holds is answered in half a second; one that does not must be read
from the start, at a few hundred tokens a second, which for a long
conversation is minutes. So gnosh must (1) wait as long as such a server
takes, (2) never change a byte of what it sent before, (3) never let a
small side request push the main conversation out of its slot, (4) never
name a model the user did not choose, and (5) tell a slow server from a
dead one.

Read first: `harness-00-principles.md` (PRIN-9). The wire formats themselves
are public (the OpenAI-compatible chat-completions API; llama.cpp's server
documentation).

---

## 1. The machine this is for

So that the numbers below mean something. (Measured between 2026-10-18 and
2026-11-07.)

- One box at home: a processor with a built-in GPU and 128 GB of memory
  shared between the two. No separate graphics card.
- `llama-server` from llama.cpp, in **router mode**: one port, several
  models declared, **one loaded at a time**. A request that names another
  model unloads the one in use and loads the one asked for (about 25
  seconds for Qwen, about a minute for the bigger model).
- The daily model: Qwen3.8-27B, about 262,000 tokens of context **per
  slot**, **3 slots**, with speculative decoding and a prompt cache in RAM.
  It thinks by default.
- A second, larger model (gpt-oss-120b) that he sometimes switches to.
- A separate small embedding server for `mem`.

| What | Speed |
| :- | :- |
| Reading new tokens, context under 10K | 267 to 291 tokens a second |
| Reading, context 10K to 40K | about 178 |
| Reading, context 40K to 80K | about 131 |
| Reading, context 80K to 120K | about 100 |
| Reading, context over 120K | 53 to 70 |
| Writing, context 10K to 20K | about 14 tokens a second |
| Writing, context over 80K | about 10 |
| Writing with thinking off, small context | about 22 |

What that means in time:

| Event | Cost |
| :- | :- |
| A new session's first request, nothing cached (about 15K tokens of fixed prompt) | about 48 s; 70 to 90 s when it was 17K to 22K |
| The same, with the fixed prompt already in a slot | about 0.5 s |
| A conversation of 180K tokens that lost its slot | about 20 minutes before the first word |
| One summary of a conversation (median 4,500 tokens written) | 5 to 6 minutes; 17 at worst |
| A session title | seconds, but see LM-14 for what it used to cost |

A short context is where every tool result is read three to five times
faster. That, as much as the window, is the reason for frames.

---

## 2. Waiting

### LM-1 No limit on how long a local server takes to start answering (P0)

Jae, 2026-10-24: "ai box said 'provider error' what was that about". His
session showed a timeout "awaiting response headers" exactly ten minutes
after his prompt.

What happened: three programs shared the server. Another one's request lost
its slot and began re-reading 185,000 tokens. The server fills each batch
slot by slot, so Jae's request, though 98% cached, got no processing time
and sent **nothing at all** for ten minutes. The ten-minute limit was not
gnosh's: it is the common default for "how long until the response starts"
in HTTP clients for model APIs, and Claude Code's default too.

Requirement:

- For a **local** model server there is **no limit** on how long the
  response takes to start.
- For a model API elsewhere the limit is ten minutes, as in Claude Code.
- `API_TIMEOUT_MS` sets the limit in milliseconds for either, and `0` means
  none (CFG-51).

"Local" means: `localhost`; a loopback address; a private address; a
link-local address; an address in the shared range that private overlay
networks such as Tailscale use (100.64.0.0/10); a host name of one label;
or a name under `.localhost`, `.local`, `.lan`, `.internal` or `.home.arpa`.

### LM-2 The idle limit, and the longer budget before the first piece (P0; OQ-13)

`option request-timeout <seconds>` (default 60; Jae's 900) is how long a
streaming reply may go quiet **between pieces** before gnosh gives up on it.
It is reset by every piece. It is not a limit on the whole request.

Before the **first** piece arrives the budget is **five times** that. A
local model re-reading a long conversation streams nothing while it reads
(about 50 s for 15K tokens, minutes for more), and a request making steady
progress was being cancelled in the middle of reading at the old two-minute
default. Jae's comment in his own config: "Re-reading a long conversation
the single slot has dropped takes minutes; the default 2-minute wait
cancelled it." Once the first piece has arrived, a true stall in the middle
of a reply is still caught at the plain limit.

- The error says which budget ran out: "no reply started within N" or
  "nothing received for N".
- `0` waits for ever.
- Unconfirmed: whether the server's keep-alive bytes during a long read
  count as pieces. If they do, the limit can never fire on a stalled
  connection; if they do not, a long read needs the bigger budget. The new
  gnosh must decide and test this against a real `llama-server`.

OQ-13 recommends going further: for a local server, **no limit before the
first piece** unless the user sets one, so that the line gnosh.ai tells
every new user to add (`option request-timeout 900`) is not needed.

### LM-3 A server that stopped answering, and one that never did (P0)

Two different situations that look alike.

**It was answering and stopped** (it is restarting, or loading a model): the
connection is refused or reset, the stream ends early, or it answers 502,
503 or 504 ("loading model"). gnosh **waits for it to come back, up to five
minutes, reconnecting**, and then goes on with the turn. The first version
of this was built after a sub-agent lost an hour of work to a restart that
the main agent survived.

**It has not answered once since this model was chosen** (it is not
running): gnosh waits **fifteen seconds** before each of its few retries, so
the user is told in about a minute and a half. Before this rule, switching
to a model whose server was off looked like a twenty-minute hang.

- The message names the server and its address and says plainly which case
  it is: "did not come back within five minutes", or "is not running".
- gnosh never logs or shows "reconnected" unless it did reconnect.
- While waiting, the status says it is waiting for the model server, and
  Esc cancels.
- A failed request is retried a few times with a short, growing pause
  (5, 10, 20 seconds), counted **per failure**. The retry budget of a long
  round must not be used up by failures that happened an hour apart.

### LM-4 Errors (P0)

A provider error shown to the user always has the provider's name, its
address, and what happened in plain words (PRIN-7). The kinds, as Claude
Code names them for hooks: `rate_limit`, `overloaded`,
`authentication_failed`, `billing_error`, `model_not_found`,
`invalid_request`, `server_error`, `unknown`. A local server that refused,
never answered or timed out is `server_error`.

A request the server refuses as too large for its context must never be
sent again as it is (LOOP-50); a session that keeps resending it is dead.

---

## 3. Thinking, on the wire

### LM-5 How a thinking level reaches each kind of server (P0 for the first row)

| Server type | Model family | What is sent |
| :- | :- | :- |
| `llamacpp`, `openai-compat`, `ollama`, `litellm`, `omlx` | Qwen and other on/off thinkers | In the request body: `"chat_template_kwargs": {"enable_thinking": true}` or `false`. |
| the same | gpt-oss (three levels, no "off") | `"chat_template_kwargs": {"reasoning_effort": "low"}` (or `medium`, `high`). The on/off switch means nothing to it and is dropped by the server. |
| `lmstudio` | either | Its own top-level `reasoning_effort`: `"none"` for off (which turns a Qwen's thinking off); a level as it is; and **nothing** for "on", so the setting chosen in LM Studio itself stays in force. It ignores `chat_template_kwargs`. |
| `anthropic`, `openai` | | Their own documented fields. |

- Writing `/no_think` into the prompt text is **not** a switch: newer Qwen
  ignores it. Only the request field works.
- The model's reasoning arrives apart from its answer and is stored as
  thinking, not as answer text (UI-22). When a short answer has to be read
  as data (a title, a JSON verdict), any thinking that leaked into the text
  is stripped first, and the JSON is taken from the first `{` to the last
  `}`.
- Side requests ask for the lowest level (LOOP-42).

---

## 4. The server's cache: never make it read twice

### LM-10 What was sent before never changes (P0)

A llama.cpp slot reuses what it has read **only up to the first token that
differs** from the new request. So the rule for everything gnosh sends:

> For a given session, each request begins with exactly the bytes of the
> request before it, plus whatever is new.

What this forbids, each of which the earlier gnosh did at some point:

- **Adding something to the newest user message only, and dropping it from
  older ones.** The line about `mem` that goes on every typed prompt, a
  hook's added context, and recalled facts are all **stored with the
  prompt** and resent identically for ever. Stored, the `mem` line costs
  about 75 tokens per prompt; not stored, the server re-reads the whole
  previous turn, tool output included, on every prompt.
- **Building a message differently the second time.** The first send of a
  prompt and its later rebuild from storage must be byte-identical: the same
  trimming, the same tags, the same order. (A prompt that was trimmed when
  rebuilt but not when first sent never matched the cache.)
- **Changing the fixed part** (LM-11).
- **Rewriting the middle of the conversation.** Frames and the slide cut
  from the context in ways that keep the beginning unchanged (`frames.md`),
  and a cut is paid for once.
- **Putting a marker for a hosted provider's cache on a shared object.** A
  request's cache marks belong to that request. (One mark leaked onto every
  later request until a hosted API refused them for carrying more than it
  accepts.)

A test for this: build a session's requests one after another against a
stand-in server that reports, for each, how long a prefix it shares with the
one before. Anything less than "all of it" is a failure with a reason.

### LM-11 The fixed part is identical across sessions (P0)

The system prompt and the tool definitions are the same bytes in every
session with the same config. What differs per session (the working
directory, the date, the git status, the machine) is sent as **the first
message of the conversation**, not inside the system prompt.

Why: the tools are rendered after the system prompt, so one differing byte
in the system prompt made every new session re-read the prompt **and every
tool**. Measured on the box: 70 to 90 seconds per new session; after the
change, a request sharing a 10,000-token beginning read 19 new tokens in
half a second, against 33 seconds.

Corollaries: tool definitions are emitted in a fixed order; nothing with a
time, a random value or a counter appears in the fixed part; the user's
global context files are part of what must stay stable between sessions.

One cost that remains and is accepted: the first session after a new build
that changes the instructions reads them once.

### LM-12 A summary request begins as the conversation does (P1)

LOOP-61.

### LM-13 Where gnosh's own mid-turn notes go (P1; OQ-19)

gnosh sometimes speaks to the model in the middle of a turn: a reminder, a
notice that rounds were slid out, a nudge after an empty reply. The earlier
gnosh sent each as a new hidden message in the user's role.

Measured on 2026-11-06, the same 18K to 21K-token turn, tokens the server
had to re-read:

| Model and server | A normal round | With the note as a user message | With the note inside the tool result |
| :- | :- | :- | :- |
| Qwen 3.8 on llama.cpp (the AI box) | 135 | 32 | 159 |
| gpt-oss-120b (vLLM) | 134 | 32 | 29 |
| GLM-4.7-Flash (vLLM) | 121 | **16,746** | 33 |
| Gemma 4 (vLLM) | 227 | **19,727** (the whole prompt) | 124 |

So on Jae's box either place is free, and on some chat templates a user
message in the middle of a tool exchange throws the cache away. Putting the
note inside the tool result is cheap everywhere but mixes gnosh's words with
tool output, which gnosh otherwise treats as untrusted. OQ-19 has the
recommendation.

### LM-14 Side requests must not evict the main conversation (P0)

A server with three slots holds three conversations. A request that matches
no slot takes the least recently used one, and whatever was there must later
be read again from the start.

What went wrong, each time measured in the server's own log:

- **A session title.** A sub-agent's start sent two requests at once: its
  first prompt and a title for its session (a session nobody ever sees).
  The title took a free slot and the sub-agent took the slot holding the
  **parent's** conversation. When the parent resumed it re-read 75K, 79K and
  110K tokens, six to seven minutes each. Of a 195-minute run, 35 minutes
  went to this.
- **Titles for three new sessions at once** took all three slots.
- **Reviewers.** Three review runs started together took all three slots;
  the run they were reviewing then re-read 196K tokens, about twenty
  minutes, before it could answer.
- **A test run inside the agent's own work.** A nested gnosh run sharing
  the same instructions matched the parent's slot (they begin alike) and
  replaced its 146K-token cache.

Requirements:

1. **No title for a sub-agent's session.** Ever.
2. **A session's title is asked for after its first turn ends**, never
   beside the turn's first request. By then the least recently used slot is
   one no live conversation is in. Until then the session is named after
   its prompt (LOOP-9).
3. Side requests (naming, checking, reviewing) are **few, small, and sent
   when the main conversation is not waiting on the server**. gnosh keeps a
   count of what they cost: on one model they added about 40,000 freshly
   read tokens to a run.
4. A provider can be **pinned to a slot** from config
   (`--extra-body '{"id_slot": 1}'`), so that reviewers and other helpers
   can be kept off the slot a long conversation lives in.
5. Sub-agents are the unavoidable case: each is a conversation of its own.
   On a server with N slots, more than N conversations at once means
   somebody re-reads. `doctor` can say how many slots the server has (P2).

### LM-15 Side requests are short and do not think (P0)

Title, todo names, goal check, close review: lowest thinking level
(LOOP-42), a small reply limit, and for a judging request a temperature of
zero. Real close reviews ran a median of about 55 generated tokens. A reply
limit that is too tight makes a model that does think look broken: leave
headroom (the earlier gnosh raised one from 400 to 1,500).

### LM-16 Do not disturb the server while looking (P0)

- To learn which models exist and their state, ask only for **the list of
  models**. In router mode, asking for a model's slots or properties
  **loads** that model and throws out the one in use.
- gnosh's own tests, benchmarks and checks never talk to the user's live
  model server unless that is the point of the check, and then they wait
  for it to be idle and name exactly one model (LM-30). The rule on the
  box: while Jae is using gnosh, nothing else posts to the model server.

---

## 5. Finding models and their windows

### LM-20 Discovery (P0)

- For a local provider with no models listed, gnosh asks the server for its
  models and uses what it finds. A listed model keeps every field the user
  set; discovery fills in the rest.
- **The context window** is the most important thing to learn. A plain
  llama.cpp server reports each model's context **per slot** with its model
  list. In router mode it does **not**, so on Jae's box the windows are
  written in config (`model add ... --context-window 262144`).
- A model listed without a window gets it from the server when the server
  knows.
- When the window cannot be learned, it is 32,768 (LOOP-51), **and gnosh
  says so**: at startup, in `doctor`, in `/status`. Discovery used to fail
  silently inside a three-second limit and leave the window unknown.
- A base address ending in `/v1` and one without both work, wherever the
  address came from. (Twice gnosh asked for `/v1/v1/models`.)
- A provider whose discovery fails is left out with the reason (CFG-12).

### LM-21 A window smaller than gnosh's own prompt (P1)

Some local setups give a model a very small default window (one common
runner gives 4,096 tokens on a machine with little video memory). That is
smaller than gnosh's fixed prompt. `doctor` and the first request must say
so plainly: the model's window is N, gnosh sends M before the first word,
and how to raise it. Without this a newcomer sees only nonsense or an
error.

Likewise when the configured window is larger than the slot the server
really has: a reply that ends on the length limit with usage adding up to
less than the configured window means the real window is that smaller
number. gnosh says so and uses it.

### LM-22 Reply length (P1)

No reply limit is sent on the main request to a local server unless the
user set one (`--default-max-tokens`, `CLAUDE_CODE_MAX_OUTPUT_TOKENS`).

---

## 6. One model at a time

### LM-30 Never name a model the user did not choose (P0)

On 2026-10-30 this happened twice in one day. A scratch config written for
a check named only the provider. gnosh discovered two models, took the
**first one listed** as its default, and sent a request naming it. The
router unloaded Qwen, the model three of Jae's sessions were using, and
loaded the other. Qwen's cache was gone; a queued run hung for fifteen
minutes; the wrong session was blamed for it at first.

The standing rule on the box: any run from a scratch config must know only
the Qwen. For gnosh itself:

- With more than one model available and none chosen, **gnosh does not
  guess** (CFG-13).
- A non-interactive run started by a script passes `-m` and
  `--small-model`. If only `-m` is given, the small jobs use the same model:
  a run must never name a second model by default.
- Every side request uses a model the user chose (the small slot if set,
  otherwise the large one).
- Where the choice lives matters: his model choice was in the state file,
  not the config file, so a copy of only the config file lost it. The
  commands that copy a setup for a check copy both, or pin the model.

### LM-31 Asking, not guessing (P0)

Interactively, with several models and no choice, gnosh asks which one.
Non-interactively it stops with a message naming the models and the `-m`
form.

### LM-32 Listing never loads (P0)

CFG-61, LM-16.

### LM-33 After a switch (P1)

LOOP-75. The first request after `/model` on such a server waits for the
other model to load. A request for an unloaded model is queued by the
server until the loaded one is idle; a reply in flight finishes first.

---

## 7. Replies that are empty or odd

### LM-40 Empty replies (P0)

LOOP-11. Seen on gpt-oss: about one reply in seven had no usable content
(reasoning only, or nothing), because the model sometimes ends after its
analysis without a final answer.

### LM-41 A high thinking level can stop tool calls (P1)

LOOP-43. Measured against the real server: at low and medium the model
returned tool calls; at high with a generous budget, 2,568 characters of
prose and **no tool call**; at high with a small budget, **nothing**.

### LM-42 A model with no tools writes tool calls as text (P2)

With every tool turned off (a review run), Qwen wrote its tool calls out as
plain text and never gave the verdict it was asked for. A run with no tools
must say, in what it sends, that there are none.

### LM-43 A claim is not a result (P1)

The same models report work they did not do: a render "of 1,091,784 KB"
after three listings and no render; "verified by view" with no view;
`LOOP_DONE` to get out of a hard step. Everything that ends work on the
model's word (a goal, a loop, a close) asks for the evidence, and the
harness checks it where it can (HOOK-22, LOOP-22, `close-check.md`).

---

## 8. What is the server's business, and worth telling the user

gnosh cannot fix these, but its `doctor`, its docs and its messages should
help the user see them (P2). Each cost Jae hours.

| On the server | What it does to gnosh |
| :- | :- |
| **One slot** | Every side request evicts the main conversation. Two slots at least. |
| **Total context is split across slots** | Three slots of 256K need a total of 786K. |
| **The RAM prompt cache's size** | With it too small, a long conversation's saved state is discarded every turn and re-read cold (eight minutes for a one-word reply at 109K tokens). An entry costs about 1 GB plus 55 KB per token on this model. |
| **A low slot-similarity threshold** | Lets a new conversation that merely begins like an old one take the old one's warm slot instead of an idle one. |
| **A vision projector loaded for a coding model** | Makes the server read media markers out of plain text (SAFE-20). |
| **A crash in the prompt-cache save** | On this box, a GPU allocation limit crashed the server about hourly with several slots and the cache on; every crash wiped all slots. A setting fixed it. |
| **The server's temporary folder being in RAM** | A model file moved through it took the memory the server needed, and the machine killed the server and Jae's session. |
| **Other programs sharing the server** | Three other clients starved his request for ten minutes (LM-1). |

---

## 9. Hosted providers (P1)

Jae's daily use is local, but "any model, hosted or on your own machine" is
a public promise.

- `anthropic` and `openai` types with keys from config or the usual
  environment variables (CFG-52).
- A request to Anthropic never carries more cache marks than Anthropic
  accepts.
- Ten-minute response-start limit; `API_TIMEOUT_MS` (LM-1).
- Compaction near the window as CC does it, where frames are not in use.

Which other hosted providers gnosh knows out of the box is OQ-3.

---

## 10. Measuring (P2)

The comparisons of 2026-11-06/07 used a small proxy between the harness and
the model server that pins the model, answers the model list itself with a
fixed window, forwards only chat requests, and logs for each request the
tokens read, the tokens read from cache, the tokens written, and whether it
was a title or a summary. With four tasks that have checks, it gives one row
per run: passed, time, requests, tokens, GPU time, peak context. The new
gnosh should be measured the same way, on the same tasks, before any claim
about speed is made. What those runs found is in `harness-open-questions.md`
(OQ-20) and in LOOP-2.
