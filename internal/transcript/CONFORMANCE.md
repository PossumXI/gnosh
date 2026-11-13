# How `internal/transcript` conforms to `spec/gnosh/frames.md`

This file lists every rule, edge case and worked example of the frame spec
beside the code that implements it and the test that proves it. It was
written in an audit of the package against the final spec (13 November
2026), after the package was first built from a spec that was still being
finished.

How to read the tables:

- **Spec** is a section of `spec/gnosh/frames.md` unless it says otherwise
  (IC: `integration-contract.md`; OQ: `open-questions.md`; LOOP:
  `harness-agent-loop.md`; DECISIONS: `spec/DECISIONS.md`).
- **Code** is a function or field of this package. Where the duty is the
  loop's join, the loop's or mem's, it says so and names the fact this
  package supplies for it.
- **Test** is the test that proves the row. The tests named `TestSpec…` are
  written from the spec's words, one section each, in `frames_spec_test.go`,
  `slide_spec_test.go`, `join_spec_test.go` and `property_spec_test.go`.
  `TestSpecOracle` (`oracle_spec_test.go`) is the rule a second time, as
  literally as the spec states it; every view any test computes is compared
  with it.
- **Before** says whether the first build's tests held the row: `yes`,
  `part` (a test touched it but not this clause), `no`, or `-` where the
  row is not this package's to test.

## 1. Rulings on the first build's four departures

1. **A push whose reply cannot be found cuts from the push entry, not from
   the summary or the first row.** Kept. Section 4.1, step 5 starts such a
   range "after the last summary row before the pop, or at the first row
   when there is none". That is the fallback of a store that knew a push
   only by its call: with the call's row gone, the frame's start was
   unknown. Here a push is an entry with a place of its own (DECISIONS 13),
   so the start is known, and the rule itself can be kept: "a pop removes
   ... every row after the frame's push round and before the pop's round".
   Read to the letter, step 5 would remove what came before the frame was
   entered, which section 1 says the model is sent ("the pushes and pops of
   the steps already finished"), and with it the results of every earlier
   step. The rest of step 5 is followed: removal begins at the first
   assistant row from there, a summary between is handled as in step 4, and
   the range carries no note. Test:
   `TestSpec4_1_Step5_ThePushIsNotAmongTheRows`.
2. **Slides are recorded by entry uuid, not by call id.** Kept. The spec's
   stored marker (8.6) names a tool call of each of the two replies that
   bound the cut, "so 'a record derived again from the rows cuts the
   same'". A slide entry names the two replies themselves. That meets the
   requirement (`TestSpec8_6_ARecordedSlideCutsTheSameInEveryLaterRequest`:
   in later requests, after a restart, in a copy), it is what DECISIONS 13
   asks for ("appending an entry that names the entries it marks"), and an
   entry's uuid cannot repeat where a server's call ids can. The spec's
   marker is not lost: `Cut.PushCall` and `Cut.PopCall` are the two calls it
   names and `Cut.Mark()` writes it
   (`TestSpec11_TheStoredMarker`).
3. **The note for a slide that removed tool calls leaves out some of the
   spec's wording.** Conformed. It now reads as the pop's line with the
   slide's ending: "[Then came the work in it, 2 tool calls (Read 2),
   removed from your context to make room.]". With no tool call it reads as
   the spec spells it: "[Then came 2 messages, removed from your context to
   make room.]". Test: `TestSpec8_6_TheSlidesNote`.
4. **A pop takes the frames above it off the stack too.** Conformed to the
   spec: "a pop removes the topmost frame with that id ... The stack is
   'the pushes without a matching pop, in order'" (3.3). mem pops only the
   frame the session stands in, so the two readings differ only for a
   transcript mem did not write; there the frames above stay, and
   `Cut.Above` names them so that the join can say so. Test:
   `TestSpec3_3_APopRemovesTheTopmostFrameWithThatID`.

The first build's claim that **a pop can be previewed by viewing the
entries with a pop appended** holds only for an entry that carries the
session's id, which an entry not yet appended does not: without it the pop
belongs to no session's stack and changes nothing. `Preview` and
`Session.Preview` stamp it and return the cut. What they return is, field
for field, what `View` reports once the entries are stored, whether the
preview is made after the round's results are stored or with only the reply
that made the call (`TestPreview`, and `TestSpecProperties` over random
sessions: 1,298 previews compared, 834 of them made before the result).

**`Stack` against section 3 for entries stamped with another session's
id:** conforms. Such entries (what a branch inherits) keep their cuts and
put nothing on this session's stack; a pop this session makes of the other
session's frame is a pop of a frame not on the stack and records nothing;
this session's own pushes count (`TestSpec3_4_OnlyPushesUnderTheSessionsOwnIDCount`,
`TestSpec12_5_ABranchCarriesTheCutsAndStartsAtTheRoot`).

## 2. Where the code was wrong, and what changed

| # | What was wrong | Spec | Fix |
|---|---|---|---|
| W1 | A pop took every frame above the popped one off the stack. | 3.3 | `derive` takes off the one frame. |
| W2 | A slide's note with tool calls read "[Then came N tool calls …" without "the work in it,". | 8.6 | `noteText`. |
| W3 | A slide's note for a frame's start went on the answer to the first call of the push's reply, not on the frame's own push answer, so a later pop's note stood beside it instead of taking its place. | 8.6, 4.4 | `bound` uses the frame's push call. |
| W4 | A reply that was cancelled or failed before anything of it arrived is not a row, and so did not end its turn: the next prompt was taken as folded into the turn, and a pop spared the cancelled turn's prompt. | 4.3 | `conv.halts`: such a reply still ends its turn. |
| W5 | The spared count included prompts of the turn that a slide had already taken (the spec's own "known and left" defect). | 14 | `apply` counts what the model is left with. |
| W6 | With the push's reply behind the summary and its results after it, the rows that continue the summary were cut. | 4.1.4 | `cut.behind` goes by the reply, not by the first row of the range. |
| W7 | Across a summary, the fact that the last reply before it was cancelled was forgotten. | 4.3 | `conv.endedBehind`. |
| W8 | A recorded slide of a frame id that stands twice on a stack was tied to the wrong one of the two (mem never writes that). | 8.6 | `topmost` goes by the anchor. |
| W9 | Nothing gave the join a pop's facts before the pop was stored (see the claim above), nor the replies and tool calls a cut removed, nor a frame's token total at its push. | 5, 6, 8.7, 9, 10 | `Preview`, `Cut.Replies`, `Frame.TotalAtPush`, and the fields below. |

## 3. Passages where the spec is unclear or contradicts itself

| # | Passage | The reading taken |
|---|---|---|
| U1 | 4.1 step 5 against the rule of 4.1 and section 1. | Ruling 1 above. |
| U2 | 8.6: the slide's note with tool calls is given only as "`…, removed from your context to make room.]`". | The elided part is the pop's line up to that comma: "[Then came the work in it, N tool calls (…)". |
| U3 | 4.4 gives "[Then came N messages of work in it, …]" and no form for one message. | "1 message", as "1 tool call". |
| U4 | 4.4: "the tools are listed by name, sorted"; the example has the earlier harness's lower-case names. | The names are the tools' own (`Bash`, `Edit`, `Read`, `gnovm_run`), sorted as text. |
| U5 | 13.2, the list "before": the answer that entered `fix-flaky/reproduce` carries no note, though that frame is popped and the note "is said in every request after the pop" (4.4). | 4.4: the note is there. The example is the design's and older than the note. |
| U6 | 13.2, the list "after": no prompt of the later turn is shown, though a pop "spares the prompt rows of the turn it is made in" (4.2). | 4.2: the later turn's prompt stays, after the push's answer. The example is older than the sparing; its last sentence says as much. |
| U7 | 12.5 and 14.10: after a rewind into a popped range "the stack is derived again from what remains". In the earlier store the marker stayed on the rows that remained; the spec does not say whether the model is sent them again. | A rewind is an entry that takes what came after its target off the conversation, the pop with it (DECISIONS 13: nothing is rewritten). So the frame is open and its rows are its work again, exactly as at that point. The next pop cuts them as any other (12.6). |
| U8 | 4.3 speaks of "the nearest assistant row"; section 2 calls an assistant row "one model reply: its text, reasoning, and zero or more tool calls". A reply with none of these (the model said nothing, or was stopped first) is not covered. | It is not a row: the model is never sent it, and it is no "answer to the turn before". If it was cancelled or failed it still ends its turn (W4). |
| U9 | 5 and 6 say "the removed rows" for the files edited, the changes to mem and the reads to forget. 4.4 counts rows an inner pop had removed; 10 does not. | `Cut.Replies` lists every reply between the two rounds and marks the ones that were gone already (`Reply.Gone`), so the join can take either. The note's reading (all of them) is the natural one for "In it you edited …". |
| U10 | 8.6: the marker's pop call is "the first call of the row the cut ends before (the next frame's push call for a whole body)". Whether the rounds before the first frame, cut up to the bottom frame's push, name that frame's push call is not said. | Only a whole body does; the rounds before the first frame name the first call. |
| U11 | 8.2 frees "a tenth of the window besides"; `View` with a `Budget` knows no window. | The loop gives `Slide` the need. `View`'s own planning, for callers with no token counts, adds `Room`, by default a tenth of the budget. |
| U12 | 3.1's event carries `Thoughts`; 3.2 says the events are persisted with the row. | A frame entry holds what the stack and the cut are derived from (kind, id, title, result, call, session). The count of thoughts is used once, in the pop's result, and is the join's to pass on. |
| U13 | 11: the flag is "set by explicit row ids ... and each change is published". | Nothing is set and nothing changes: the pop or slide entry is the one new thing, and `Cut.Removed` names the entries it marks. |
| U14 | 14, "known and left": the spared count "can disagree with the list". | Not carried over (W5). |

## 4. The table

### Section 2: the words

| Id | Spec | The rule | Code | Test | Before |
|---|---|---|---|---|---|
| 2.a | 2 | Four kinds of row are told apart: prompt, assistant, result, summary. | `newConv`: `rowPrompt`, `rowAssistant`, `rowResult`; `conv.summary`, `conv.start` | `TestSpecOracle`; `TestSummaries` | yes |
| 2.b | 2 | Results and text in one user message are separate rows. | `splitResults` | `TestSpec2_ResultsAndWordsInOneMessageAreSeparateRows` | yes |
| 2.c | 2 | A prompt row is typed, folded in, or hidden. | `row.hidden` (the entry's source is not `prompt`) | `TestSpec4_3_WhereATurnStarts` | yes |
| 2.d | 2 | A round is a reply with a tool call and the result rows that answer it; "a cut between rounds never splits a call from its result". | `lastAnswer`, `answers`, `round` | `TestSpec4_1_ThePushRoundAndThePopRoundStayWhole`; `TestSpecProperties` (repairs) | part |
| 2.e | 2 | The model-facing list and the human-facing list. | `View`; `Conversation` | `TestSpec11_ModelFacingReadersSkipHumanFacingReadersKeep` | no |

### Section 3: the stack is derived

| Id | Spec | The rule | Code | Test | Before |
|---|---|---|---|---|---|
| 3.a | 3 | No stack store: the stack is derived from the session's own rows. | `derive`; `Stack` | `TestSpec3_3_TheStackIsThePushesWithoutAMatchingPop` | yes |
| 3.b | 3.1 | An event is a push or a pop with the frame's id, its title, a pop's result and the session. | `PushEntry`, `PopEntry`; `Entry.SessionID` is the stamp | `TestDocExamples`; `TestSpec4_6_OtherSpellingsThatPop` | part |
| 3.c | 3.1 | Pop, Done or Drop of the present frame, Leave, and taking a closed frame off the stack all make a pop event; Start makes none. | mem; a pop entry is a pop however it was spelled | `TestSpec4_6_OtherSpellingsThatPop`; `TestSpec12_6_AFrameMemAlreadyHoldsClosed` | no |
| 3.d | 3.2 | Events are kept with the result row, persisted and copied with it; a refused input records none. | the join appends the frame entries with the result (`doc.go`); `Store.Branch` and `inherit` carry them | `TestSpec12_4_AResumedSessionStandsWhereItStood`; `TestSpec12_5_ABranchCarriesTheCutsAndStartsAtTheRoot`; `TestSpec4_8_ACallThatWasNotKeptCutsNothing` | part |
| 3.e | 3.3 | A push appends a frame, with the id of the call that pushed it. | `derive` (`OpPush`); `Frame.ToolUseID` | `TestSpec3_3_TheStackIsThePushesWithoutAMatchingPop` | yes |
| 3.f | 3.3 | A pop removes the topmost frame with that id. | `derive` (`OpPop`) | `TestSpec3_3_APopRemovesTheTopmostFrameWithThatID` | no |
| 3.g | 3.3 | It records a range: frame id, push call id, pop call id. | `Cut.Frame`, `Cut.PushCall`, `Cut.PopCall`; `Cut.PushReply`, `Cut.PopReply` | `TestSpec3_3_ARangeIsAFrameIDAPushCallIDAndAPopCallID` | no |
| 3.h | 3.3 | The stack is "the pushes without a matching pop, in order". | `derive` | `TestSpec3_3_TheStackIsThePushesWithoutAMatchingPop`; `TestSpec3_3_APopRemovesTheTopmostFrameWithThatID` | part |
| 3.i | 3.3, OQ D | A pop naming a frame that is not on the stack records nothing. | `derived.unmatched`; `ViewInfo.UnmatchedPops` | `TestSpec3_3_APopNamingAFrameNotOnTheStackRecordsNothing` | yes |
| 3.j | 3.3 | Each slide mark puts its range and its frame back. | slide entries; `slideCut` | `TestSpec8_6_ARecordedSlideCutsTheSameInEveryLaterRequest`; `TestSpec8_6_TheSlidListFollowsTheStack` | yes |
| 3.k | 3.3 | Each range gets its spared count and its note, worked out from the rows. | `apply` | `TestSpec4_2_APopSparesThePromptsOfItsOwnTurn`; `TestSpec4_4_TheNote` | yes |
| 3.l | 3.4 | Only events stamped with the session's own id count: a branch and a rewind copy start at the root. | `derive` (a stack for each `SessionID`); `ViewOptions.Session`; `Session.View`, `Session.Stack` | `TestSpec3_4_OnlyPushesUnderTheSessionsOwnIDCount` | yes |
| 3.m | 3.5 | Derive once and cache; derive again when rows are removed (a rewind). | no cache: `View` and `Stack` are pure functions of the entries | `TestSpec12_5_ARewind`; `mustView` (the same answer twice) | yes |
| 3.n | 3.6 | The stack is told to mem before every call, bottom to top. | `Stack`, `Session.Stack` | `TestSpec3_3_TheStackIsThePushesWithoutAMatchingPop`; `TestSpec12_3_ASubAgentHasAStackOfItsOwn` | yes |
| 3.o | 3 | The stack persists across turns and restarts; a frame is never popped by a turn's end. | everything is derived from the file | `TestSpec3_TheStackPersistsAcrossTurnsAndRestarts` | no |
| 3.p | 3 | Only a session's own pops change its stack. | mem; stacks by session id | `TestSpec3_4_OnlyPushesUnderTheSessionsOwnIDCount`; `TestPreview` | part |

### Section 4: the cut

| Id | Spec | The rule | Code | Test | Before |
|---|---|---|---|---|---|
| 4.a | 4.1 | A pop removes every row after the push round and before the pop round. | `popCut` | `TestSpec4_1_ThePushRoundAndThePopRoundStayWhole` | yes |
| 4.b | 4.1, 14.7 | The push round stays whole: its other calls and their results. | `lastAnswer` | same; `TestFrameNoteIsAddedToThePushAnswerOnly` | yes |
| 4.c | 4.1 | The pop round stays whole: the text beside the call, the reply's other calls, their results. | `popCut` (the range ends before the pop's reply) | same | part |
| 4.d | 4.1 | Several tool calls in one reply, each result an entry of its own. | `lastAnswer`, `answerRow` | same; `TestSpec4_1_Step3_TheRangeStartsAfterTheLastResultOfThePushRound` | no |
| 4.e | 4.1 | Between them go rounds, replies, questions, reasoning, the words of earlier turns, notices and reminders. | `apply` | `TestSpec14_4_WordsInsideAFrameAcrossTurns` | yes |
| 4.f | 4.1 | A reply that was cancelled or failed inside the frame goes with the rest, and its unanswered calls with it. | `apply` | `TestSpec4_1_ACancelledOrFailedReplyInsideTheFrameGoesWithTheRest` | no |
| 4.g | 4.1 | Nothing is rewritten and nothing synthetic is inserted but the note. | `withNotes`, `pieces` | `TestSpec4_1_NothingIsRewrittenAndNothingSyntheticIsInserted` | part |
| 4.h | 4.1.1 | The pop call's row is not among the rows: nothing is removed. | `popCut` | `TestSpec4_1_Step1_ThePopCallIsNotAmongTheRows` | yes |
| 4.i | 4.1.2 | Pushed and popped in one reply: nothing is removed. | `popCut` | `TestSpec4_1_Step2_PushedAndPoppedInOneInput` | yes |
| 4.j | 4.1.3 | The range starts after the last result row that answers any call of the push's reply, and ends before the pop's reply. | `popCut`, `lastAnswer` | `TestSpec4_1_Step3_TheRangeStartsAfterTheLastResultOfThePushRound` | part |
| 4.k | 4.1.4, 14.6 | A summary inside the range is never removed; the rows that continue it stay; removal resumes at the first assistant row. | `stays`, `conv.cont`, `cut.behind` | `TestSpec4_1_Step4_ASummaryInsideTheRange` | yes |
| 4.l | 4.1.4 | The rows between that push and the summary are flagged with the rest, free nothing and are not counted. | `apply` (`present`) | same; `TestSpec12_1_SummariesAndTheStack` | yes |
| 4.m | 4.1.5 | The push is not known or not among the rows: removal begins at the first assistant row, and the range carries no note (ruling 1 for where it starts). | `popCut` | `TestSpec4_1_Step5_ThePushIsNotAmongTheRows` | yes |
| 4.n | 4.1 | A pop whose push round another cut has removed is still read off the rows. | `popCut` (rows, not what is left of them) | `TestSpec3_3_APopRemovesTheTopmostFrameWithThatID` (third case) | no |
| 4.o | 4.2, OQ Q2 | A pop spares the prompt rows of its own turn: the prompt, prompts folded in, hidden continuations and reminders. Earlier turns' words are cut. | `stays`, `cut.turn` | `TestSpec4_2_APopSparesThePromptsOfItsOwnTurn` | yes |
| 4.p | 4.3 | Scan back for a visible prompt row; take the whole run of prompt rows that ends at it. | `turnStart` | `TestSpec4_3_WhereATurnStarts` ("the run takes in the hidden rows around the typed prompt") | part |
| 4.q | 4.3 | The run starts a turn when nothing precedes it. | `turnStart` | same ("nothing precedes the run") | yes |
| 4.r | 4.3 | … or a summary row. | `turnStart` | same ("a summary row precedes the run") | no |
| 4.s | 4.3 | … or an assistant row with no tool call. | `turnStart` | same ("an answer precedes the run") | yes |
| 4.t | 4.3, 14.17 | … or anything else, when the nearest assistant row before it was cancelled; result rows may lie between. | `turnStart`, `row.ended` | same ("cancelled, with results between") | yes |
| 4.u | 4.3 | … or failed. | same | same ("the reply before the run failed") | no |
| 4.v | 4.3, U8 | A reply cancelled or failed before it held anything ends its turn too; an empty reply that was not cut off does not. | `conv.halts`, `conv.endedBehind` | same (three cases) | no |
| 4.w | 4.3 | Otherwise the prompt was folded into a running turn: keep scanning back. | `turnStart` | same ("a prompt that follows tool results", "the scan goes back") | yes |
| 4.x | 4.3 | No such run: the turn starts at the first row. | `turnStart` | same ("no prompt the person typed at all") | no |
| 4.y | 4.3 | The spared rows are every prompt row of the range at or after the turn's start; a hidden continuation after an answer starts no turn. | `stays` | same ("a hidden continuation", "an earlier turn's hidden rows") | yes |
| 4.z | 4.4 | The line, appended to the text of the push's result. | `noteText`, `appendLine`, `withNotes` | `TestSpec4_4_TheNote` | yes |
| 4.aa | 4.4 | It counts the tool calls of the removed replies, by name, sorted; "1 tool call". | `noteText` | same (two cases) | yes |
| 4.ab | 4.4 | Rows an inner pop had removed are counted with the rest. | `apply` (`between`) | same ("an inner pop") | yes |
| 4.ac | 4.4 | Rows a slide had removed are counted with the rest. | same | same ("a slide") | yes |
| 4.ad | 4.4 | Only the spared prompt rows are left out of the count. | `apply` | same ("it counts messages") | part |
| 4.ae | 4.4 | No tool call: "[Then came N messages of work in it, …]". | `noteText` | same | yes |
| 4.af | 4.4 | Every row of the range was spared: no note. | `apply` | same ("every row of the range was spared") | no |
| 4.ag | 4.4 | The push is not among the rows: no note. | `popCut` (`noteRow`) | `TestSpec4_1_Step5_ThePushIsNotAmongTheRows` | yes |
| 4.ah | 4.4, IC 6.2.6 | It is worked out from the session's rows, never from the list a request sends. | `apply`, over the entries | `mustView` (the same twice); `TestSpecProperties` (a reload) | part |
| 4.ai | 4.4, 14.18 | It is said in every request after the pop: the pop's turn, later turns, a summary's request. | `View` is a function of the entries alone | `TestSpec4_4_TheNoteIsSaidInEveryRequestAfterThePop` | no |
| 4.aj | 4.4 | It changes only the model-facing copy of the result. | `withNotes` | `TestSpec4_4_TheNote` ("the stored row is not edited") | yes |
| 4.ak | 4.4 | One answer, one note: a pop's wins over a slide's. | `addNote` | `TestSpec4_4_OneAnswerCarriesOneNote` | yes |
| 4.al | 4.4 | Of two of a kind, the one whose range reaches furthest. | `addNote` | same (two cases) | no |
| 4.am | 4.5 | Frames nest: an inner pop cuts at once, the outer one takes what is left; the list is cut by the union of the ranges. | `apply`, `conv.removed` | `TestSpec4_5_NestedFramesAreCutByTheUnionOfTheirRanges` | yes |
| 4.an | 4.6, 14.9 | Done and Drop of the present frame, and Leave, pop and cut. | a pop entry | `TestSpec4_6_OtherSpellingsThatPop` | no |
| 4.ao | 4.6 | Done of a todo not on the stack, or of the bound root, cuts nothing. | mem makes no event | `TestSpec4_8_ACallThatWasNotKeptCutsNothing` | no |
| 4.ap | 4.7 | Reopen and undo do not bring rows back. | nothing un-cuts; a frame pushed again is a new frame | `TestSpec4_7_ReopenAndUndoDoNotBringRowsBack` | yes |
| 4.aq | 4.8 | A refused or failed input cuts nothing. | no entry, no cut | `TestSpec4_8_ACallThatWasNotKeptCutsNothing` | no |

### Sections 5 to 7: what the pop tells the model, what is forgotten, the frame line

These are the join's duties. The table names the fact each needs.

| Id | Spec | The rule | Code | Test | Before |
|---|---|---|---|---|---|
| 5.a | 5, IC 5.3.4 | The harness's clauses go into the pop's result, so they are needed before the result is stored. | `Preview`, `Session.Preview` | `TestSpec5_ThePopsResultIsWrittenFromThePreview`; `TestPreview`; `TestSpecProperties` (previews) | no |
| 5.b | 5, 14.13 | "Everything since you entered … was removed", or "nothing came between". | `Cut.Between` | `TestSpec5_TheOtherClausesHaveTheirFacts`; `TestSpec14_13_NoRoundBetweenPushAndPop` | part |
| 5.c | 5 | ", but for what the user said in this turn". | `Cut.Spared`, `Cut.SparedEntries` | `TestSpec5_TheOtherClausesHaveTheirFacts` | yes |
| 5.d | 5 | The thoughts clause. | mem's event (the join) | - | - |
| 5.e | 5 | Files edited: the successful calls of the file-changing tools in the removed rows. | `Cut.Replies`: `Call.Name`, `Call.Path`, `Call.Answered`, `Call.IsError` | `TestSpec5_ThePopsResultIsWrittenFromThePreview`; `TestSpec13_3_ARealPop` | no |
| 5.f | 5 | Other changes to mem: the calls on mem that were not refused, by their code. | `Call.Input`, `Call.IsError`, `Call.Content` | `TestSpec5_ThePopsResultIsWrittenFromThePreview` | no |
| 5.g | 5 | One line for each pop when an input pops more than once. | `Preview` returns one `Cut` for each pop, in order | `TestSpec5_TheOtherClausesHaveTheirFacts` | no |
| 5.h | 5, 15 | "removed from your context", never "left"; no token number for the model. | `noteText` | `TestSpec4_4_TheNote` | yes |
| 6.a | 6.1, 14.11 | The reads of every path the removed rows touched are forgotten: `file_path` arguments, and the file a shell command showed. | `Call.Path`; `Call.Input` holds the command for the harness's parser | `TestSpec5_ThePopsResultIsWrittenFromThePreview` | no |
| 6.b | 6.2 | The views in the removed rows are forgotten. | the same facts | same | no |
| 6.c | 6.3 | Context files given in the removed rows are forgotten. | `Cut.Removed`; `Call.ResultUUID` | same | part |
| 7.a | 7 | The frame line: the frame, its title, the stack when it holds more than one. | `Stack`: `Frame.ID`, `Frame.Label` | `TestSpec7_TheFrameLineHasItsFacts` | part |
| 7.b | 7 | Computed as the prompt goes out. | `Stack` of the entries at that moment | same | part |
| 7.c | 7, DECISIONS 13 | Stored with the prompt as a hidden part; it goes or stays with its prompt. | the join writes it as a user entry after the prompt; `turnStart` takes the run | `TestSpec4_3_WhereATurnStarts` ("an earlier turn's hidden rows go with it") | part |

### Section 8: the slide

| Id | Spec | The rule | Code | Test | Before |
|---|---|---|---|---|---|
| 8.a | 8.1, LOOP-50 | Standing in frames the session slides instead of summarizing; no summary, no model call; compaction only with no frame to cut. | `Slide`, `planSlides` | `TestSpec8_1_OnlyASessionInFramesSlides` | yes |
| 8.b | 8.2 | When it is tried, and the need: what is over the point and a tenth of the window. | the loop gives `Slide` the need | - | - |
| 8.c | 8.2 | If nothing can slide, compaction decides. | `Slide` returns nothing | `TestSpec8_1_OnlyASessionInFramesSlides`; `TestSpec8_4_HowMuchAPartialCutTakes` | yes |
| 8.d | 8.3 | Only rows the model still has are counted or cut. | `present`, `span` | `TestSpec8_3_OnlyRowsTheModelStillHasAreCountedOrCut` | part |
| 8.e | 8.3a | First the rounds before the first frame; the region's first round stays; the cut ends before a round, the bottom frame's push at the latest. | `planSlides` (a), `firstRound`, `partial` | `TestSpec8_3a_TheRoundsBeforeTheFirstFrameGoFirst` | yes |
| 8.f | 8.3a | The prompt lies in that region: the turns before it first, then the turn's own rounds. | `planSlides` (a) | `TestSpec8_3a_ThePromptLiesBeforeTheFirstFrame` | yes |
| 8.g | 8.3b | Then each frame with a frame above it: its whole body, to the next frame's push; a body with nothing left is passed over. | `planSlides` (b) | `TestSpec8_3b_TheLowerFramesWholeBodyThenTheTopFramesOldestRounds` | yes |
| 8.h | 8.3b | The prompt lies inside that body: only the oldest rounds, up to a row before the prompt. | `planSlides` (b) | `TestSpec8_3b_ThePromptLiesInsideTheLowerFramesBody` | no |
| 8.i | 8.3c | Last the oldest rounds of the top frame; before the prompt when it lies in its body. | `planSlides` (c) | `TestSpec8_3c_TheTopFrameAndThePrompt` | yes |
| 8.j | 8.3 | The walk stops at the first frame whose push row is not among the rows; when that is the bottom frame, (a) is skipped. | `planSlides` (`push`) | `TestSpec8_3_TheWalkStopsAtAFrameWhosePushIsNotAmongTheRows` | part |
| 8.k | 8.4 | The shortest cut that frees max(need, body/2), else the longest; a cut that frees nothing is no cut. | `partial` | `TestSpec8_4_HowMuchAPartialCutTakes` | yes |
| 8.l | 8.5 | The prompt being answered is never cut: typed, folded in or hidden, held by id. | `ViewOptions.Prompt`, `promptRow` | `TestSpec8_3c_TheTopFrameAndThePrompt` (three kinds) | part |
| 8.m | 8.5 | Before a new prompt goes out nothing is protected. | `Prompt` empty | `TestSpec8_3a_ThePromptLiesBeforeTheFirstFrame` | yes |
| 8.n | 8.5 | Summary rows and what is behind them are never cut, nor the anchor round of a cut. | `planSlides`, `bound` | `TestSpec8_3_OnlyRowsTheModelStillHasAreCountedOrCut`; `TestViewProperties` | yes |
| 8.o | 8.5 | A slide spares no other prompt. | `cut.turn` is past the end | `TestSpec8_3c_TheTopFrameAndThePrompt` (last case) | yes |
| 8.p | 8.6 | A slide is recorded and applied as a pop's range; every later request leaves its rows out; derived again it cuts the same. | slide entries: `slideEntry`, `slideCut` | `TestSpec8_6_ARecordedSlideCutsTheSameInEveryLaterRequest` | yes |
| 8.q | 8.6, U2 | Its note with tool calls. | `noteText` | `TestSpec8_6_TheSlidesNote` | no |
| 8.r | 8.6 | Its note with none: "[Then came N messages, removed from your context to make room.]". | `noteText` | same | no |
| 8.s | 8.6 | The note is on the anchor's answer: the first call of that reply, or the frame's own push call. | `bound` | same ("on the frame's own push answer") | part |
| 8.t | 8.6, IC 6.4 | The mark `slide:<frame>:<push call id>:<pop call id>`. | `Cut.Mark`, `Cut.PushCall`, `Cut.PopCall` (ruling 2 for the record itself) | `TestSpec11_TheStoredMarker` | no |
| 8.u | 8.6 | The line the model is told: "The start of `x`", or "Your rounds before `x`" for the bottom frame. | the join; `Cut.Frame`, `Cut.Before` | `TestSpec7_TheFrameLineHasItsFacts`; `TestSpec8_3a_TheRoundsBeforeTheFirstFrameGoFirst` | no |
| 8.v | 8.6, 14.15 | The line is repeated only while its frame is on the stack; a pop takes its frame out of the list; an emptied stack clears it; a frame pushed again is a new one. | `derive` (`cut.of`, `cut.live`); `Cut.Live`, `Frame.Slid` | `TestSpec8_6_TheSlidListFollowsTheStack` | part |
| 8.w | 8.6 | The slid rows' reads are forgotten, their tokens go to the estimate, the person is told "slid `x`: about N tokens". | `Slide`'s cuts: `Replies`, `Tokens`, `Frame` | `TestSpec8_6_ASlidesCutHasTheFactsOfAPops` | part |
| 8.x | 8.7, IC 6.2 | The large-step line compares the session's total with its total when the frame was pushed. | `Frame.TotalAtPush` (the last usage entry before the push) | `TestSpec8_7_TheSessionsTotalWhenAFrameWasEntered` | no |
| 8.y | 8.8 | Measured: the exploration before the first push is what slides; pops do the rest; no summary. | - | `TestSpec8_8_ExplorationSlidesPopsDoTheRest` | no |

### Sections 9 and 10: the fresh guard's input, the token estimate

| Id | Spec | The rule | Code | Test | Before |
|---|---|---|---|---|---|
| 9.a | 9 | Asked only when the session stands in a frame and the code holds `Pop(` or `Done(`. | the join | - | - |
| 9.b | 9.1 | Not refused when the push call is not known, its row is not before this call's row, or this reply entered it. | a previewed pop's `Cut.PushCall`, `PushReply`, `PopReply` | `TestSpec9_AFrameWhosePushIsNotBeforeTheCall` | no |
| 9.c | 9.2 | Work is judged over the rows a pop made by this call would take, those inner pops removed included. | `Preview`: `Cut.Replies`, `Reply.Gone` | `TestSpec9_TheFreshGuardsRowsAreAPreviewedPops` | no |
| 9.d | 9.2, 14.16, OQ Q21 | What work is: a tool call that did not fail; a change to mem that was not refused; an answer, unless a close was refused before it. Not a prompt, a read of mem, a refused pop. | the join's rule, over `Call.Name`, `IsError`, `Answered`, `Input`, `Content` and `Reply.Said`, in order | same | no |
| 9.e | 9.3 | Work since the plan was made counts. | a previewed pop of the bottom frame | same | no |
| 10.a | 10.1 | The removed rows are estimated with the harness's estimator, counting only what the model still had. | `Cut.Tokens`; `ViewOptions.Estimate` | `TestSpec10_TheEstimateCountsOnlyWhatTheModelStillHad` | yes |
| 10.b | 10.2 to 10.4 | The total less the estimate; spent at the next usage report; the counts shown. | the loop | - | - |
| 10.c | 10 | The person is told "popped `x`: about N tokens out of the model's context". | the join; `Cut.Tokens`, `Cut.Frame` | `TestSpec5_ThePopsResultIsWrittenFromThePreview` | part |
| 10.d | 14.12 | A pop on the round that crosses the compaction point: the estimate is there before the next request. | `Preview` | `TestSpec14_12_APopOnTheRoundThatCrossesTheCompactionPoint` | no |

### Section 11: persisting the cut

| Id | Spec | The rule | Code | Test | Before |
|---|---|---|---|---|---|
| 11.a | 11, IC 6.4 | The marker of a row: empty, a frame's id, or a slide mark; set by row id; never on the summary; copied with the row. | the pop or slide entry is the record (DECISIONS 13); `Cut.Mark`, `Cut.Removed` | `TestSpec11_TheStoredMarker` | no |
| 11.b | 11 | A row's marker names the outermost frame that removed it. | the cut made last that holds the entry | same | no |
| 11.c | 11 | Model-facing readers skip flagged rows. | `View`; `ViewInfo.Sources` | `TestSpec11_ModelFacingReadersSkipHumanFacingReadersKeep` | yes |
| 11.d | 11 | Human-facing readers, and the goal judge, keep them. | `Conversation`, `Session.Entries` | same | no |
| 11.e | 11 | Every request re-applies the ranges: injected rows at their place, spared prompts kept, notes appended. | `View`, from the entries each time | `TestSpecProperties`; `TestSpecOracle` | yes |
| 11.f | 11 | The list stays valid for every provider; a spared prompt sits after the push's results in one user message, results first. | `repair`, `Check` | `TestSpec4_1_Step3_TheRangeStartsAfterTheLastResultOfThePushRound`; `mustView` | yes |
| 11.g | 11 | The note goes on the result that answers the push call itself, not on a sibling's. | `withNotes` | `TestSpec4_1_ThePushRoundAndThePopRoundStayWhole`; `TestSpec8_6_TheSlidesNote` | yes |
| 11.h | 11, LOOP-61 | A summary's request is cut the same way, with the same notes. | the same `View` | `TestSpec4_4_TheNoteIsSaidInEveryRequestAfterThePop` | no |
| 11.i | 11 | Flush first. | the join; `AppendAll` returns when the entries are written | - | - |
| 11.j | 11, 14.19 | Applied even on Esc, and for a kept call whose reply was lost. | the join's context (`AppendAll` takes none); a cancelled reply's pop counts | `TestSpec11_AppliedEvenOnEsc` | no |

### Section 12: how it meets the rest

| Id | Spec | The rule | Code | Test | Before |
|---|---|---|---|---|---|
| 12.a | 12.1 | The stack survives a summary; the next pop removes everything after it. | `derive` walks the frame entries behind the summary too | `TestSpec12_1_SummariesAndTheStack`; `TestSpec4_1_Step4_ASummaryInsideTheRange` | yes |
| 12.b | 12.1 | Rows at and before the summary are neither counted nor cut by a later pop or slide. | `present` | `TestSpec12_1_SummariesAndTheStack` | yes |
| 12.c | 12.2 | A push costs nothing on the server. | a push entry leaves the view as it was | `TestSpecProperties` (pushes) | no |
| 12.d | 12.2, 14.14 | A pop keeps the prefix through the push's call; the note is at the end of the push's answer. | `apply`, `appendLine` | `TestSpec12_2_TheRequestAfterAPopSharesThePrefixThroughThePushsCall`; `TestSpecProperties` | no |
| 12.e | 12.2 | Measured: after a 3K pop only the pop is new, and later requests bring a few hundred tokens each. | - | same | no |
| 12.f | 12.2 | A slide cuts early rounds; what lies before the cut holds. | `planSlides`, `bound` | `TestSpec12_2_ASlideKeepsThePrefixThroughItsAnchor`; `TestSpecProperties` | no |
| 12.g | 4.4, LOOP | What one request sent, the next sends again. | `View`, `repair` | `TestSpecProperties` (appended entries) | no |
| 12.h | 12.3 | A sub-agent is a session of its own with its own, empty, stack. | a session file of its own | `TestSpec12_3_ASubAgentHasAStackOfItsOwn` | no |
| 12.i | 12.4, 14.5 | A resumed session: the stack is derived again, flagged rows stay out. | everything is in the file | `TestSpec12_4_AResumedSessionStandsWhereItStood`; `TestSpecProperties` (a reload) | no |
| 12.j | 12.5, 14.10 | A branch starts at the root and carries the flags. | `Store.Branch`, `derive` | `TestSpec12_5_ABranchCarriesTheCutsAndStartsAtTheRoot` | part |
| 12.k | 12.5, U7 | A rewind: a point inside a popped range is still a point; the stack is derived from what remains. | `survivors`, `derive` | `TestSpec12_5_ARewind` | yes |
| 12.l | 12.6 | A frame mem already holds closed: its pop is applied like any other. | a pop entry | `TestSpec12_6_AFrameMemAlreadyHoldsClosed` | no |
| 12.m | 12.7 | Each session has its own stack and its own cut. | stacks by session id | `TestSpec3_4_OnlyPushesUnderTheSessionsOwnIDCount` | yes |
| 12.n | 12.9 | A stream must not print rows again when a pop flags them. | no row is ever rewritten: a pop is one new entry | `TestSpec11_ModelFacingReadersSkipHumanFacingReadersKeep` | yes |
| 12.o | 12.10 | No change to the removed rows; nothing is deleted. | the file only grows | `mustView` (its input is unchanged) | yes |

### Section 13: the worked examples

| Id | Spec | The example | Test | Before |
|---|---|---|---|---|
| 13.a | 13.1 | The walkthrough: the stack after each call; the popped todo's work leaves. | `TestSpec13_1_TheWalkthrough` | no |
| 13.b | 13.2, U5, U6 | The list before and after a pop; "8080" gone with the question; kept had it been said in the pop's turn. | `TestSpec13_2_TheListBeforeAndAfterAPop` | no |
| 13.c | 13.3 | A real pop: what was edited, what was read, no thought, nothing spared. | `TestSpec13_3_ARealPop` | no |
| 13.d | 13.4 | Eight pops freed about 11.6K tokens; the last request about 17K where about 29K would have piled up. | `TestSpec13_4_EightPops` | no |

### Section 14: the spec's own list of edge cases

| Id | Spec | The case | Test | Before |
|---|---|---|---|---|
| 14.1 | 14 | Trampoline: three todos entered and popped in turn; the last Next a kept no-op. | `TestSpec14_1_TheTrampolineAndTheTask` | part |
| 14.2 | 14 | Nested: the task popped after its todos; its push and its pop remain. | same | yes |
| 14.3 | 14 | A side quest. | `TestSpec14_3_ASideQuest` | no |
| 14.4 | 14 | Words inside a frame across turns. | `TestSpec14_4_WordsInsideAFrameAcrossTurns` | part |
| 14.5 | 14 | Resume and restart inside a frame. | `TestSpec3_TheStackPersistsAcrossTurnsAndRestarts`; `TestSpec12_4_AResumedSessionStandsWhereItStood` | no |
| 14.6 | 14 | Compaction inside a frame. | `TestSpec4_1_Step4_ASummaryInsideTheRange` | yes |
| 14.7 | 14 | A parallel sibling of the push. | `TestSpec4_1_ThePushRoundAndThePopRoundStayWhole` | yes |
| 14.8 | 14 | Pop then Next in one input: two events, in order; a refusal discards both. | `TestSpec3_3_TheStackIsThePushesWithoutAMatchingPop`; `TestSpec4_8_ACallThatWasNotKeptCutsNothing` | yes |
| 14.9 | 14 | Done and Drop pop and cut; Leave cuts and leaves the todo open. | `TestSpec4_6_OtherSpellingsThatPop` | no |
| 14.10 | 14 | Branch and rewind copies; a rewind into a popped range. | `TestSpec12_5_ABranchCarriesTheCutsAndStartsAtTheRoot`; `TestSpec12_5_ARewind` | yes |
| 14.11 | 14 | Reads and views inside a popped frame (the facts; the refusal is the tools'). | `TestSpec5_ThePopsResultIsWrittenFromThePreview` | no |
| 14.12 | 14 | A pop on the round that crosses the compaction point. | `TestSpec14_12_APopOnTheRoundThatCrossesTheCompactionPoint` | no |
| 14.13 | 14 | No round between push and pop. | `TestSpec14_13_NoRoundBetweenPushAndPop` | yes |
| 14.14 | 14 | Prefix cache. | `TestSpec12_2_TheRequestAfterAPopSharesThePrefixThroughThePushsCall` | no |
| 14.15 | 14 | The slide: instead of compaction; lower frame first; before the prompt; before a hidden prompt; the rounds before the first frame first; rows behind a summary; a frame popped and pushed again is not slid. ("When compaction gave up" is the loop's.) | the `TestSpec8_…` tests | part |
| 14.16 | 14 | The fresh guard (the facts). | `TestSpec9_TheFreshGuardsRowsAreAPreviewedPops` | no |
| 14.17 | 14 | The pop spares this turn's prompts; a cancelled turn ends where it stopped. | `TestSpec4_2_APopSparesThePromptsOfItsOwnTurn`; `TestSpec4_3_WhereATurnStarts` | yes |
| 14.18 | 14 | The note is the same in the pop's turn, later turns and a summary. | `TestSpec4_4_TheNoteIsSaidInEveryRequestAfterThePop` | no |
| 14.19 | 14 | Frame events survive Esc and a lost reply. | `TestSpec11_AppliedEvenOnEsc` | no |
| 14.k1 | 14, known | A spared count can disagree with the list. | not carried over: `TestSpec14_Known_ASparedCountAgreesWithTheList` | no |
| 14.k2 | 14, known | A spared prompt's frame line still names the popped frame. | left: the line is an entry, and no entry is rewritten | - |
| 14.k3 | 14, known; OQ Q2 | The sparing covers only the pop's own turn. | as landed: `TestSpec13_2_TheListBeforeAndAfterAPop` | yes |
| 14.k4 | 14, known; OQ Q3, LOOP-53 | The 32K overflow. | the loop's | - |
| 14.k5 | 14, known; OQ Q20 | Hidden rows in the middle of a turn, and the prefix cache. | as landed: they are user entries | - |
| 14.k6 | 14, known | Not built: folding popped rows in the chat, self-trimming, compacting only the present frame. | not built | - |
| 15.a | 15 | Rejected and still rejected: rewriting the push's result with the pop's result; a stack kept as a store; popping at a turn's end; a token figure for the model. | none of them is done: `TestSpec4_1_NothingIsRewrittenAndNothingSyntheticIsInserted`; `TestSpec3_TheStackPersistsAcrossTurnsAndRestarts` | part |

## 5. The properties

`TestSpecProperties` and `TestViewProperties` hold these over random
sessions with pushes, pops, slides, summaries, rewinds, branches, cancelled
and failed replies, empty replies, several tool calls a reply, results in
entries of their own, and the odd thing gone wrong; `FuzzView` holds the
first, the second and the last over arbitrary bytes.

| Property | In the spec's words | Where |
|---|---|---|
| `Check(View(…))` holds. | "The list stays valid for every provider" (11). | `mustView` |
| The view is deterministic. | "worked out once, from the session's rows" (4.4). | `mustView` |
| The view is the one the spec's words give. | Sections 2 to 4, 8.6, 11. | `TestSpecOracle`, `mustView`, `FuzzView` |
| An entry written at the end changes no message of the view but the last. | "what one request sent, the next sends again" (4.4). | `checkStep` (21,690 entries) |
| A push changes nothing. | "A push costs nothing on the server" (12.2). | `checkStep` |
| After a pop, every block before the cut point is as it was, and the push's answer is what was stored with one line at its end. | "the prefix is intact through the push"; "The note is appended at the end of the push's answer" (12.2); "the request after a pop shares the prefix through the push's call" (14.14). | `checkStep` (930 pops, 368 slides, 747 notes) |
| A prompt of the turn a pop is made in is never removed by it. | "A pop spares the prompt rows of the turn it is made in" (4.2). | `checkStep` |
| A tool call is never separated from its result. | "a call and its result are never on opposite sides of a cut" (4.1). | `TestSpecProperties` (the repairs a view needs are those it needs with nothing cut) |
| Nothing in the newest round, which is the open top frame's latest, is ever removed. | 8.3c: the cut ends "before a later assistant row that has a tool call"; 4.1: the pop round stays whole. | `TestSpecProperties`; `TestViewProperties` |
| No entry is needed that is not in the file. | "The stack is derived again from the rows" (12.4). | `TestSpecProperties` (a reload gives the same view, stack and cuts) |
| What `Preview` says of a pop or slide is what `View` says once it is stored. | IC 6.2.6: "Work out the range's note once and keep it with the range." | `checkStep` |

## 6. What is not this package's

The loop (`internal/agent`) decides when a slide is tried and how much it
must free (8.2), holds the pending estimate against the provider's counts
(10.2 to 10.4), places cache marks (12.2) and writes the summary (12.1,
LOOP-60 to 65). The join writes the pop's result (5), forgets reads (6),
writes the frame line and the slide's line (7, 8.6), computes the fresh
guard (9), and tells the person (8.6, 10). mem refuses what it refuses
(4.6, 9). `doc.go` says how the join uses this package, in order.
