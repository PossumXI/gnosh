package permission

import (
	"path"
	"strings"
)

// This file is the table of commands that only read. A command on it runs
// without a question when every flag it is given is one listed here as
// keeping it read-only, and everything it reads lies in the working
// directories. The table is deliberately short and each entry lists what
// is known to be safe: a flag that is not listed means "ask".

// opKind says what a command's operands (its non-flag arguments) are.
type opKind uint8

const (
	// opsPaths: every operand is a file or directory that is read.
	opsPaths opKind = iota
	// opsWords: operands are not files (echo's text, tr's sets).
	opsWords
	// opsPattern: the first operand is a pattern, unless a flag supplied
	// one; the rest are files.
	opsPattern
	// opsNone: no operands at all.
	opsNone
)

// roSpec describes one read-only command.
type roSpec struct {
	// short lists single-letter flags that take no value.
	short string
	// valued lists single-letter flags that take a value, attached or as
	// the next word.
	valued string
	// long lists long flags, separated by spaces. "--name" takes no
	// value; "--name=" takes one, attached or as the next word.
	long string
	// ops says what the operands are; max is the most that are allowed
	// (0 means any number).
	ops opKind
	max int
	// digits lets -5 stand for a count, as head and tail take it.
	digits bool
	// noGlob: an unquoted file name pattern among the operands asks,
	// because the command has flags that write or run things and a
	// matched file name could be read as one.
	noGlob bool
	// patFlags are the valued flags that supply the pattern (grep -e).
	patFlags string
	// pathFlags are the valued flags whose value is a file that is read.
	pathFlags string
	// anyArgs: the arguments are only text to the command (echo), so
	// nothing that looks like a flag needs to be known.
	anyArgs bool
}

// roTable is the read-only set. Commands with rules of their own (cd,
// git, find, sed, date, command, test, [) are handled in readOnlyFacts.
var roTable = map[string]roSpec{
	"ls": {short: "aAbBcCdFfgGhiklmnopqrRsStuUvxX1@eO",
		long: "--all --almost-all --color --color= --human-readable --recursive --reverse --size --classify --directory --inode --numeric-uid-gid --no-group --group-directories-first --full-time --time-style= --sort= --time= --format= --indicator-style= --si --block-size= --hide= --ignore= --literal --quote-name --escape --file-type --width= --author --kibibytes --zero"},
	"cat":  {short: "beEnstTuvA", long: "--number --number-nonblank --squeeze-blank --show-all --show-ends --show-tabs --show-nonprinting"},
	"head": {short: "qvz", valued: "nc", digits: true, long: "--lines= --bytes= --quiet --silent --verbose --zero-terminated"},
	"tail": {short: "qvzfFr", valued: "ncbs", digits: true, long: "--lines= --bytes= --quiet --silent --verbose --zero-terminated --follow --follow= --retry --pid= --sleep-interval="},
	"wc":   {short: "clmwL", long: "--bytes --chars --lines --words --max-line-length"},
	"nl":   {short: "p", valued: "bnwsvihfdl", long: "--body-numbering= --number-format= --number-width= --number-separator= --starting-line-number= --line-increment="},
	"grep": {short: "abcEFGHhIiLlnoPqsTUvwxZzr", valued: "efmABC", patFlags: "ef", pathFlags: "f", ops: opsPattern,
		long: "--include= --exclude= --exclude-dir= --color --color= --colour --colour= --line-number --ignore-case --no-messages --count --files-with-matches --files-without-match --only-matching --quiet --silent --invert-match --word-regexp --line-regexp --extended-regexp --fixed-strings --basic-regexp --perl-regexp --recursive --with-filename --no-filename --null --text --binary-files= --max-count= --after-context= --before-context= --context= --regexp= --line-buffered --byte-offset --initial-tab --null-data"},
	"rg": {short: "niSslcwxvFoHINUuP0", valued: "egtTABCmjM", patFlags: "e", ops: opsPattern,
		long: "--glob= --type= --type-not= --files --hidden --no-ignore --no-ignore-vcs --line-number --no-line-number --no-heading --heading --color= --json --count --count-matches --files-with-matches --files-without-match --ignore-case --smart-case --case-sensitive --fixed-strings --word-regexp --line-regexp --invert-match --only-matching --multiline --multiline-dotall --max-count= --context= --after-context= --before-context= --max-depth= --sort= --sortr= --trim --column --no-messages --pcre2 --stats --vimgrep --null --max-columns= --regexp= --threads= --with-filename --no-filename --text --unrestricted --no-config --passthru"},
	"which": {short: "as", ops: opsWords},
	"type":  {short: "at", ops: opsWords},
	"diff": {short: "ucqsiwbBaNytdEZpT", valued: "UCWILx", max: 2,
		long: "--color --color= --unified --unified= --context --context= --brief --ignore-case --ignore-all-space --ignore-space-change --ignore-blank-lines --ignore-trailing-space --ignore-tab-expansion --side-by-side --width= --label= --exclude= --strip-trailing-cr --minimal --text --new-file --report-identical-files --no-dereference --suppress-common-lines --expand-tabs --show-c-function --ignore-matching-lines="},
	"stat":  {short: "xlnqFs", valued: "c", long: "--format= --printf= --terse"},
	"du":    {short: "achkmsxgbP", valued: "dt", long: "--max-depth= --human-readable --summarize --total --all --apparent-size --block-size= --si --exclude= --one-file-system --bytes --threshold= --time --no-dereference"},
	"pwd":   {short: "LP", ops: opsNone},
	"echo":  {ops: opsWords, anyArgs: true},
	"true":  {ops: opsWords, anyArgs: true},
	"false": {ops: opsWords, anyArgs: true},
	"sort": {short: "rnufbdghiMVRszcCm", valued: "kt", noGlob: true,
		long: "--reverse --numeric-sort --unique --ignore-case --ignore-leading-blanks --dictionary-order --general-numeric-sort --human-numeric-sort --month-sort --version-sort --stable --zero-terminated --check --check= --merge --key= --field-separator="},
	"uniq":     {short: "cduizD", valued: "fsw", max: 1, long: "--count --repeated --unique --ignore-case --zero-terminated --skip-fields= --skip-chars= --check-chars="},
	"cut":      {short: "sz", valued: "dfcb", long: "--delimiter= --fields= --characters= --bytes= --only-delimited --complement --output-delimiter= --zero-terminated"},
	"tr":       {short: "dscCt", ops: opsWords, long: "--delete --squeeze-repeats --complement --truncate-set1"},
	"basename": {short: "az", valued: "s", ops: opsWords, long: "--multiple --suffix= --zero"},
	"dirname":  {short: "z", ops: opsWords, long: "--zero"},
	"realpath": {short: "emqsz", long: "--canonicalize-existing --canonicalize-missing --quiet --strip --no-symlinks --zero"},
	"readlink": {short: "fnemqsvz", long: "--canonicalize --canonicalize-existing --canonicalize-missing --no-newline --quiet --silent --verbose --zero"},
	"file":     {short: "bihkINnprsz0", long: "--brief --mime --mime-type --mime-encoding --no-dereference --keep-going --no-buffer --preserve-date --raw --special-files --print0"},
	"whoami":   {ops: opsNone},
	"uname":    {short: "asnrvmpio", ops: opsNone, long: "--all --kernel-name --nodename --kernel-release --kernel-version --machine --processor --hardware-platform --operating-system"},
	"seq":      {short: "w", valued: "sf", ops: opsWords, long: "--equal-width --separator= --format="},
}

// cmdFacts is what is known about one simple command after its wrappers
// are taken off.
type cmdFacts struct {
	// name is the program's name; plain says it was written as plain
	// text, with nothing for the shell to expand or unquote.
	name  string
	plain bool
	// readOnly says the command, as written, only reads. why says why
	// not.
	readOnly bool
	why      string
	// fsEdit says the command is one of the file commands acceptEdits
	// lets run (mkdir, touch, rm, rmdir, mv, cp, sed -i) in a form that
	// is fully understood.
	fsEdit bool
	// reads and writes are the operands the command reads and changes.
	reads, writes []word
	// cd is set for a cd command: the directory it changes to.
	cd *word
	// git is set for any git command.
	git bool
}

// parsed is the result of reading a command's arguments against a spec.
type parsed struct {
	ops      []word
	pathVals []word
	sawPat   bool
	bad      string
}

// parseArgs reads args against spec. bad is set to the reason when a flag
// is not one the spec lists, or a word is not static text.
func parseArgs(args []word, spec roSpec) parsed {
	var out parsed
	long := " " + spec.long + " "
	rest := false
	for i := 0; i < len(args); i++ {
		w := args[i]
		if !w.static() {
			out.bad = "an argument is computed when the command runs: " + w.raw
			return out
		}
		if w.brace {
			out.bad = "an argument holds braces the shell may expand: " + w.raw
			return out
		}
		if w.glob && spec.noGlob {
			out.bad = "a file name pattern (" + w.raw + ") is given to a command that has flags that write or run things"
			return out
		}
		s := w.joined()
		switch {
		case rest || spec.anyArgs || s == "-" || !strings.HasPrefix(s, "-") || w.tilde:
			out.ops = append(out.ops, w)
		case strings.ContainsAny(s, " \t\n"):
			out.bad = "a flag that holds white space: " + w.raw
			return out
		case s == "--":
			rest = true
		case strings.HasPrefix(s, "--"):
			name, _, hasVal := strings.Cut(s, "=")
			if name == "--regexp" {
				out.sawPat = true
			}
			switch {
			case hasVal && strings.Contains(long, " "+name+"= "):
			case !hasVal && strings.Contains(long, " "+s+" "):
			case !hasVal && strings.Contains(long, " "+s+"= "):
				i++ // the value is the next word
				if i >= len(args) || !args[i].static() || args[i].glob {
					out.bad = "the flag " + s + " has no plain value"
					return out
				}
			case s == "--help" || s == "--version":
			default:
				out.bad = "the flag " + name + " is not one known to only read"
				return out
			}
		default:
			for j := 1; j < len(s); j++ {
				c := s[j]
				switch {
				case strings.IndexByte(spec.valued, c) >= 0:
					var val word
					if j+1 < len(s) {
						val = word{raw: s[j+1:], parts: []wordPart{{lit: s[j+1:]}}}
					} else {
						i++
						if i >= len(args) || !args[i].static() || args[i].glob {
							out.bad = "the flag -" + string(c) + " has no plain value"
							return out
						}
						val = args[i]
					}
					if strings.IndexByte(spec.patFlags, c) >= 0 {
						out.sawPat = true
					}
					if strings.IndexByte(spec.pathFlags, c) >= 0 {
						out.pathVals = append(out.pathVals, val)
					}
					j = len(s)
				case strings.IndexByte(spec.short, c) >= 0:
				case spec.digits && c >= '0' && c <= '9':
				default:
					out.bad = "the flag -" + string(c) + " is not one known to only read"
					return out
				}
			}
		}
	}
	return out
}

// litsOf returns the literal text of each word.
func litsOf(ws []word) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.lit()
	}
	return out
}

// baseName returns the last element of a program path.
func baseName(s string) string {
	if strings.Contains(s, "/") {
		return path.Base(s)
	}
	return s
}

// plainName reports whether a program name is plain text made of the
// characters program names are made of.
func plainName(w word) bool {
	if w.raw == "[" || w.raw == "[[" {
		return true
	}
	if !w.plain() {
		return false
	}
	s := w.joined()
	if s == "" || s[0] == '-' {
		return false
	}
	if s == "[" {
		return true
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(isNameChar(c) || strings.IndexByte("./+-@%:,", c) >= 0) {
			return false
		}
	}
	return true
}

// facts works out what the command made of words does, as far as this
// package's tables know.
func facts(words []word) cmdFacts {
	var f cmdFacts
	if len(words) == 0 {
		f.readOnly = true
		return f
	}
	w0 := words[0]
	f.plain = plainName(w0)
	if !w0.static() {
		f.why = "the program name is computed when the command runs"
		return f
	}
	f.name = w0.joined()
	args := words[1:]
	if !f.plain {
		f.why = "the program name is not written as plain text"
	}
	// A program reached by a path is not the one the table describes.
	if strings.Contains(f.name, "/") {
		f.name = baseName(f.name)
		fillTargets(&f, args)
		f.git = f.name == "git"
		if f.why == "" {
			f.why = w0.joined() + " is named by its path, so gnosh cannot tell it is the usual " + f.name
		}
		return f
	}
	fillTargets(&f, args)
	if f.why != "" {
		return f
	}
	switch f.name {
	case "cd":
		cdFacts(&f, args)
	case "git":
		f.git = true
		gitFacts(&f, args)
	case "find":
		findFacts(&f, args)
	case "sed":
		sedFacts(&f, args)
	case "date":
		dateFacts(&f, args)
	case "command":
		p := parseArgs(args, roSpec{short: "vV", ops: opsWords})
		if p.bad != "" || len(p.ops) == 0 || len(args) == len(p.ops) {
			f.why = "only \"command -v name\" is known to only read"
		} else {
			f.readOnly = true
		}
	case "test", "[", "[[":
		testFacts(&f, args)
	case "mkdir", "touch", "rm", "rmdir", "mv", "cp":
		fsFacts(&f, args)
	default:
		spec, ok := roTable[f.name]
		if !ok {
			f.why = f.name + " is not on the list of commands that only read"
			return f
		}
		p := parseArgs(args, spec)
		if p.bad != "" {
			f.why = p.bad
			return f
		}
		ops := p.ops
		switch spec.ops {
		case opsNone:
			if len(ops) > 0 {
				f.why = f.name + " is given arguments it does not take when it only reads"
				return f
			}
		case opsWords:
			// The operands are text, not files, except that the shell
			// turns an unquoted pattern into the names of files: echo
			// /etc/* is a listing of /etc.
			var named []word
			for _, o := range ops {
				if o.glob {
					named = append(named, o)
				}
			}
			ops = named
		case opsPattern:
			if !p.sawPat && !hasLong(args, "--files") {
				if len(ops) == 0 {
					f.why = f.name + " has no pattern"
					return f
				}
				ops = ops[1:]
			}
		}
		if spec.max > 0 && len(ops) > spec.max {
			f.why = f.name + " is given more files than it reads"
			return f
		}
		for _, o := range append(ops, p.pathVals...) {
			if o.glob && spec.noGlob {
				f.why = "a file name pattern is given to " + f.name + ", which has flags that write or run things"
				return f
			}
		}
		f.reads = append(ops, p.pathVals...)
		f.readOnly = true
	}
	return f
}

// hasLong reports whether args holds the flag as a word of its own.
func hasLong(args []word, flag string) bool {
	for _, a := range args {
		if a.lit() == flag {
			return true
		}
	}
	return false
}

// fillTargets records the files the well-known file commands read and
// write, whatever their flags, so that Read and Edit deny rules and the
// protected paths can be checked even when the command is not otherwise
// understood.
func fillTargets(f *cmdFacts, args []word) {
	var ops []word
	for _, a := range args {
		if a.static() && strings.HasPrefix(a.joined(), "-") && !a.tilde {
			continue
		}
		ops = append(ops, a)
	}
	switch f.name {
	case "cat", "head", "tail", "less", "more", "nl", "bat", "batcat", "wc", "sort", "uniq", "cut", "tac", "strings", "xxd", "hexdump", "od", "base64", "source", ".":
		f.reads = ops
	case "grep", "egrep", "fgrep", "rg", "awk":
		if len(ops) > 1 {
			f.reads = ops[1:]
		}
	case "tee", "touch", "mkdir", "rm", "rmdir", "truncate", "shred", "unlink", "chmod", "chown":
		f.writes = ops
	case "mv":
		f.writes = ops
	case "cp", "ln", "install", "rsync", "scp":
		if len(ops) > 0 {
			f.reads = ops[:len(ops)-1]
			f.writes = ops[len(ops)-1:]
		}
	case "sed":
		// The script is the first operand unless -e or -f gave one.
		hasE := false
		inPlace := false
		for _, a := range args {
			l := a.lit()
			if l == "-e" || l == "-f" || strings.HasPrefix(l, "--expression") || strings.HasPrefix(l, "--file") {
				hasE = true
			}
			if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "--") && strings.Contains(l, "i") || strings.HasPrefix(l, "--in-place") {
				inPlace = true
			}
		}
		if !hasE && len(ops) > 0 {
			ops = ops[1:]
		}
		if inPlace {
			f.writes = ops
		} else {
			f.reads = ops
		}
	case "dd":
		for _, a := range ops {
			l := a.lit()
			if v, ok := strings.CutPrefix(l, "of="); ok {
				f.writes = append(f.writes, word{raw: v, parts: []wordPart{{lit: v}}})
			}
			if v, ok := strings.CutPrefix(l, "if="); ok {
				f.reads = append(f.reads, word{raw: v, parts: []wordPart{{lit: v}}})
			}
		}
	}
}

// cdFacts judges cd: with one plain directory it only moves the shell.
func cdFacts(f *cmdFacts, args []word) {
	if len(args) != 1 || !args[0].static() || args[0].glob || args[0].brace {
		f.why = "cd without one plain directory may leave the working directories"
		return
	}
	if l := args[0].joined(); l == "-" || strings.HasPrefix(l, "-") && !args[0].quoted {
		f.why = "cd to a directory gnosh cannot name ahead of time"
		return
	}
	f.cd = &args[0]
	f.readOnly = true
}

// dateFacts judges date: it may print the time, never set it or read a
// file of dates.
func dateFacts(f *cmdFacts, args []word) {
	p := parseArgs(args, roSpec{short: "uRIj", valued: "d", ops: opsWords,
		long: "--utc --universal --iso-8601 --iso-8601= --rfc-3339= --rfc-email --rfc-2822 --date="})
	if p.bad != "" {
		f.why = p.bad
		return
	}
	for _, o := range p.ops {
		if !strings.HasPrefix(o.joined(), "+") {
			f.why = "date with a bare argument sets the clock"
			return
		}
	}
	f.readOnly = true
}

// testOps are the operators of test, [ and [[ that only look at files
// and strings. -v and -R are left out because the shell evaluates their
// operand, and an operand it evaluates can run a command.
var testOps = map[string]bool{
	"-e": true, "-f": true, "-d": true, "-r": true, "-w": true, "-x": true, "-s": true,
	"-L": true, "-h": true, "-b": true, "-c": true, "-p": true, "-S": true, "-g": true,
	"-u": true, "-k": true, "-O": true, "-G": true, "-N": true, "-z": true, "-n": true,
	"-nt": true, "-ot": true, "-ef": true, "-a": true, "-o": true,
}

// testNumOps compare numbers. Shells evaluate their operands as
// arithmetic, and arithmetic can run a command (a[$(cmd)]), so each
// operand must be a plain whole number.
var testNumOps = map[string]bool{"-eq": true, "-ne": true, "-lt": true, "-le": true, "-gt": true, "-ge": true}

// testFacts judges test, [ and [[: they only look, and what they look at
// is read like any other file. Nothing an operand holds may be something
// the shell could evaluate.
func testFacts(f *cmdFacts, args []word) {
	if closer := map[string]string{"[": "]", "[[": "]]"}[f.name]; closer != "" {
		if len(args) == 0 || args[len(args)-1].raw != closer {
			f.why = f.name + " without its closing " + closer
			return
		}
		args = args[:len(args)-1]
	}
	for i, a := range args {
		if !a.static() || a.brace || a.glob {
			f.why = "an argument is computed when the command runs, or is a pattern: " + a.raw
			return
		}
		l := a.joined()
		switch {
		case strings.ContainsAny(l, "$`[]()"):
			f.why = "an operand (" + a.raw + ") holds characters the shell could evaluate"
			return
		case testNumOps[l]:
			if i == 0 || i+1 >= len(args) || !isInt(args[i-1].joined()) || !isInt(args[i+1].joined()) {
				f.why = "a number comparison whose operands are not plain numbers"
				return
			}
		case strings.HasPrefix(l, "-") && !testOps[l] && !isInt(l):
			f.why = f.name + "'s " + l + " is not one known to only look"
			return
		case a.tilde || strings.Contains(l, "/") || l == "." || l == "..":
			f.reads = append(f.reads, a)
		}
	}
	f.readOnly = true
}

// findFacts judges find: starting points are read, and only tests and
// printing actions are allowed.
func findFacts(f *cmdFacts, args []word) {
	i := 0
	for ; i < len(args); i++ {
		if !args[i].static() || args[i].brace {
			f.why = "an argument is computed when the command runs: " + args[i].raw
			return
		}
		l := args[i].joined()
		if l == "-P" {
			continue
		}
		if strings.HasPrefix(l, "-") || l == "!" || l == "(" {
			break
		}
		if args[i].glob {
			f.why = "a file name pattern is given to find, which has flags that write or run things"
			return
		}
		f.reads = append(f.reads, args[i])
	}
	for _, a := range args {
		if a.glob {
			f.why = "a file name pattern is given to find, which has flags that write or run things: " + a.raw
			return
		}
	}
	noVal := " -empty -depth -prune -print -print0 -not -a -and -o -or -readable -writable -executable -true -false -daystart -mount -xdev -noleaf -ls -quit -nouser -nogroup ! ( ) "
	withVal := " -name -iname -path -ipath -wholename -iwholename -regex -iregex -type -xtype -size -mtime -mmin -atime -amin -ctime -cmin -maxdepth -mindepth -user -group -perm -links -printf -regextype -newermt -uid -gid "
	for ; i < len(args); i++ {
		if !args[i].static() {
			f.why = "an argument is computed when the command runs: " + args[i].raw
			return
		}
		l := args[i].joined()
		switch {
		case strings.Contains(noVal, " "+l+" "):
		case strings.Contains(withVal, " "+l+" "):
			i++
			if i >= len(args) || !args[i].static() {
				f.why = "find's " + l + " has no plain value"
				return
			}
		default:
			f.why = "find's " + l + " is not one known to only read"
			return
		}
	}
	f.readOnly = true
}

// gitGlobal are git's own flags, before the subcommand, that change
// nothing about what runs.
var gitGlobal = map[string]bool{"--no-pager": true, "--no-optional-locks": true}

// gitRead lists, per read-only subcommand, the flags that keep it so.
// Flags are matched whole: git accepts any unambiguous start of a long
// flag, and a shortened --output or --ext-diff must not slip through, so
// anything not spelled exactly as here asks.
var gitRead = map[string]roSpec{
	"status": {short: "sbvz", long: "--short --branch --porcelain --porcelain= --long --untracked-files --untracked-files= --ignored --ignored= --show-stash --ahead-behind --no-ahead-behind --renames --no-renames --verbose --column --no-column"},
	"diff": {short: "pubwRWzMCa", valued: "U", noGlob: true,
		long: "--stat --stat= --numstat --shortstat --name-only --name-status --cached --staged --patch --unified= --no-color --color --color= --word-diff --word-diff= --color-words --ignore-all-space --ignore-space-change --ignore-blank-lines --minimal --patience --histogram --diff-filter= --find-renames --find-renames= --summary --check --no-ext-diff --no-textconv --exit-code --quiet --relative --raw --compact-summary --dirstat --no-renames --merge-base --function-context --no-prefix --abbrev --abbrev= --full-index --text --diff-algorithm="},
	"log": {short: "pusi", valued: "nSG", digits: true, noGlob: true,
		long: "--oneline --max-count= --stat --stat= --graph --all --decorate --decorate= --no-decorate --pretty= --format= --abbrev-commit --name-only --name-status --since= --until= --after= --before= --author= --committer= --grep= --no-merges --merges --first-parent --reverse --follow --date= --color --color= --no-color --shortstat --numstat --patch --no-patch --topo-order --date-order --branches --tags --remotes --left-right --cherry-pick --no-ext-diff --no-textconv --unified= --diff-filter= --full-history --ancestry-path --source --skip= --regexp-ignore-case --invert-grep --abbrev= --raw --summary"},
	"show": {short: "pus", valued: "U", noGlob: true,
		long: "--stat --stat= --name-only --name-status --pretty= --format= --no-patch --patch --color --color= --no-color --oneline --abbrev-commit --no-ext-diff --no-textconv --unified= --shortstat --numstat --raw --summary --date= --decorate --no-decorate --abbrev= --word-diff --word-diff= --ignore-all-space --ignore-space-change"},
	"branch":    {short: "arv", ops: opsNone, long: "--list --show-current --all --remotes --verbose --no-color --color --color= --column --no-column"},
	"rev-parse": {short: "q", ops: opsWords, long: "--show-toplevel --abbrev-ref --short --short= --verify --git-dir --is-inside-work-tree --is-inside-git-dir --show-prefix --show-cdup --is-bare-repository --symbolic-full-name --absolute-git-dir --git-common-dir --quiet --symbolic --all --branches --tags --remotes"},
	"ls-files":  {short: "cdmoisukztv", valued: "x", noGlob: true, long: "--cached --deleted --modified --others --ignored --stage --unmerged --killed --exclude-standard --full-name --error-unmatch --exclude= --abbrev --abbrev= --directory --no-empty-directory --deduplicate"},
	"blame":     {short: "lsewMCctfn", valued: "L", noGlob: true, long: "--date= --porcelain --line-porcelain --show-name --show-number --show-email --root --abbrev= --color-lines --color-by-age --incremental"},
}

// gitFacts judges a git command.
func gitFacts(f *cmdFacts, args []word) {
	i := 0
	for i < len(args) && args[i].static() && gitGlobal[args[i].joined()] {
		i++
	}
	if i >= len(args) || !args[i].plain() {
		f.why = "git without a plain subcommand"
		return
	}
	sub := args[i].joined()
	spec, ok := gitRead[sub]
	if !ok {
		if strings.HasPrefix(sub, "-") {
			f.why = "git's " + sub + " is not one known to only read"
		} else {
			f.why = "git " + sub + " is not on the list of git commands that only read"
		}
		return
	}
	rest := args[i+1:]
	if hasLong(rest, "--help") {
		// git shows help by starting another program, which its settings
		// can name.
		f.why = "git " + sub + " --help starts a viewer"
		return
	}
	p := parseArgs(rest, spec)
	if p.bad != "" {
		f.why = "git " + sub + ": " + p.bad
		return
	}
	if spec.ops == opsNone && len(p.ops) > 0 {
		f.why = "git " + sub + " with a name may change the repository"
		return
	}
	// Operands are revisions and paths. Anything that could name a file
	// outside the repository is read like a file.
	for _, o := range p.ops {
		if o.glob && spec.noGlob {
			f.why = "a file name pattern is given to git, which has flags that write or run things"
			return
		}
		l := o.joined()
		if spec.ops != opsWords && (o.tilde || strings.HasPrefix(l, "/") || strings.HasPrefix(l, ".") || strings.Contains(l, "/")) {
			f.reads = append(f.reads, o)
		}
	}
	f.readOnly = true
}

// sedFacts judges sed. Without -i it reads; with -i it edits its files in
// place. Either way the script must be made only of commands that cannot
// run a program or write another file.
func sedFacts(f *cmdFacts, args []word) {
	inPlace := false
	var scripts, files []word
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !a.static() || a.brace || a.glob && !a.quoted {
			f.why = "an argument to sed is computed when the command runs, or is a file name pattern: " + a.raw
			return
		}
		l := a.joined()
		switch {
		case l == "--quiet" || l == "--silent" || l == "--regexp-extended":
		case len(l) > 1 && strings.Trim(l[1:], "nEr") == "" && l[0] == '-':
		case l == "-i" || l == "--in-place" || len(l) > 2 && l[0] == '-' && strings.Trim(l[1:], "nEri") == "":
			inPlace = true
			// BSD sed takes the backup suffix as the next word; an empty
			// one is the usual way to say "no backup".
			if i+1 < len(args) && args[i+1].static() && args[i+1].joined() == "" {
				i++
			}
		case l == "-e" || l == "--expression":
			i++
			if i >= len(args) || !args[i].static() {
				f.why = "sed's -e has no plain script"
				return
			}
			scripts = append(scripts, args[i])
		case strings.HasPrefix(l, "-") && l != "-":
			f.why = "sed's " + l + " is not one gnosh follows"
			return
		case len(scripts) == 0:
			scripts = append(scripts, a)
		default:
			files = append(files, a)
		}
	}
	if len(scripts) == 0 {
		f.why = "sed has no script"
		return
	}
	for _, s := range scripts {
		if !sedScriptSafe(s.joined()) {
			f.why = "the sed script holds a command gnosh does not follow (only s, d, p, q and = are)"
			return
		}
	}
	if inPlace {
		if len(files) == 0 {
			f.why = "sed -i names no file"
			return
		}
		f.reads, f.writes, f.fsEdit = nil, files, true
		return
	}
	f.reads, f.writes = files, nil
	f.readOnly = true
}

// sedScriptSafe reports whether a sed script is a list of the commands
// s, d, p, q and =, each with an optional address, and none with a flag
// that runs a program (e) or writes a file (w).
func sedScriptSafe(s string) bool {
	for _, cmd := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '\n' }) {
		c := strings.TrimSpace(cmd)
		// An address: numbers, $, commas, steps, or /regex/ pairs.
		for c != "" {
			switch {
			case c[0] >= '0' && c[0] <= '9', c[0] == '$', c[0] == ',', c[0] == '~', c[0] == '+', c[0] == ' ', c[0] == '!':
				c = c[1:]
				continue
			case c[0] == '/':
				end := closing(c, 1, '/')
				if end < 0 {
					return false
				}
				c = c[end+1:]
				continue
			}
			break
		}
		switch {
		case c == "d" || c == "p" || c == "q" || c == "=":
		case strings.HasPrefix(c, "s") && len(c) > 3:
			d := c[1]
			if isNameChar(d) || d == ' ' || d == '\\' {
				return false
			}
			mid := closing(c, 2, d)
			if mid < 0 {
				return false
			}
			end := closing(c, mid+1, d)
			if end < 0 {
				return false
			}
			for _, fl := range c[end+1:] {
				if !(fl == 'g' || fl == 'i' || fl == 'I' || fl == 'p' || fl >= '0' && fl <= '9') {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}

// closing returns the index of the next unescaped d in s at or after i.
func closing(s string, i int, d byte) int {
	for ; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == d {
			return i
		}
	}
	return -1
}

// fsFacts judges the file commands acceptEdits lets run: mkdir, touch,
// rm, rmdir, mv and cp, with only their everyday flags.
func fsFacts(f *cmdFacts, args []word) {
	flags := map[string]string{
		"mkdir": "pv", "touch": "", "rm": "rRfvd", "rmdir": "v", "mv": "fnv", "cp": "fnvrRpa",
	}[f.name]
	var ops []word
	rest := false
	for _, a := range args {
		if !a.static() || a.brace {
			f.why = "an argument is computed when the command runs: " + a.raw
			return
		}
		l := a.joined()
		if rest || !strings.HasPrefix(l, "-") || a.tilde || l == "-" {
			ops = append(ops, a)
			continue
		}
		if l == "--" {
			rest = true
			continue
		}
		for _, c := range l[1:] {
			if !strings.ContainsRune(flags, c) || strings.HasPrefix(l, "--") {
				f.why = f.name + "'s " + l + " is not one of its everyday flags"
				return
			}
		}
	}
	if len(ops) == 0 || (f.name == "mv" || f.name == "cp") && len(ops) < 2 {
		f.why = f.name + " names too few files"
		return
	}
	if f.name == "touch" {
		for _, o := range ops {
			if o.glob {
				f.why = "touch with a file name pattern"
				return
			}
		}
	}
	f.writes = ops
	if f.name == "cp" {
		f.reads, f.writes = ops[:len(ops)-1], ops[len(ops)-1:]
	}
	f.fsEdit = true
}
