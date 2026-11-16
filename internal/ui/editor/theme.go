package editor

import "github.com/gnosh-ai/gnosh/internal/text"

// Theme is the look of the box: the marks it draws and the style of each
// kind of thing in it. Nothing depends on colour alone: the marks say
// which mode the box is in, which candidate is chosen and where text is
// hidden, so the box reads the same under text.NoColor.
type Theme struct {
	// Prompt stands before the first row of a prompt. ShellPrompt and
	// CommandPrompt stand there instead when the first character is "!"
	// or "/": that character is drawn as the prefix, not in the text.
	// Rows after the first are indented to match. The three should be
	// equally wide; the shorter are padded.
	Prompt, ShellPrompt, CommandPrompt string
	// MoreAbove and MoreBelow mark, at the left edge, that rows are
	// hidden above the first row shown or below the last. MoreBoth is
	// for a single row with rows hidden both ways.
	MoreAbove, MoreBelow, MoreBoth string
	// Pointer marks the candidate that is chosen.
	Pointer string

	// PromptStyle, ShellStyle and CommandStyle are the looks of the
	// three prefixes.
	PromptStyle, ShellStyle, CommandStyle text.Style
	// Text is the look of what is typed.
	Text text.Style
	// Placeholder is the look of the words shown in an empty box.
	Placeholder text.Style
	// Paste is the look of a collapsed paste.
	Paste text.Style
	// Control is the look of what stands in for a control character or
	// any other character with no cell of its own.
	Control text.Style
	// More is the look of the marks for hidden rows.
	More text.Style
	// Locked is the look of the whole box while it is locked.
	Locked text.Style
	// Candidate, Selected and Detail are the looks of a candidate, of
	// the one chosen, and of the few words beside each.
	Candidate, Selected, Detail text.Style
	// SearchLabel is the look of the words before a history search,
	// Match of the part of the entry that matched, and Note of what is
	// said after the search and of the line of keys under it.
	SearchLabel, Match, Note text.Style
	// Selection is the look of the text a VISUAL selection of the vim
	// keys holds.
	Selection text.Style
}

// DefaultTheme is gnosh's look for the box, written for the colour
// profile p: a hot pink prompt mark, gold for a shell line, mint for a
// command, purple for collapsed pastes and the marks for hidden rows, and
// plain text for what is typed. Under text.NoColor it is marks alone.
func DefaultTheme(p text.Profile) Theme {
	// gnosh's mint and gold as the specs give them, and a purple and a
	// hot pink chosen here until Jae picks his own.
	var (
		purple = text.RGB(0xA8, 0x78, 0xFF)
		pink   = text.RGB(0xFF, 0x4F, 0xB6)
		mint   = text.RGB(0x00, 0xFF, 0xB2)
		gold   = text.RGB(0xFB, 0xD2, 0x37)
	)
	plain := text.Style{}.In(p)
	return Theme{
		Prompt:        "> ",
		ShellPrompt:   "! ",
		CommandPrompt: " /",
		MoreAbove:     "↑",
		MoreBelow:     "↓",
		MoreBoth:      "↕",
		Pointer:       ">",

		PromptStyle:  plain.Fg(pink).Bold(),
		ShellStyle:   plain.Fg(gold).Bold(),
		CommandStyle: plain.Fg(mint).Bold(),
		Text:         plain,
		Placeholder:  plain.Dim(),
		Paste:        plain.Fg(purple),
		Control:      plain.Fg(gold),
		More:         plain.Fg(purple),
		Locked:       plain.Dim(),
		Candidate:    plain,
		Selected:     plain.Fg(pink).Bold(),
		Detail:       plain.Dim(),
		SearchLabel:  plain.Fg(purple),
		Match:        plain.Fg(pink).Underline(),
		Note:         plain.Dim(),
		Selection:    plain.Reverse(),
	}
}
