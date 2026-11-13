// Package term is gnosh's hold on the terminal: it reads what the person
// types, writes what gnosh shows, and puts the terminal back the way it
// found it. gnosh uses no terminal-interface library, so everything a
// terminal needs said to it is said here.
//
// # The pieces
//
// Decoder turns the bytes a terminal sends into events: keys (Key), pasted
// text (Paste), the window gaining or losing focus (Focus), answers to
// questions gnosh asked (Reply), mouse reports (Mouse, which gnosh never
// asks for but must not mistake for typing) and anything it cannot make
// sense of (Unknown). It is a plain state machine with no clock and no I/O.
//
// Terminal owns a real terminal. Open puts it in raw mode and starts a
// goroutine that reads it, feeds a Decoder and hands the events over on a
// channel, together with Resize when the window changes size and Resume
// when gnosh gets the terminal back after being stopped. Close, Release,
// Suspend and the signals that end or stop the process all restore the
// terminal's settings exactly.
//
// The file seq.go has every control sequence gnosh writes, by name, and
// Probe asks a terminal which of the optional ones it understands.
//
// Renderer draws the interface, as described below.
//
// The package vtest is a terminal emulator for tests: write to it what
// gnosh writes to a terminal, and ask it what a person would see.
//
// # The rules gnosh's interface follows on a terminal
//
// The screen is the terminal's, not gnosh's. gnosh never switches to the
// alternate screen and never turns on mouse reporting, so scrolling back,
// selecting and copying are the terminal's own and work as they do for
// any command's output. There are deliberately no functions here for
// either.
//
// What is finished is written once. Text gnosh is done with is committed:
// written above everything that may still change, and never touched again.
// It scrolls away into the terminal's scrollback like the output of any
// program.
//
// Only the live region is redrawn. Below the committed text are a few
// lines that change in place: the answer as it streams, the input box, a
// status line. The renderer redraws the rows that changed and no others,
// in one write, with the cursor hidden while it draws and, where the
// terminal knows how, the display held still until the update is whole.
//
// Movement is relative. The renderer knows how many rows the cursor is
// below the top of the live region, and nothing else about where it is on
// the screen. It goes up with "cursor up", down with line feeds (which
// scroll the terminal when the live region grows past the bottom), and
// never names a row by number.
//
// A line is one row. The caller wraps and styles its lines; the renderer
// makes sure each takes exactly one row, because all of its arithmetic
// rests on that. It passes on text, colours and styles (SGR) and
// hyperlinks (OSC 8) and drops every other control character and escape
// sequence; it cuts off what is wider than the terminal; and it turns the
// terminal's own line wrapping off while it draws, so that where the
// terminal measures a character wider than gnosh did, the line is clipped
// at the edge instead of spilling onto a row gnosh does not know about.
//
// A row is erased whole before it is written, and the screen is never
// erased "to the end". tmux remembers how long a line used to be unless
// the whole row is erased, and saves the screen in its history when a
// program erases to the end of the screen from the top left corner; both
// leave stray rows behind when the window is resized. Erasing row by row
// costs a few bytes and works the same everywhere.
//
// The terminal is left as it was found. Raw mode is undone, and the modes
// gnosh turned on (bracketed paste, focus reports, the kitty keyboard
// level) are turned off, on every way out: Close, a panic in a goroutine
// that deferred Terminal.RestoreOnPanic, and the signals SIGTERM, SIGHUP,
// SIGINT and SIGQUIT. SIGTSTP gives the terminal back, stops the process,
// and takes the terminal again when the shell continues it.
//
// Keys are keys. In raw mode Ctrl+C, Ctrl+Z and Ctrl+\ send no signals;
// they arrive as Key events and mean what the interface decides. Only the
// first level of the kitty keyboard protocol is asked for, and only of a
// terminal that says it has it; with it, Shift+Enter and Ctrl+Enter can be
// told from Enter. Without it, a line feed (Ctrl+J) is the newline key
// that works everywhere.
//
// Nothing is assumed about the terminal that it did not say. Probe asks,
// waits briefly, and gnosh does without whatever was not confirmed.
//
// # Starting and stopping
//
// A program that draws on a terminal does this, in order:
//
//	t, err := term.Open(os.Stdin, os.Stdout, term.Options{})
//	// on error: input or output is not a terminal; print plainly instead
//	defer t.Close()
//	caps, typed, _ := term.Probe(ctx, t, t.Events(), 0)
//	t.SetModes(term.Modes{BracketedPaste: true, KittyKeyboard: caps.KittyKeyboard})
//	cols, rows, _ := t.Size()
//	r := term.NewRenderer(t, cols, rows, text.Width)
//	r.SetSynchronized(caps.SynchronizedOutput)
//	// act on typed, then on each event from t.Events():
//	//   term.Resize: r.Resize(ev.Cols, ev.Rows), then set the live region again
//	//   term.Resume: r.Reset(), then set the live region again
//	// before exit, and before t.Release or t.Suspend: r.Clear()
//
// The renderer starts drawing at the cursor and assumes the cursor is at
// the start of an empty line. A program that cannot be sure (the command
// before it may have printed a line with no line feed at its end) can ask
// with Terminal.QueryCursorPosition and write "\r\n" first if the column
// is not 1.
//
// # What cannot be made right
//
// A resize is the hard case, because the terminal rearranges the screen
// before gnosh hears of it and terminals differ in how. See Renderer.Resize
// for the strategy and its limits. In short: with the cursor hidden the
// redraw is exact on every kind of terminal; with the cursor shown inside
// the live region it is exact on terminals that wrap lines again and keep
// the cursor on its character (iTerm2, Apple's Terminal, Ghostty, tmux),
// and on others after SetReflow(false); and rows of the live region that
// the terminal moved off the top of the screen cannot be reached at all.
// tmux moves rows off the top to make room for re-wrapped lines even when
// the screen has empty rows at the bottom, so there the last limit is met
// sooner: when a pane is made narrower while the live region is near the
// top of a mostly empty screen.
//
// With the terminal's line wrapping off, tmux joins an accent written
// after a letter in the last column to the letter before it. The renderer
// turns wrapping back on for such a letter. Other marks that follow a
// character in the last column (variation selectors, joiners) are left to
// the terminal, which may draw them one cell to the left.
//
// # Sources
//
// The behaviour here comes from public documents: ECMA-48 (control
// functions), the DEC VT100 and VT220 manuals and Paul Williams' parser
// for them (vt100.net), XTerm Control Sequences
// (invisible-island.net/xterm/ctlseqs), the kitty keyboard protocol
// (sw.kovidgoyal.net/kitty/keyboard-protocol), the synchronized output
// specification (mode 2026), and the documentation of golang.org/x/term
// and golang.org/x/sys/unix. Where the documents are silent, vtest's tests
// say what a real terminal, tmux, was seen to do.
package term
