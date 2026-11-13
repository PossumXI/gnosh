// Package instructions finds, loads and presents the instruction files that
// a person and a project keep for the model: the files other harnesses call
// memory files, such as CLAUDE.md and AGENTS.md. In gnosh "mem" is the
// model's own plan, kept in Gno, so this package says "instructions"
// throughout.
//
// The package reads files and returns values. It prints nothing, asks
// nothing (the caller supplies the function that approves an import from
// outside the project), and knows no file name of its own: every name and
// place is in Options.
//
// # What is read, and in what order
//
// Load reads what is in effect from the start of a session, broadest
// first, so that what is nearest to the work is read last:
//
//  1. The person's own files and folders (Options.UserFiles), in the order
//     given. A folder gives every ".md" file under it, as rules.
//  2. Every directory from the top of the file system down to the working
//     directory. In each one:
//     a. the file under each of Options.ProjectNames, and after each the
//     file of the same name in each of Options.DotDirs;
//     b. the files under Options.FallbackNames, laid out the same way,
//     when Options.Fallback calls for them;
//     c. the rules in the "rules" directory of each dot directory, in
//     name order, sub-directories included;
//     d. the person's private files (Options.LocalNames).
//  3. The project's extra files and folders (Options.ProjectFiles).
//
// Steps 2 and 3 happen only if Options.Trusted is set. Without it a folder
// gets nothing of its own: only the person's files are read.
//
// Files are joined, never overridden. A file is loaded once, however many
// names, links and imports lead to it: files are told apart by their path
// with every symbolic link followed.
//
// Two kinds of file wait until the model touches a file they are about, and
// are handed over by Set.ForPath:
//
//   - Files in a directory below the working directory. The first time a
//     file in such a directory is touched, ForPath returns what starting the
//     session in that directory would have added, for it and for each
//     directory between it and the working directory.
//   - Rules whose frontmatter lists path patterns. ForPath returns a rule
//     the first time a touched file matches one of its patterns.
//
// Each is returned once. Set.Forget makes them returnable again, for when
// the part of the conversation that held them has been cut away.
//
// # A worked example
//
// Take this tree, a session started in /work/shop, and these options:
// UserFiles /home/ana/.config/gnosh/GNOSH.md and the folder
// /home/ana/.config/gnosh/rules, ProjectNames GNOSH.md, LocalNames
// CLAUDE.local.md, FallbackNames AGENTS.md, DotDirs .gnosh, Trusted.
//
//	/home/ana/.config/gnosh/GNOSH.md        "Be brief."
//	/home/ana/.config/gnosh/rules/tone.md   "Plain words."
//	/home/ana/notes/go.md
//	/work/GNOSH.md                          (above the project)
//	/work/shop/GNOSH.md                     "Build with @docs/build.md. My Go notes: @~/notes/go.md"
//	/work/shop/CLAUDE.local.md
//	/work/shop/docs/build.md
//	/work/shop/.gnosh/rules/tests.md        (no frontmatter)
//	/work/shop/.gnosh/rules/api.md          (frontmatter: paths: "api/**/*.go")
//	/work/shop/api/GNOSH.md
//	/work/shop/api/server.go
//	/work/shop/web/AGENTS.md
//	/work/shop/web/app.ts
//
// Load returns, in this order:
//
//	/home/ana/.config/gnosh/GNOSH.md        the person's own
//	/home/ana/.config/gnosh/rules/tone.md   the person's own rule
//	/work/GNOSH.md                          project, from a directory above
//	/work/shop/GNOSH.md                     project
//	/work/shop/docs/build.md                imported by /work/shop/GNOSH.md
//	/work/shop/.gnosh/rules/tests.md        project rule
//	/work/shop/CLAUDE.local.md              the person's private notes
//
// and one warning: /work/shop/GNOSH.md imports /home/ana/notes/go.md, which
// is outside the project, and nobody approved it. With an
// Options.ApproveImport that says yes, go.md is loaded right after
// build.md.
//
// When the model reads /work/shop/api/server.go, ForPath returns
// /work/shop/api/GNOSH.md (the directory's own file) and then
// /work/shop/.gnosh/rules/api.md (its pattern matches). A second file in
// api brings nothing more.
//
// When the model reads /work/shop/web/app.ts, ForPath returns nothing:
// under the default policy, FallbackInstead, an AGENTS.md is read only in a
// project that keeps no GNOSH.md, and this one keeps one. Under
// FallbackAlso it would be returned.
//
// # Imports
//
// A file brings in another by writing "@path" in its text: relative to the
// file itself, from the home directory with "~/", or absolute. The import
// stays in the text as written, and the imported file follows its importer
// in the list as a file of its own, with its own imports after it. Imports
// may follow one another Options.MaxDepth deep. A file that is imported
// twice, or imports itself, is still loaded once.
//
// An "@" counts only at the start of a line or after a space, "(" or "[",
// so an e-mail address is not an import, and never inside a fenced code
// block or a code span, which is how to write a path that is not one. A
// backslash before a space keeps the space in the path. A single word with
// no slash or dot ("@alice", "@Override") is an import if a file of that
// name is there and plain text if not. Any other import that leads nowhere
// is reported.
//
// What the person's own files import is read without question. A file of
// the project may import only from inside the project root; anything else
// goes to Options.ApproveImport, and is refused when there is nobody to
// ask. The same holds for a file of the project that turns out to be a
// link to somewhere outside it: a repository can hold a link named
// CLAUDE.md that points at a private key. A rules directory that is such a
// link is asked about once, as a whole, and is not searched before the
// answer is yes: the link could point at the top of the disk.
//
// # Rules
//
// A rule is a ".md" file in a rules directory. It may open with
// frontmatter, which is taken off before the text is used:
//
//	---
//	paths:
//	  - "src/api/**/*.ts"
//	  - "*.{md,txt}"
//	---
//
// A rule with no "paths" is in effect from the start. A rule with "paths"
// waits for a file that matches. The patterns are read like the lines of an
// ignore file, against the directory that holds the rule's dot directory
// (for one of the person's own rules, against the working directory):
// "*.md" matches at any depth, "/*.md" only at the top, "src/**" everything
// under src, and a pattern that matches a directory matches what is in it.
// Frontmatter that cannot be read, or a pattern that cannot, is reported,
// and the rule is then loaded for every file rather than dropped.
//
// # What is done to the text
//
// Bytes that are not UTF-8 are replaced (and reported); a file with a zero
// byte is taken for something other than text and skipped; line ends become
// "\n". HTML comments that stand on lines of their own are removed, since
// they are notes for the people who keep the file; comments inside code
// blocks stay. A file over Options.MaxBytes is skipped. A late file over
// Options.LateMaxChars is cut, and Render says so to the model.
//
// # Directories above the project
//
// A directory above the project root may be shared with other accounts
// (/tmp is). A file there is read only if it belongs to the person or to
// root, and so does everything on the way to it; a rules directory there
// is searched on the same condition. A stranger's file there cannot give
// the model instructions, and cannot switch a project's fallback files off
// either.
//
// # Limits
//
// Paths are compared as they are spelled once links are followed. On a file
// system that ignores letter case, the same file reached under two
// spellings is taken for two files.
//
// # Handing the text to the model
//
// Set.Render lays out the files from the start under a short preamble, each
// introduced by one line with its path and what it is. RenderFiles does the
// same for files ForPath returned. The same files always give the same
// text. Set.Changed tells when the files on disk are no longer the ones
// that were read, so that a caller can offer to load again.
package instructions
