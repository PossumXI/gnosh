package ui

import (
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// The theme option: auto follows what the terminal said of its background,
// dark and light say it whatever the terminal said, and ansi takes the
// terminal's own sixteen colours.
func TestThemeFor(t *testing.T) {
	darkReply, lightReply := DefaultPalette(text.TrueColor).Reply, DefaultPalette(text.TrueColor).OnLight(text.TrueColor).Reply
	for _, c := range []struct {
		name  string
		light bool
		want  text.Color
	}{
		{"auto", false, darkReply},
		{"auto", true, lightReply},
		{"", true, lightReply},
		{"dark", true, darkReply},
		{"light", false, lightReply},
		{"ansi", false, text.BrightGreen},
		{"ansi", true, text.Green},
	} {
		if got := themeFor(c.name, text.TrueColor, c.light).Palette.Reply; got != c.want {
			t.Errorf("themeFor(%q, light %v): the reply is %v, want %v", c.name, c.light, got, c.want)
		}
	}
	if got := themeFor("ansi", text.NoColor, false).Palette; got != DefaultPalette(text.NoColor) {
		t.Errorf("ansi without colours: %+v", got)
	}
}

// /theme and /config theme= put the colours in force at once, and keep the
// colour /color gave the prompt mark.
func TestThemeCommandChangesColours(t *testing.T) {
	g := newRig(t, 100, 24, text.TrueColor)
	g.s.themed, g.s.profile, g.s.light = true, text.TrueColor, false
	g.s.setTheme("auto")
	if g.s.th.Palette.Reply != DefaultPalette(text.TrueColor).Reply {
		t.Fatalf("auto on a dark terminal gave %+v", g.s.th.Palette.Reply)
	}
	g.s.ran("/theme light", app.CommandResult{Name: "theme", Handled: true}, nil)
	if g.s.th.Palette.Reply != DefaultPalette(text.TrueColor).OnLight(text.TrueColor).Reply {
		t.Errorf("/theme light gave the reply %v", g.s.th.Palette.Reply)
	}
	g.s.ran("/config theme=ansi", app.CommandResult{Name: "config", Handled: true}, nil)
	if g.s.th.Palette.Reply != text.BrightGreen {
		t.Errorf("/config theme=ansi gave the reply %v", g.s.th.Palette.Reply)
	}
	// A theme a test gave the interface is kept.
	g.s.themed = false
	g.s.setTheme("light")
	if g.s.th.Palette.Reply != text.BrightGreen {
		t.Errorf("a given theme was changed")
	}
}
