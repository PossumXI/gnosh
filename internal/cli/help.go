package cli

import (
	"strings"
)

// helpWidth is the widest line help writes: an 80-column terminal with two
// columns to spare.
const helpWidth = 78

// labelWidth is how wide the left column of a list of commands is.
const labelWidth = 24

// nbsp stands in help text for a space a line must not be broken at, inside
// an example. It is written out as an ordinary space.
const nbsp = "\u00a0"

// Help returns the help text for gnosh, or for the command the words name
// ("session", "list"). It is made from the table, so it always says what
// Parse does, including which commands and flags are not built yet. Every
// line fits 80 columns. A word that names no command is an *Error with the
// nearest real name in it.
func (t *Table) Help(command ...string) (string, error) {
	c, err := t.find(command)
	if err != nil {
		return "", err
	}
	return t.render(c), nil
}

// helpText builds help a line at a time.
type helpText struct {
	b strings.Builder
}

// line writes one line as it is.
func (h *helpText) line(s string) {
	h.b.WriteString(strings.ReplaceAll(s, nbsp, " "))
	h.b.WriteByte('\n')
}

// heading writes a blank line and then a section's name.
func (h *helpText) heading(s string) {
	h.line("")
	h.line(s)
}

// para writes text as lines indented by indent columns.
func (h *helpText) para(indent int, text string) {
	pad := strings.Repeat(" ", indent)
	for _, l := range wrap(text, helpWidth-indent) {
		if l == "" {
			h.line("")
			continue
		}
		h.line(pad + l)
	}
}

// item writes a thing and, under it and further in, what it does.
func (h *helpText) item(what, does string) {
	h.line("  " + what)
	h.para(6, does)
}

// row writes a thing and, beside it, what it does. A thing too wide for the
// left column gets a line to itself.
func (h *helpText) row(what, does string) {
	lines := wrap(does, helpWidth-2-labelWidth)
	if len(what) >= labelWidth {
		h.line("  " + what)
		what = ""
	}
	for i, l := range lines {
		left := ""
		if i == 0 {
			left = what
		}
		h.line(strings.TrimRight("  "+left+strings.Repeat(" ", labelWidth-len(left))+l, " "))
	}
}

// wrap breaks text into lines of at most width columns, at spaces. A
// newline in text starts a new line, and an empty line stays empty. A word
// wider than width gets a line to itself.
func wrap(text string, width int) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		cur := words[0]
		for _, w := range words[1:] {
			if len(cur)+1+len(w) > width {
				lines = append(lines, cur)
				cur = w
				continue
			}
			cur += " " + w
		}
		lines = append(lines, cur)
	}
	return lines
}

// notBuiltMark is what help adds to a command or flag that is not built.
const notBuiltMark = "(Not built yet.)"

// render writes a command's help.
func (t *Table) render(c *command) string {
	h := &helpText{}
	if c == t.root {
		h.line(c.summary)
		h.heading("Usage")
		h.item("gnosh [flags] [prompt]",
			"Open a session in this folder. A prompt, if there is one, is sent as its first message.")
		h.item("gnosh -p [flags] [prompt]",
			"Answer one prompt and exit: print mode. Text piped in goes before the prompt.")
		h.item("gnosh <command> [flags] [arguments]",
			`Run one of the commands below. "gnosh <command> --help" explains it.`)
	} else {
		h.line(c.title())
		h.line("")
		text := c.summary
		if c.about != "" {
			text += " " + c.about
		}
		h.para(2, text)
		if len(c.aliases) > 0 {
			var also []string
			for _, a := range c.aliases {
				also = append(also, c.parent.title()+" "+a)
			}
			h.para(2, "Also typed: "+oneOf(also, "or")+".")
		}
		switch {
		case c.built():
		case c.needSub:
			h.line("")
			h.para(2, "None of these commands is built yet in this build of gnosh: typing one stops with a message that says so.")
		default:
			h.line("")
			h.para(2, "Not built yet in this build of gnosh: typing it stops with a message that says so.")
		}
		h.heading("Usage")
		h.line("  " + usage(c))
	}

	if len(c.subs) > 0 {
		h.heading("Commands")
		for _, s := range c.subs {
			does := s.summary
			if !s.built() {
				does += " " + notBuiltMark
			}
			h.row(label(s), does)
		}
	}

	if c.session {
		for _, group := range groupOrder {
			first := true
			for _, f := range c.flags {
				if f.group != group {
					continue
				}
				if first {
					h.heading(group)
					first = false
				}
				h.item(flagLine(f), flagHelp(f))
			}
		}
		h.heading("Reading the command line")
		for i, note := range sessionNotes(c) {
			if i > 0 {
				h.line("")
			}
			h.para(2, note)
		}
	} else if len(c.flags) > 0 {
		h.heading("Flags")
		for _, f := range c.flags {
			h.item(flagLine(f), flagHelp(f))
		}
	}
	return h.b.String()
}

// usage returns the line that shows how a command is typed.
func usage(c *command) string {
	if c.needSub {
		return c.title() + " <command> [flags] [arguments]"
	}
	return strings.TrimSpace(c.title() + " [flags] " + c.argsUsage)
}

// label returns a command's entry in a list of commands: its name and, in
// short, what follows it.
func label(c *command) string {
	args := c.argsUsage
	if c.argsShort != "" {
		args = c.argsShort
	}
	return strings.TrimSpace(c.name + " " + args)
}

// flagLine returns a flag's spellings and the shape of its value, as help
// lists them: "-r, --resume [session]". A letter that is the flag's only in
// print mode is not among them: the flag's own help says so.
func flagLine(f *flag) string {
	var names []string
	for _, r := range f.short {
		names = append(names, "-"+string(r))
	}
	for _, l := range f.long {
		names = append(names, "--"+l)
	}
	line := strings.Join(names, ", ")
	switch f.kind {
	case kindText, kindList:
		line += " <" + f.arg + ">"
	case kindOptional:
		line += " [" + f.arg + "]"
	case kindAttached:
		line += "[=" + f.arg + "]"
	}
	return line
}

// flagHelp returns what help says a flag does, with a mark when the flag,
// or some of its values, are not built.
func flagHelp(f *flag) string {
	text := f.help
	if f.status == NotBuilt {
		return text + " " + notBuiltMark
	}
	var missing []string
	for _, ch := range f.choices {
		if ch.refuse == "" && ch.means == "" && ch.status == NotBuilt {
			missing = append(missing, ch.name)
		}
	}
	if len(missing) > 0 {
		text += " (Not built yet: " + strings.Join(missing, ", ") + ".)"
	}
	return text
}

// sessionNotes returns the paragraphs that end the help of `gnosh`: how the
// line is read, where that is not obvious.
func sessionNotes(c *command) []string {
	var lists []string
	for _, f := range c.flags {
		if f.kind == kindList {
			lists = append(lists, f.main())
		}
	}
	return []string{
		"Flags go before or after the prompt, in any order. Everything after a lone -- is the prompt, even if it begins with a dash.",
		"A list flag (" + strings.Join(lists, ", ") + ") takes the argument after it, and then each argument after that up to the next flag, for as long as the argument has no space in it (spaces inside parentheses do not count). So a prompt of two words or more ends a list (gnosh stops to ask if that prompt reads like more tools), but a one-word prompt right after a list would join it: put the prompt first, put -- before it, or attach the list with an equals sign, as in --tools=Read,Edit.",
		`In a list of tools or rules, commas and spaces separate the items, but not inside parentheses: "Bash(git` + nbsp + `diff` + nbsp + `*),Edit" is two rules.`,
		"A value that begins with a dash is attached with an equals sign: --name=-draft. A flag typed twice counts the last time, except list flags, which add up. One-letter flags can share a dash (-yC); one that takes a value goes last, and its value is the next argument.",
		"A first word that is one of the commands above is that command, not a prompt. To send such a word as a prompt, put -- before it, or use -p.",
	}
}
