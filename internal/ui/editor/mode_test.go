package editor

import (
	"strings"
	"testing"
)

// The first character decides the mode. It is drawn as the prefix, and
// the cursor never goes in front of it.
func TestModes(t *testing.T) {
	g := newRig(t, 30, Options{})
	if g.e.Mode() != ModePrompt {
		t.Fatalf("mode of an empty box: %v", g.e.Mode())
	}
	g.do(`"!"`).text("!").screen(0, 2, "!")
	if g.e.Mode() != ModeShell || g.e.IsEmpty() {
		t.Fatalf("after !: mode %v, empty %v", g.e.Mode(), g.e.IsEmpty())
	}
	g.do(`"ls -la"`).text("!ls -la").screen(0, 8, "! ls -la")
	// Home and Left stop after the character that sets the mode.
	g.do(`<Home>`).cursor(1).screen(0, 2, "! ls -la")
	g.do(`<Left> <Ctrl+A> <Ctrl+Home> <Alt+B> <Up>`).cursor(1)
	// Enter sends the line with its "!" and says it is a shell line.
	g.do(`<Enter>`).submitted("!ls -la", ModeShell).text("")

	g.do(`"/"`).text("/").screen(0, 2, " /")
	g.do(`"model opus"`).text("/model opus").screen(0, 12, " /model opus")
	if g.e.Mode() != ModeCommand {
		t.Fatalf("after /: mode %v", g.e.Mode())
	}
	g.do(`<Enter>`).submitted("/model opus", ModeCommand)

	// A space first keeps a prompt a prompt.
	g.do(`" /etc/hosts is odd"`).text(" /etc/hosts is odd")
	if g.e.Mode() != ModePrompt {
		t.Fatalf("mode with a space first: %v", g.e.Mode())
	}
	g.do(`<Enter>`).submitted(" /etc/hosts is odd", ModePrompt)

	// Rows after the first are indented under the text.
	g = newRig(t, 12, Options{})
	g.do(`"!echo one two three"`).screen(1, 11, "! echo one", "  two three")
	g.do(`<Ctrl+Home>`).screen(0, 2, "! echo one", "  two three")
	g = newRig(t, 12, Options{})
	g.do(`"/rename the session"`).screen(2, 9, " /rename", "  the", "  session")
}

// Leaving a mode: Backspace, Ctrl+U, Ctrl+W and Alt+Backspace at the
// start take the character away, and so does Esc on a line that holds
// nothing else.
func TestLeavingModes(t *testing.T) {
	for _, k := range []string{`<Backspace>`, `<Ctrl+U>`, `<Ctrl+W>`, `<Alt+Backspace>`, `<Ctrl+H>`, `<Esc>`} {
		for _, first := range []string{"!", "/"} {
			g := newRig(t, 30, Options{})
			g.do(`"` + first + `"`).do(k).text("")
			if g.e.Mode() != ModePrompt || !g.last.Handled || g.last.Action != ActionNone || g.last.Hint != HintNone {
				t.Errorf("%s after %q: mode %v, result %+v", k, first, g.e.Mode(), g.last)
			}
			g.screen(0, 2, ">")
			// It is undone like any edit.
			g.do(`<Ctrl+_>`).text(first)
		}
	}
	// With text after it, the keys that delete take only the character
	// and keep the text, and Esc is the ordinary first Esc.
	for _, k := range []string{`<Backspace>`, `<Ctrl+U>`, `<Ctrl+W>`, `<Alt+Backspace>`} {
		g := newRig(t, 30, Options{})
		g.do(`"!ls" <Home>`).do(k).text("ls").cursor(0).screen(0, 2, "> ls")
		// The character is not kept for Ctrl+Y: it is not text.
		g.do(`<Ctrl+Y>`).text("ls")
	}
	g := newRig(t, 30, Options{})
	g.do(`"!ls" <Esc>`).text("!ls")
	if g.last.Hint != HintEscClear {
		t.Errorf("Esc on a shell line with text: %+v", g.last)
	}
	// Ctrl+U after the text clears the text and stays in the mode; a
	// second leaves it.
	g = newRig(t, 30, Options{})
	g.do(`"!ls -la" <Ctrl+U>`).text("!").screen(0, 2, "!")
	g.do(`<Ctrl+U>`).text("")
	g.do(`<Ctrl+Y>`).text("ls -la")
	// Ctrl+W and Alt+Backspace stop at the mode character too.
	g = newRig(t, 30, Options{})
	g.do(`"/model" <Ctrl+W>`).text("/")
	g.do(`"model" <Alt+Backspace>`).text("/")
	// Ctrl+D on a line that holds only the character does nothing: it
	// neither deletes it nor counts towards leaving gnosh.
	g.do(`<Ctrl+D> <Ctrl+D>`).text("/").action(ActionNone)
	// Enter on a shell line with no command sends nothing.
	g = newRig(t, 30, Options{})
	g.do(`"! " <Enter>`).action(ActionNone).text("! ")
}

// The mode follows the first character however it got there.
func TestEnteringModes(t *testing.T) {
	// Pasted.
	g := newRig(t, 30, Options{})
	g.do(`<Paste "!git status">`).text("!git status").cursor(11).screen(0, 12, "! git status")
	if g.e.Mode() != ModeShell {
		t.Errorf("mode after pasting a shell line: %v", g.e.Mode())
	}
	// Typed in front of text that was there.
	g = newRig(t, 30, Options{})
	g.do(`"ls" <Home> "!"`).text("!ls").cursor(1).screen(0, 2, "! ls")
	// Loaded.
	g = newRig(t, 30, Options{})
	g.e.SetText("/help")
	g.screen(0, 6, " /help")
	// Recalled from the history.
	g = newRig(t, 30, Options{})
	g.e.SetHistory(drafts("!make test"))
	g.do(`<Up>`).screen(0, 11, "! make test")
	// A line joined onto the first line cannot bring a mode with it,
	// but the first line's own first character can change.
	g = newRig(t, 30, Options{})
	g.do(`"a" <Ctrl+J> "!b" <Home> <Up> <Home> <Delete>`).text("\n!b")
	if g.e.Mode() != ModePrompt {
		t.Errorf("mode with an empty first line: %v", g.e.Mode())
	}
	g.do(`<Delete>`).text("!b").cursor(1)
	if g.e.Mode() != ModeShell {
		t.Errorf("mode after the lines joined: %v", g.e.Mode())
	}
	// A mark that would join the character before it stands alone after
	// the mode character.
	g = newRig(t, 30, Options{})
	g.do(`"!\u0301x"`).text("!\u0301x").screen(0, 11, "! <U+0301>x")
	g.do(`<Home>`).cursor(1)
	g.do(`<Right>`).cursor(3)
	g.do(`<Backspace>`).text("!x")
	// A collapsed paste first is a prompt, whatever the paste begins
	// with.
	g = newRig(t, 30, Options{})
	g.e.Handle(pasteOf("!" + strings.Repeat("x\n", 20)))
	if g.e.Mode() != ModePrompt {
		t.Errorf("mode with a collapsed paste first: %v", g.e.Mode())
	}
	g.screen(0, 19, "> [pasted 20 lines]")
}

// A shell line keeps the backslash before a new line, because that is
// how a shell is told a command goes on.
func TestShellBackslash(t *testing.T) {
	g := newRig(t, 30, Options{})
	g.do(`"!echo a \\" <Enter> "b"`).text("!echo a \\\nb").screen(1, 3, "! echo a \\", "  b")
	g.do(`<Enter>`).submitted("!echo a \\\nb", ModeShell)
}

// Under no colour the modes are told apart by their marks alone, and
// with colour each mark has its own.
func TestModeLooks(t *testing.T) {
	th := DefaultTheme(3)
	g := newRig(t, 30, Options{Theme: th})
	g.do(`"x"`)
	if v := g.e.Render(30); v.Lines[0] != th.PromptStyle.Render("> ")+"x" {
		t.Errorf("prompt row: %q", v.Lines[0])
	}
	g.do(`<Home> "!"`)
	if v := g.e.Render(30); v.Lines[0] != th.ShellStyle.Render("! ")+"x" {
		t.Errorf("shell row: %q", v.Lines[0])
	}
	g.do(`<Backspace> "/"`)
	if v := g.e.Render(30); v.Lines[0] != th.CommandStyle.Render(" /")+"x" {
		t.Errorf("command row: %q", v.Lines[0])
	}
	// No colour: nothing but text.
	g = newRig(t, 30, Options{})
	g.do(`"!x"`)
	if v := g.e.Render(30); v.Lines[0] != "! x" {
		t.Errorf("shell row without colour: %q", v.Lines[0])
	}
	// A theme with marks of its own, of different widths: the text
	// stays in one column.
	th = DefaultTheme(0)
	th.Prompt, th.ShellPrompt, th.CommandPrompt = "gnosh> ", "$ ", "/"
	g = newRig(t, 30, Options{Theme: th})
	g.do(`"x"`).screen(0, 8, "gnosh> x")
	g.do(`<Home> "!"`).screen(0, 7, "$      x")
	g.do(`<Backspace> "/" <Ctrl+E> <Ctrl+J> "y"`).screen(1, 8, "/      x", "       y")
}
