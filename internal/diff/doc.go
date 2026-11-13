// Package diff compares two versions of a text and shows the difference.
//
// Every file edit a model makes is shown to a person as a diff, and when the
// person is asked whether a change may be made, the diff is what they decide
// on. So the package holds itself to two things before all else. The
// difference it reports is the real one: applied to the text before, it
// gives exactly the text after, byte for byte. And nothing in it is hidden:
// not a line, not a space at the end of one, not a character that takes no
// room on the screen.
//
// # What is in it
//
// Lines and Compare work out the edit script that turns one text into the
// other: runs of lines that are kept, deleted and inserted. Compare also
// returns the Stats, the counts for a status line: lines added and removed,
// and whether the line endings or the final newline changed.
//
// Unified writes the difference in the unified format of "diff -u", the form
// a model is shown after an edit and the form patch reads. Hunks cuts a
// script into the groups of changes that format is made of.
//
// Render draws the difference for a terminal: a header, line numbers, a
// sign on every changed line, the changed words marked inside the lines,
// long lines wrapped. A Theme sets its colours and DefaultTheme is gnosh's
// own.
//
// Spans compares one line with another and returns the stretches that
// changed, which Render uses to mark words.
//
// Nothing here reads the terminal, the clock or the environment, and the
// package keeps no state. What a caller knows about the terminal it passes
// in: the width, the colour profile, the theme.
//
// # The comparison
//
// A text is taken as a list of lines, each with its line ending. The script
// is the smallest there is: no other deletes and inserts fewer lines. It is
// found with the algorithm of Eugene W. Myers, "An O(ND) Difference
// Algorithm and Its Variations" (1986), in the form that needs memory only
// in proportion to the size of the texts: the search runs from both ends at
// once, and where the two meet the problem is cut in two. Before it starts,
// the lines the texts begin and end with in common are set aside, and so are
// the lines that have no equal in the other text, which no script could
// keep. What is left for the search is small even for two large texts with
// little in common.
//
// The time the search takes grows with the number of differences as well as
// with the size, so it is given a budget, counted in line comparisons. Two
// texts that are too different for the budget are finished more roughly: a
// part that is small or differs little still gets its smallest script; a
// larger part is cut at the lines that occur exactly once on each side, the
// idea of patience diff; and a part without such lines is gone through a
// short search at a time. A search that is plainly not going to finish is
// given up before its budget is spent. The script is then no longer certain
// to be the smallest, and Result.Minimal says so, but it is as correct as
// any other, and in the cases measured it is within a hundredth of the
// smallest. The bound on the work is in Compare's documentation.
//
// The smallest script is seldom the only one of its size, and two more steps
// choose among them for the sake of the reader. Where a few kept lines lie
// between changes, such as the brace and the empty line between a function
// that was changed and one that was added, they are kept in the way that
// sets each changed line against its own old form. And a run of lines that
// is only inserted, or only deleted, and could begin a line earlier or later
// is put where it begins and ends at empty lines and at the least indented
// lines, so that an added function is shown as that function. This last is
// the idea git documents as its indent heuristic; the rules and scores here
// are this package's own. Neither step changes the size of the script.
//
// Before a script leaves the package it is checked once more against both
// texts, line by line. Should the two steps that tidy it ever have spoilt
// it, the script as the search found it is handed out, and should that be
// wrong as well, one that deletes all of the old text and inserts all of the
// new, which is always right.
//
// # Line endings and the last line
//
// Lines are compared whole. A line that ends in CR LF and the same words
// ending in LF are different lines, and so are a line that ends with a
// newline and the same words at the end of a text that has none: the
// unified form says so for each such line, with "\ No newline at end of
// file" where the format calls for it, because patch needs it said.
//
// A person does not need it said two hundred times. After the script is
// found, the lines that were replaced are compared once more by their words
// alone, and pairs that differ only in how they end are told apart from
// lines that changed. Stats counts them separately, and Render says "line
// endings changed: CRLF to LF" once and shows only the lines whose words
// changed.
//
// # Inside a line
//
// Where lines are replaced, Render pairs each deleted line with the inserted
// line most like it, by the words the two share, keeping the pairs in order.
// Pairing the first with the first and the second with the second would go
// wrong as soon as a line was added among them. For each pair, Spans finds
// what changed: it compares the lines word by word, and where one word took
// the place of another much like it, letter by letter from both ends, in
// whole grapheme clusters. Two lines with less than half their words in
// common are not marked at all: marks over most of a line say nothing the
// line does not.
//
// # What is drawn
//
// The look is plain on purpose. Colour is in the letters only: a rose for
// what went, a green for what came, grey for numbers and notes, and an
// underline under the words that changed. Nothing is a filled block, so the
// same theme reads on a dark and on a light background. Every changed line
// has a sign, every line its numbers, and every note is in words, so without
// colour nothing is lost but the underline.
//
// A file's text is never sent to the terminal as it is. An escape sequence
// in a file could move the cursor and redraw the diff above it; a character
// that takes no room could make two different names look the same. Control
// characters are drawn as their pictures, invisible characters as their
// numbers, spaces at the end of a changed line as dots, and a tab that
// became spaces, or the other way round, as what it is.
//
// # What it was written from
//
// Myers' paper for the search; Bram Cohen's description of patience diff for
// the rough cut at lines that occur once; git's documentation of its indent
// heuristic for the idea of sliding a change; the POSIX specification of
// "diff -u" and the GNU diffutils manual for the unified format; Unicode's
// rules for grapheme clusters, by way of the text package. No other diff
// program's code was read or used.
package diff
