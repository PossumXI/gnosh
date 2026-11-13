// Package markdown turns the Markdown a model writes into styled lines for a
// terminal, while the text is still arriving.
//
// gnosh draws in the terminal's own scrollback: what is finished is written
// once and never touched again, and only a few lines at the bottom are
// redrawn. So this renderer has a job that an ordinary one does not have.
// At every moment it must say which of its lines are final, and can be
// written for good, and which may still change. A Renderer does that:
//
//	r := markdown.New(markdown.Options{Width: cols, Profile: profile})
//	final := r.Write(delta) // lines that have just become final
//	live := r.Live()        // the lines after them, as they stand now
//	final = r.Close()       // the text is over: the rest, now final
//
// Render does the same for a text that is complete.
//
// # The one promise
//
// However a text is cut into pieces, the lines returned by Write, in order,
// followed by the lines of Live, are exactly the lines Render returns for
// the text so far. Two things follow. A line that was returned as final is
// never different in the end, whatever came after it. And Live is not a
// guess of its own: it is what Close would return if the text stopped
// here, so the two can never disagree.
//
// The renderer keeps this promise by never deciding anything on the
// strength of how the text happened to arrive. Its state after a piece of
// text depends only on the text so far. Where the meaning of some bytes
// depends on bytes that have not come, it waits, and the waiting is
// bounded (see Reach below), so nothing can be kept from becoming final
// for ever.
//
// # When a line becomes final
//
// A paragraph. Prose becomes final as it goes: a line of a paragraph is
// final as soon as the line after it has begun. More exactly, a line is
// final once (1) the look of everything on it is settled and (2) a word
// has been seen that does not fit on it, or a hard break ends it. The
// last line of a paragraph stays live until the paragraph ends: with a
// blank line, with the start of another block, or with the end of the
// text. So an ordinary paragraph has one line live, the one being
// written, plus the word being written.
//
// What can keep the look of a line from being settled is something that
// reaches back:
//
//   - An opening mark that is still waiting for its partner: "*", "_" or
//     "~" for emphasis, a backtick for a code span, "[" for a link or an
//     image, "<" for an address or a tag. Everything from the mark on
//     stays live until the partner comes, the paragraph ends, or Reach
//     bytes have gone by. In the last two cases the mark is plain text.
//     A mark that cannot open anything (a "*" with blanks on both sides,
//     an "_" inside a word) holds nothing back.
//   - The word being written, which may turn out to be an address
//     ("www.", "http://", "https://", "mailto:", "xmpp:", or the part of
//     an e-mail address before the "@"), a character reference such as
//     "&amp;", or a backslash escape. It is settled when it ends.
//   - The character being written, which may still combine with what
//     follows: an accent, a skin tone, the second half of a flag.
//   - An underline on the next line, which would make the paragraph a
//     setext heading. Only a paragraph of a single line of at most 80
//     bytes can become one, so this holds back the first line of a
//     paragraph until it is longer than that or the next line has begun.
//     A longer paragraph followed by "---" is a paragraph and a thematic
//     break; followed by "===", the "===" is part of its text.
//   - A delimiter row on the next line, which would make the line above
//     it the header of a table. Only a line with a pipe in its first 80
//     bytes, and of at most Reach bytes, can become one. So the first 80
//     bytes of every line are held until the line is longer or has ended,
//     and a line with an early pipe is held until the next line has shown
//     that it is not a delimiter row.
//
// A link reference definition does not reach back at all: it gives its
// label an address from that point of the text on. A reference to a label
// that is defined only later is shown as it was written. The definition
// itself is shown as a line, since a terminal has no other place for its
// address.
//
// A heading. An ATX heading ("## Title") flows like a paragraph of one
// line: its lines become final as the next one begins, and the last when
// the line ends. A run of "#" at its end is held back until then, since it
// is the closing sequence if nothing follows it. A setext heading becomes
// final, all of it, when its underline is complete.
//
// A code block. Once the opening fence is complete, each line of code is
// final when its line ending arrives, and so is each line of an indented
// code block. A line too long for one row becomes final row by row: a row
// is final as soon as text has come that does not fit on it. (With a
// highlighter, a line is first kept whole, up to 2048 bytes, so that the
// highlighter sees all of it.) The line being written is live while it
// could still be the closing fence. Blank lines after an indented code
// block are live until the next line shows whether the block goes on. A
// fence that is never closed runs to the end of the text.
//
// A table. A table is the one block whose first lines depend on its last:
// a column is as wide as its widest cell. So a table is live as a whole,
// from its header on, until it ends, which is at a blank line or at the
// start of another block. To keep that bounded there is a cap: when a
// table would take more than TableLive rows of the terminal (24 unless
// set), all of it so far becomes final with the widths its columns have
// then, the widths are frozen, and from there on each row becomes final
// when its line ends, wrapped into those widths. (If the terminal is
// resized after that, the frozen widths are shared out again for the new
// width.)
//
// A list. A list is no more live than the blocks inside its items: an
// item's paragraph becomes final line by line like any paragraph, with
// the item's marker on its first line. Nothing about a list reaches back.
// Whether items are a blank line apart is decided item by item, from
// whether the text has a blank line there, and not for the list as a
// whole; and a numbered item shows the number it was written with, so
// its marker is as wide as its own number.
//
// A block quote. The same: the bar is put in front of each line of the
// blocks inside it, and those become final by their own rules.
//
// A thematic break, a link reference definition and a line of an HTML
// block are final when their line ends; HTML is shown as the text it is.
// A blank line between two blocks is returned together with the first
// line of the second block, so that final output never ends in one.
//
// # Reach
//
// Standard Markdown lets an opening mark wait for its partner until the
// end of the paragraph, however far that is. A renderer that writes lines
// for good cannot: a single "*" at the start of a long paragraph would
// keep all of it live. So this package bounds how far back later text can
// reach, with Options.Reach (640 bytes unless set), and applies the bound
// to whole texts and streamed ones alike, which is what keeps the two
// equal:
//
//   - emphasis, strikethrough, a code span, a link or image, an address in
//     angle brackets and raw HTML must end within Reach bytes of where
//     they begin, or their opening mark is plain text;
//   - a thematic break, a setext underline, a code fence, a table's
//     delimiter row, a link reference definition and a tag that has a line
//     to itself are recognised only on lines of at most Reach bytes;
//   - a run of more than Reach bytes of one mark ("`", "*", "_", "~") or of
//     blanks means nothing: it is text, and such a run of blanks is not a
//     blank line either.
//
// A few more limits serve the same end, that no text can keep the renderer
// waiting or working without bound. Block quotes and list items nest at
// most 32 deep, and a marker past that is text. A table row has at most
// 8192 bytes; a longer line ends the table. A character with more than
// 256 bytes of marks on it is taken as several, and a line never holds
// more than 32 bytes for each of its cells, so characters that take no
// room cannot fill a line for ever.
//
// Inside these bounds the syntax is CommonMark's, with the tables,
// strikethrough, task lists and bare addresses of GitHub Flavored
// Markdown. Text a model writes within them, which is nearly all of it,
// renders as the specifications say. The other differences are deliberate
// too: the limits on setext headings and table headers above, both rows
// of a table's top must hold a pipe, link reference definitions take one
// line and work forwards only, "<br>" is a line break, lists keep the
// spacing and the numbers they were written with, and lines that begin
// with a pipe but are not a table stay on lines of their own.
//
// # Layout
//
// Lines are wrapped to the width at spaces, after a hyphen between
// letters, and between East Asian characters (never before closing
// punctuation or after an opening bracket), with widths counted by
// internal/text, so that wide characters and emoji take the two cells
// they are drawn in. A word longer than a line is broken where the line
// is full. Nothing is ever cut off or replaced by an ellipsis.
//
// Blocks are a blank line apart. A list item's text hangs behind its
// marker, a quote's lines stand behind a bar, a heading behind its "#"
// marks. Containers nest by putting their marks side by side, up to half
// the width (or the width less sixteen cells in a wide terminal); deeper
// than that they are not set in any further.
//
// Code is never re-flowed. A code line that fits is written as it is,
// after a margin of two spaces, with tabs as spaces up to the next tab
// stop. There is no border, no gutter and no background, so what is
// selected in the terminal is the code. A line that does not fit is cut
// at the edge of the terminal into rows, and each row after the first
// carries a mark in the margin, where the first row has spaces: the mark
// says that the row continues a line, and it never stands among the
// code's own characters, so that a column selection of the code still
// gets the code alone. What a terminal shows can never be the code byte
// for byte (tabs, and lines longer than the terminal), so CodeBlocks
// returns each block's code exactly as it was written, for copying.
//
// A table is drawn as a grid when its columns fit side by side. If they do
// not fit at their full widths, each column is given what it cannot do
// without (room for its longest word, up to 24 cells, and at least eight
// cells), the rest is shared among the columns that want more, and cells
// wrap inside their columns. A row of several lines is fenced off from its
// neighbours by a rule. A table that does not fit even so is stacked:
// each row becomes a short record, one line per cell, each behind its
// column's header.
//
// # Safety
//
// A model's text can hold anything, including the escape sequences that
// make a terminal move its cursor or change its title. None of them
// reaches the output: control characters are shown as the pictures
// Unicode has for them, and bytes that are not UTF-8 as U+FFFD. The only
// escape sequences in the lines this package returns are the styles and
// hyperlinks it writes itself, and under text.NoColor there are none.
//
// # Cost
//
// Write takes time proportional to the text it is given plus the size of
// what is still live, and Live to the size of what is live. What is live
// is bounded: by Reach for prose, by a line for code, by TableLive rows
// for a table. A text of n bytes fed in pieces of any size therefore
// costs O(n) in all, with a factor that is small for ordinary text and at
// worst proportional to Reach for text made of marks that never close;
// it is never quadratic. (An ordinary answer fed five bytes at a time,
// with Live looked at after every piece, has the renderer look at about
// 24 bytes for each byte of text; the tests count this and hold every
// kind of text they can think of to a fixed factor.) Memory holds what
// is live and the link reference definitions seen; what is final is the
// caller's.
package markdown
