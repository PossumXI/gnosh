// Package menu is the small questions gnosh puts to a person: choose one
// of these, yes or no, answer these few questions, type an answer, type a
// password.
//
// Each question is a plain value with no terminal behind it, like the
// editor: Handle takes the terminal's events one at a time and says what
// each did, and Render says what to draw at a width, as rows that are
// already styled and cut to fit, with the cursor's row and column.
//
//	q := menu.NewSelect(menu.SelectOptions{
//		Title: "Run this command?",
//		Body:  []string{command},
//		Options: []menu.Option{
//			{Label: "Allow once", Text: menu.TextOptional},
//			{Label: "Allow, and do not ask again for", Description: rule},
//			{Label: "Deny", Text: menu.TextOptional},
//			{Label: "Deny, and tell the model why", Text: menu.TextRequired},
//		},
//		CancelHint: "deny",
//	})
//	for ev := range events {
//		res := q.Handle(ev)
//		if res.Done {
//			// res.Cancelled, or res.Index with res.Text
//			break
//		}
//		v := q.Render(cols)
//		renderer.SetLive(v.Lines, v.CursorRow, v.CursorCol)
//	}
//
// A result with Done set ends the question; Cancelled says it ended with
// no answer, and that is never also a choice. A question that has ended
// takes no more events and draws nothing. An event that means nothing to
// a question comes back with Handled false. A key that the terminal says
// is being held down moves the cursor but never chooses: the Enter that
// sent a prompt must not also answer the question the prompt led to.
// (Terminals report that only when asked to, which gnosh does not do yet;
// until then the application should let a moment pass, or a key go by,
// between drawing a question and taking its answer.)
//
// # The questions
//
// Select offers a list of options under a title, with an optional body
// above them: the thing being asked about, such as a command or a diff,
// as lines the caller has already styled. An option may take text: a note
// that goes with it (TextOptional), or text that is the point of it
// (TextRequired). The text field is the editor's, so the same keys edit in
// it, a paste goes in whole, and it grows as lines are added or wrap.
//
// NewConfirm makes a Select that is a yes-or-no question. The cursor
// starts on No unless the caller says otherwise.
//
// Form asks several questions at once, one tab each, with a last tab on
// which the answers are looked over and sent: what the model's question
// tool shows. A question takes one answer or several, and has a last row
// on which the person can answer in their own words instead.
//
// Input asks for an answer that is typed.
//
// Secret asks for something that must not be seen. What is typed is drawn
// as one mark for each character and is in no result but the one that
// answers; the question has no history, no undo and no completion, and the
// buffer that held the answer is overwritten when it ends.
//
// Every question takes at most MaxHeight rows (20 unless told otherwise;
// SetMaxHeight changes it), because it is drawn in the few rows at the
// bottom of the terminal that are redrawn in place. A body or a list too
// tall for the room scrolls, with a mark at the edge where rows are
// hidden. Under each question a dim line names the keys, as many of them
// as fit. The option the cursor keys are on is marked with a pointer, a
// ticked option with a box, the tab in front with brackets: nothing is
// said by colour alone.
//
// Whatever a question is given to show is made safe to draw first. Labels,
// titles and the model's words are plain text: a control character in
// them shows as a stand-in ("^["). The body keeps its colours, styles and
// hyperlinks, and nothing else that a terminal would act on.
//
// # Keys
//
// Where each binding comes from: "brief" is the build brief for this
// package, "cc" is spec/cc/interactive.md with its section, and "own" is a
// choice made here.
//
//	Select and NewConfirm
//	Up, Down, k, j                 move, going round (brief)
//	Ctrl+P, Ctrl+N                 the same (own; as in the editor)
//	Tab                            on an option that takes text: open its field (cc 4); otherwise move down (brief)
//	Shift+Tab                      move up (brief)
//	Home, End                      first and last option (own)
//	Enter, Ctrl+M, Ctrl+J          choose the option the cursor is on (brief)
//	1 to 9                         choose that option (brief)
//	the option's own key           choose it; a letter in either case (brief)
//	Esc, Ctrl+C                    cancel (brief; Esc is "no" on a permission question: cc 4)
//	PgUp, PgDn                     scroll the body by a page (own)
//	Shift+Up, Shift+Down           scroll the body by a row (own)
//
//	In the text field of an option
//	Enter                          choose the option, with the text (brief, cc 4); for a note, with none if nothing is typed
//	Shift+Enter, Alt+Enter, Ctrl+J new line, and every other editing key, as in the editor
//	Esc, Tab, Shift+Tab            back to the list; the text stays with the option (brief; Tab cc 4)
//	Ctrl+C                         back to the list; the text is thrown away (own)
//	Up on the first row, Down on the last   leave the field for the option above or below (own)
//
//	Form
//	Left, Right, Tab, Shift+Tab    the question before or after, going round (brief; cc 3 for Left and Right)
//	Up, Down, k, j, Home, End      move among the answers (brief)
//	Space                          tick or untick (several answers); choose and stay (one answer) (brief)
//	Enter                          choose and go on (one answer); go on (several answers), ticking the row first if nothing is ticked (own)
//	1 to 9                         as Enter (one answer) or Space (several) on that row (own)
//	Enter on the last row          open the field for an answer in the person's own words (brief)
//	Enter on the review            send the answers (brief)
//	Esc, Ctrl+C                    cancel the form (brief)
//	In its field: Enter            keep the words as the answer and go on; Tab and Shift+Tab change question;
//	                               Esc leaves the field, or cancels a question that is only a field;
//	                               Ctrl+C throws the words away (own)
//
//	Input
//	Enter                          answer (brief)
//	Esc                            cancel (brief)
//	Ctrl+C                         empty the field; cancel when it is empty (own; as Ctrl+C in the editor)
//	everything else                as in the editor
//
//	Secret
//	Enter                          answer, when something is typed (spec/gnosh/harness-tools.md TOOL-45)
//	Backspace, Ctrl+H              take back one character (own)
//	Ctrl+U                         start again (own)
//	Esc, Ctrl+C                    cancel (TOOL-40: Esc at the password prompt ends the turn)
//	PgUp, PgDn and all other keys  nothing
//
// # What the specs left open, and what was chosen
//
//   - spec/cc gives yes and no a comment through Tab; the brief gives the
//     permission question an option of its own for "no, and here is why".
//     Both are here: Tab opens a note on an option made with
//     TextOptional, and an option made with TextRequired is its text.
//     Because of the first, Tab moves down only on options that take no
//     text.
//   - Shift+Tab moves up in a Select. On a file-edit question spec/cc has
//     it choose "allow for the rest of the session"; the application gives
//     the key that meaning itself before the question sees it.
//   - A form can be sent with questions unanswered. The review says which
//     they are, in so many words, and the person decides; a form that
//     refused to be sent would leave Esc as the only way out.
//   - Words typed into a form's last row and left without Enter stay on
//     the row. They become the answer if the question has no other, or
//     takes several; they do not push out an option already chosen.
//   - A pasted password is its first line: the line break that so often
//     comes with a copied password is not part of it. Nothing else is
//     trimmed.
package menu
