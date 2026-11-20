package ui

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/shellquote"
	"github.com/gnosh-ai/gnosh/internal/term"
)

// This file is pictures in the prompt box, as Claude Code takes them
// (spec/cc/interactive.md 2.7). Ctrl+V pastes the picture on the clipboard
// of the machine gnosh runs on: the session keeps it, and the box mentions
// it as @image-N. A paste that is nothing but the paths of picture files,
// as a terminal pastes files dragged into it, becomes @ mentions of them.
// Either way the picture goes with the prompt as a picture.

// picturePaster is a session that takes a picture off the clipboard and
// keeps it (app's controls.PastePicture).
type picturePaster interface {
	PastePicture(ctx context.Context) (string, error)
}

// pastePicture is Ctrl+V in the prompt box.
func (s *session) pastePicture() {
	pp, ok := s.c.(picturePaster)
	if !ok {
		s.flash("Pasting a picture (Ctrl+V) is not joined to this build.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	name, err := pp.PastePicture(ctx)
	switch {
	case errors.Is(err, app.ErrNoClipboard):
		s.flash("Ctrl+V pastes a picture from the clipboard of the machine gnosh runs on, which it cannot read here (over ssh the clipboard is your own machine's). Drag the picture's file in, or mention it with @.")
	case errors.Is(err, app.ErrNoPicture):
		s.flash("The clipboard holds no picture. Text is pasted with the terminal's own paste.")
	case err != nil:
		s.flash("The picture could not be pasted: " + err.Error() + ".")
	default:
		s.ed.Handle(term.Paste{Text: "@" + name + " "})
		s.flash("The picture goes with the prompt as @" + name + ".")
	}
}

// pictureEndings are the endings of the picture files a paste of paths
// may name.
var pictureEndings = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}

// picturePaths turns a paste that is nothing but the paths of picture
// files (as a terminal pastes files dragged into it: the spaces in them
// escaped, or quoted, or as file:// addresses) into @ mentions of them,
// and reports whether it was one.
func (s *session) picturePaths(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 8192 || strings.Contains(text, "\n") && strings.Count(text, "\n") > 20 {
		return "", false
	}
	words, err := shellquote.Split(text)
	if err != nil || len(words) == 0 || len(words) > 20 {
		return "", false
	}
	out := make([]string, 0, len(words))
	for _, w := range words {
		p := w
		if strings.HasPrefix(p, "file://") {
			u, err := url.Parse(p)
			if err != nil {
				return "", false
			}
			p = u.Path
		}
		if strings.HasPrefix(p, "~/") && s.home != "" {
			p = filepath.Join(s.home, p[2:])
		}
		if !filepath.IsAbs(p) || !pictureEndings[strings.ToLower(filepath.Ext(p))] {
			return "", false
		}
		if info, err := os.Stat(p); err != nil || !info.Mode().IsRegular() {
			return "", false
		}
		shown := p
		if s.home != "" && strings.HasPrefix(p, s.home+string(filepath.Separator)) {
			shown = "~/" + p[len(s.home)+1:]
		}
		if strings.ContainsAny(shown, "\"\n") {
			return "", false
		}
		if strings.ContainsAny(shown, " \t") {
			shown = `"` + shown + `"`
		}
		out = append(out, "@"+shown)
	}
	return strings.Join(out, " ") + " ", true
}
