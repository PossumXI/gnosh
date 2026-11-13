package transcript

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DataDir returns gnosh's data directory: $XDG_DATA_HOME/gnosh when that
// variable holds an absolute path, and otherwise home/.local/share/gnosh.
// getenv looks up an environment variable (os.Getenv) and home is the
// person's home directory.
func DataDir(getenv func(string) string, home string) (string, error) {
	if getenv != nil {
		if xdg := getenv("XDG_DATA_HOME"); filepath.IsAbs(xdg) {
			return filepath.Join(xdg, "gnosh"), nil
		}
	}
	if !filepath.IsAbs(home) {
		return "", errors.New("transcript: cannot place the data directory: XDG_DATA_HOME is not set to an absolute path and the home directory is not known")
	}
	return filepath.Join(home, ".local", "share", "gnosh"), nil
}

// maxSlug is the longest slug, in bytes. File names may be 255 bytes long
// on the systems gnosh runs on; this leaves room.
const maxSlug = 200

// Slug turns a working directory into the name of the directory that holds
// its sessions. The rule:
//
//  1. The path is cleaned (filepath.Clean).
//  2. Every character that is not a letter, a digit, '.', '_' or '-' becomes
//     '-'. That covers the path separators, spaces and control characters.
//     Letters and digits of any script are kept. A byte that is not valid
//     UTF-8 becomes '-'.
//  3. A slug longer than 200 bytes is cut to its first 183 bytes (at a
//     character boundary) and ends in '-' and the first 16 hex digits of the
//     SHA-256 of the cleaned path, so two long paths that begin alike still
//     get directories of their own.
//
// So /Users/jae/src/gnosh becomes -Users-jae-src-gnosh. Two different paths
// can share a slug (/a/b-c and /a/b/c, or two names that differ only in
// case on a file system that ignores case). That is harmless: every entry
// records its working directory, and Store.List shows only the sessions
// that were started in the directory asked for.
func Slug(cwd string) string {
	clean := filepath.Clean(cwd)
	var b strings.Builder
	b.Grow(len(clean))
	for i := 0; i < len(clean); {
		r, size := utf8.DecodeRuneInString(clean[i:])
		i += size
		switch {
		case r == utf8.RuneError && size <= 1:
			b.WriteByte('-')
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	slug := b.String()
	if len(slug) > maxSlug {
		cut := maxSlug - 17
		for cut > 0 && !utf8.RuneStart(slug[cut]) {
			cut--
		}
		sum := sha256.Sum256([]byte(clean))
		slug = slug[:cut] + "-" + hex.EncodeToString(sum[:8])
	}
	// A slug is used as one path element, so it must not be one of the two
	// names that mean a directory itself or its parent.
	if slug == "." || slug == ".." || slug == "" {
		slug = "-" + slug
	}
	return slug
}
