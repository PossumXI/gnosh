package text

import (
	"strings"
	"testing"
)

// The lines the benchmarks measure: what a terminal interface redraws.
const (
	benchASCII  = "func (m Metrics) Width(s string) int { // the width of every line, on every redraw, all ASCII"
	benchCJK    = "日本語のテキストと한국어 텍스트와中文文本が混ざった行です。ターミナルの幅を数えます。"
	benchEmoji  = "status: 👍🏽 done 👨\u200d👩\u200d👧\u200d👦 family 🇺🇸🇰🇷 flags 1\ufe0f\u20e3 keycap ❤\ufe0f heart e\u0301 accents"
	benchStyled = "\x1b[1;38;2;255;95;215mgnosh\x1b[0m \x1b[2m~/src/project\x1b[0m \x1b[32m+12\x1b[0m \x1b[31m-3\x1b[0m " +
		"\x1b]8;;https://example.com/docs\x1b\\\x1b[4mdocs\x1b[0m\x1b]8;;\x1b\\ \x1b[38;5;244mmodel: qwen · 12.4k tokens · 3 tools\x1b[0m"
	benchProse = "The quick brown fox jumps over the lazy dog, and then, having thought better of it, " +
		"turns around and jumps back again; the dog, well-known for its patience, does not so much as open an eye. "
)

func BenchmarkWidth(b *testing.B) {
	cases := []struct{ name, in string }{
		{"ascii", benchASCII},
		{"cjk", benchCJK},
		{"emoji", benchEmoji},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(c.in)))
			for b.Loop() {
				Width(c.in)
			}
		})
	}
}

func BenchmarkVisibleWidth(b *testing.B) {
	cases := []struct{ name, in string }{
		{"plain", benchASCII},
		{"styled", benchStyled},
		{"styled-cjk", "\x1b[33m" + benchCJK + "\x1b[0m"},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(c.in)))
			for b.Loop() {
				VisibleWidth(c.in)
			}
		})
	}
}

func BenchmarkGraphemes(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchEmoji)))
	for b.Loop() {
		for range Graphemes(benchEmoji) {
		}
	}
}

func BenchmarkStrip(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchStyled)))
	for b.Loop() {
		Strip(benchStyled)
	}
}

func BenchmarkTruncate(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchStyled)))
	for b.Loop() {
		Truncate(benchStyled, 40, "…")
	}
}

func BenchmarkWrap(b *testing.B) {
	prose := strings.Repeat(benchProse, 10)
	styled := "\x1b[3m" + strings.Repeat("\x1b[1mbold\x1b[22m and plain words, \x1b[38;5;206mcoloured\x1b[39m ones too. ", 30) + "\x1b[0m"
	cases := []struct {
		name string
		in   string
		opts WrapOptions
	}{
		{"prose", prose, WrapOptions{}},
		{"prose-list", prose, WrapOptions{First: "- ", Rest: "  "}},
		{"styled", styled, WrapOptions{}},
		{"cjk", strings.Repeat(benchCJK, 10), WrapOptions{}},
		{"short", "a short line that fits", WrapOptions{}},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(c.in)))
			for b.Loop() {
				Wrap(c.in, 80, c.opts)
			}
		})
	}
}

func BenchmarkStyleRender(b *testing.B) {
	style := Style{}.Fg(RGB(0xff, 0x5f, 0xd7)).Bold().In(ANSI256)
	b.ReportAllocs()
	for b.Loop() {
		style.Render("gnosh")
	}
}

func BenchmarkGradient(b *testing.B) {
	purple, pink := RGB(95, 0, 255), RGB(255, 95, 215)
	for _, profile := range []Profile{TrueColor, ANSI256} {
		opts := GradientOptions{Style: Style{}.In(profile)}
		b.Run(profile.String(), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				opts.Render("gnosh: an AI harness that thinks in Gno", purple, pink)
			}
		})
	}
}

// TestHotPathsDoNotAllocate holds the measuring functions to their promise.
func TestHotPathsDoNotAllocate(t *testing.T) {
	tests := []struct {
		name string
		fn   func()
	}{
		{"Width, ascii", func() { Width(benchASCII) }},
		{"Width, cjk", func() { Width(benchCJK) }},
		{"Width, emoji", func() { Width(benchEmoji) }},
		{"Width, wide ambiguous", func() { Metrics{AmbiguousWide: true}.Width(benchEmoji) }},
		{"VisibleWidth, plain", func() { VisibleWidth(benchASCII) }},
		{"VisibleWidth, styled", func() { VisibleWidth(benchStyled) }},
		{"VisibleWidth, sequence inside a cluster", func() { VisibleWidth("e\x1b[31m\u0301 👍\x1b[0m🏽") }},
		{"NextGrapheme", func() {
			for rest := benchEmoji; rest != ""; {
				_, _, rest = NextGrapheme(rest)
			}
		}},
		{"Graphemes", func() {
			for range Graphemes(benchEmoji) {
			}
		}},
		{"NextToken", func() {
			for rest := benchStyled; rest != ""; {
				_, rest = NextToken(rest)
			}
		}},
		{"Tokens", func() {
			for range Tokens(benchStyled) {
			}
		}},
		{"Strip, nothing to strip", func() { Strip(benchASCII) }},
		{"Truncate, fits", func() { Truncate(benchStyled, 200, "…") }},
		{"PadRight, wide enough", func() { PadRight(benchASCII, 10) }},
		{"ExpandTabs, no tabs", func() { ExpandTabs(benchASCII, 8, 0) }},
		{"Render, no style", func() { Style{}.Render(benchASCII) }},
		{"Render, no colour", func() { Style{}.Bold().In(NoColor).Render(benchASCII) }},
	}
	for _, tt := range tests {
		if allocs := testing.AllocsPerRun(50, tt.fn); allocs != 0 {
			t.Errorf("%s: %.0f allocations, want none", tt.name, allocs)
		}
	}
}
