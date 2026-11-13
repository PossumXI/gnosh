package permission

import (
	"strings"
)

// maxCommandLen is the longest command line the engine will read. A
// longer one asks.
const maxCommandLen = 10000

// maxUnits bounds how many commands the engine will dig out of one line
// (wrappers inside wrappers, scripts inside scripts) before it stops and
// treats the line as one it cannot follow.
const maxUnits = 64

// unit is one command to hold against the rules: a simple command of the
// line itself, or one found by looking inside another (after a wrapper,
// in a script handed to a shell).
type unit struct {
	cmd *simpleCmd
	// derived is set for commands found inside another command.
	derived bool
	// texts caches denyTexts for the command.
	texts []string
}

// bashView is what the engine learned about one command line.
type bashView struct {
	text string
	sc   *script
	// units are every command to check against deny and ask rules.
	units []*unit
	// opaque is not empty when something will run that could not be read:
	// a program name computed at run time, a script built by eval. It
	// says what.
	opaque string
	// frags are crude pieces of text to check against deny and ask
	// rules, for lines (or scripts inside them) the reader could not
	// follow.
	frags []string
	// tooMany is set when the line held more commands than maxUnits.
	tooMany bool
}

// shellNames are programs that run a script given as text.
var shellNames = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "ash": true, "fish": true, "su": true}

// evaluators are builtins that evaluate their operands (see derive).
var evaluators = map[string]bool{
	"[[": true, "[": true, "test": true, "let": true, "declare": true, "typeset": true, "local": true,
	"readonly": true, "export": true, "read": true, "printf": true, "unset": true, "integer": true, "float": true,
}

// wrapSpec says how to find the command inside a wrapper's arguments.
type wrapSpec struct {
	// valued lists the flags that take a separate value.
	valued string
	// pos is how many operands come before the command.
	pos int
	// assigns: NAME=value words may come before the command.
	assigns bool
	// script lists the flags whose value is itself a script.
	script string
	// joined: the wrapper hands its arguments to a shell as one line.
	joined bool
}

// wrappers are programs that run the command made of their arguments.
// This list is used to look inside commands for what deny and ask rules
// name; it is deliberately wider than the list of wrappers an allow rule
// sees through (strictStrip).
var wrappers = map[string]wrapSpec{
	"env":         {valued: " -u --unset -C --chdir ", assigns: true, script: " -S --split-string "},
	"sudo":        {valued: " -u -g -h -p -C -D -R -T -U -r -t --user --group --host --prompt --chdir ", assigns: true},
	"doas":        {valued: " -u -C "},
	"timeout":     {valued: " -s --signal -k --kill-after ", pos: 1},
	"nice":        {valued: " -n --adjustment "},
	"ionice":      {valued: " -c -n -p --class --classdata "},
	"nohup":       {},
	"time":        {valued: " -f -o --format --output "},
	"stdbuf":      {valued: " -i -o -e --input --output --error "},
	"setsid":      {},
	"watch":       {valued: " -n --interval -d ", joined: true},
	"flock":       {valued: " -w --wait --timeout -E --conflict-exit-code ", pos: 1, script: " -c --command "},
	"xargs":       {valued: " -I -n -P -L -d -s -E -a --replace --max-args --max-procs --max-lines --delimiter --arg-file "},
	"command":     {},
	"builtin":     {},
	"exec":        {valued: " -a "},
	"noglob":      {},
	"nocorrect":   {},
	"busybox":     {},
	"toybox":      {},
	"chronic":     {},
	"caffeinate":  {valued: " -t -w "},
	"unbuffer":    {},
	"chroot":      {pos: 1},
	"repeat":      {pos: 1},
	"strace":      {valued: " -o -e -p -s -u "},
	"ltrace":      {valued: " -o -e -p -s -u "},
	"nsenter":     {valued: " -t --target "},
	"unshare":     {},
	"proxychains": {}, "proxychains4": {}, "torsocks": {}, "tsocks": {},
}

// viewBash reads a command line and finds every command in it.
func (e *Engine) viewBash(text string) *bashView {
	v := &bashView{text: text}
	if len(text) > maxCommandLen {
		v.sc = &script{why: "the command is longer than 10,000 characters"}
		v.frags = crudeFragments(text)
		return v
	}
	v.sc = parseScript(text)
	if !v.sc.ok {
		v.frags = crudeFragments(text)
	}
	// The line's own commands always count; what is dug out of them is
	// bounded.
	for _, c := range v.sc.cmds {
		v.units = append(v.units, &unit{cmd: c})
	}
	var prev *simpleCmd
	for _, c := range v.sc.cmds {
		e.derive(v, c, prev, 0)
		prev = c
	}
	return v
}

// setOpaque records the first thing that could not be read.
func (v *bashView) setOpaque(why string) {
	if v.opaque == "" {
		v.opaque = why
	}
}

// addScript reads text that a command hands to a shell and adds what it
// runs.
func (e *Engine) addScript(v *bashView, text string, nest nesting, depth int) {
	sub := parseScript(text)
	if !sub.ok {
		v.setOpaque("a script inside the command could not be read: " + sub.why)
		v.frags = append(v.frags, crudeFragments(text)...)
	}
	if sub.subst {
		v.sc.subst = true
	}
	var prev *simpleCmd
	for _, c := range sub.cmds {
		c.nest |= nest | nestScript
		e.addDerived(v, c, prev, depth)
		prev = c
	}
}

// addDerived records a command found inside another and looks inside it
// in turn.
func (e *Engine) addDerived(v *bashView, c *simpleCmd, prev *simpleCmd, depth int) {
	if len(v.units) >= maxUnits+len(v.sc.cmds) {
		if !v.tooMany {
			v.tooMany = true
			v.setOpaque("it holds more nested commands than gnosh will follow")
			v.frags = append(v.frags, crudeFragments(v.text)...)
		}
		return
	}
	v.units = append(v.units, &unit{cmd: c, derived: true})
	e.derive(v, c, prev, depth+1)
}

// progName returns the program a command names, with $HOME and a leading
// ~ filled in, and whether the name is known ahead of time.
func (e *Engine) progName(w word) (string, bool) {
	if w.static() {
		return w.value(e.home), true
	}
	var b strings.Builder
	for _, p := range w.parts {
		switch {
		case p.kind == partLit:
			b.WriteString(p.lit)
		case p.kind == partVar && p.name == "HOME" && e.home != "":
			b.WriteString(e.home)
		case p.kind == partVar && p.name == "PWD":
			b.WriteString(e.cur)
		default:
			return "", false
		}
	}
	return b.String(), !w.eqHead && !w.tilde
}

// derive looks inside c for the commands it would run: the command after
// a wrapper, the script handed to a shell or to eval.
func (e *Engine) derive(v *bashView, c *simpleCmd, prev *simpleCmd, depth int) {
	if len(c.words) == 0 || v.tooMany {
		return
	}
	if depth > maxShellDepth {
		v.setOpaque("commands are nested too deeply to read")
		return
	}
	full, ok := e.progName(c.words[0])
	if !ok || progIsPattern(c.words[0]) {
		// What runs is not known ahead of time. Whether a deny or ask
		// rule could still apply is worked out from the command's shape
		// (couldHit).
		return
	}
	name := baseName(full)
	args := c.words[1:]
	nest := c.nest | nestScript

	switch {
	case shellNames[name]:
		hasScript, operand := false, false
		for i := 0; i < len(args); i++ {
			a := args[i]
			if !a.static() {
				v.setOpaque("a shell is started with arguments computed when the command runs")
				break
			}
			l := a.joined()
			if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "--") && strings.Contains(l, "c") || l == "--command" {
				hasScript = true
				if i+1 < len(args) {
					if args[i+1].static() {
						e.addScript(v, args[i+1].joined(), nest, depth)
					} else {
						v.setOpaque("a shell is handed a script computed when the command runs")
					}
				}
				break
			}
			if !strings.HasPrefix(l, "-") {
				// A script file, or the value of a flag such as -o. A -c
				// after it is still looked for: finding a script that is
				// not one only makes the check stricter.
				operand = true
			}
		}
		for _, r := range c.redirs {
			switch r.op {
			case "<<", "<<-":
				hasScript = true
				if r.bodyStatic {
					e.addScript(v, r.body, nest, depth)
				} else {
					v.setOpaque("a shell is fed a script that is computed when the command runs")
				}
			case "<<<":
				hasScript = true
				if r.target.static() {
					e.addScript(v, r.target.joined(), nest, depth)
				} else {
					v.setOpaque("a shell is fed a script that is computed when the command runs")
				}
			}
		}
		if !hasScript && !operand && c.sep == sepPipe && name != "su" {
			// The script arrives through the pipe: from echo or printf's
			// words, or from a cat that passes a here-document along.
			feeder := ""
			if prev != nil && len(prev.words) > 0 && prev.words[0].static() {
				feeder = prev.words[0].joined()
			}
			switch {
			case (feeder == "echo" || feeder == "printf") && allStatic(prev.words[1:]):
				e.addScript(v, strings.Join(litsOf(prev.words[1:]), " "), nest, depth)
			case feeder == "cat" && len(prev.words) == 1 && len(prev.redirs) > 0:
				for _, r := range prev.redirs {
					switch {
					case (r.op == "<<" || r.op == "<<-") && r.bodyStatic:
						e.addScript(v, r.body, nest, depth)
					case r.op == "<<<" && r.target.static():
						e.addScript(v, r.target.joined(), nest, depth)
					default:
						v.setOpaque("a shell is fed its script through a pipe")
					}
				}
			default:
				v.setOpaque("a shell is fed its script through a pipe")
			}
		}
	case name == "eval":
		if allStatic(args) {
			e.addScript(v, strings.Join(litsOf(args), " "), nest, depth)
		} else {
			v.setOpaque("eval runs text that is computed when the command runs")
		}
	case evaluators[name]:
		// These builtins evaluate their operands as arithmetic or as
		// variable names with subscripts, and a subscript can hold a
		// command: [[ 'a[$(cmd)]' -eq 1 ]]. Quoted text that looks like a
		// substitution is read as one.
		for _, a := range args {
			if !a.static() {
				continue
			}
			if l := a.joined(); strings.Contains(l, "$(") || strings.Contains(l, "`") {
				e.addScript(v, l, nest, depth)
			}
		}
	case name == "trap":
		for _, a := range args {
			if a.static() && !strings.HasPrefix(a.joined(), "-") {
				if s := a.joined(); s != "" {
					e.addScript(v, s, nest, depth)
				}
				break
			}
		}
	case name == "find":
		for i := 0; i < len(args); i++ {
			switch args[i].lit() {
			case "-exec", "-execdir", "-ok", "-okdir":
				j := i + 1
				for j < len(args) && args[j].lit() != ";" && args[j].lit() != "+" {
					j++
				}
				if j > i+1 {
					e.addDerived(v, &simpleCmd{words: args[i+1 : j], nest: nest}, nil, depth)
				}
				i = j
			}
		}
	default:
		spec, ok := wrappers[name]
		if !ok || name == "command" && len(args) > 0 && (args[0].lit() == "-v" || args[0].lit() == "-V") {
			return
		}
		i, pos, loose := 0, spec.pos, false
	scan:
		for ; i < len(args); i++ {
			a := args[i]
			if !a.static() {
				v.setOpaque("a wrapper is given arguments computed when the command runs")
				return
			}
			l := a.joined()
			switch {
			case l == "--":
				i++
				break scan
			case strings.HasPrefix(l, "-") && len(l) > 1:
				flag, _, attached := strings.Cut(l, "=")
				switch {
				case strings.Contains(spec.script, " "+flag+" "):
					if i+1 < len(args) && args[i+1].static() {
						e.addScript(v, args[i+1].joined(), nest, depth)
					} else if i+1 < len(args) {
						v.setOpaque("a wrapper is handed a script computed when the command runs")
					}
					i++
				case attached:
				case strings.Contains(spec.valued, " "+flag+" "):
					i++
				default:
					// A flag this table does not know may take the next
					// word as its value; both readings are tried.
					loose = true
					continue
				}
				loose = false
			case spec.assigns && isAssignWord(l):
			case pos > 0:
				pos--
			default:
				break scan
			}
		}
		if i >= len(args) {
			return
		}
		e.addDerived(v, &simpleCmd{words: args[i:], nest: nest}, nil, depth)
		if loose && i+1 < len(args) {
			e.addDerived(v, &simpleCmd{words: args[i+1:], nest: nest}, nil, depth)
		}
		if spec.joined && allStatic(args[i:]) {
			e.addScript(v, strings.Join(litsOf(args[i:]), " "), nest, depth)
		}
	}
}

// progIsPattern reports whether the word that names the program is one
// the shell would expand into something else: braces or a file name
// pattern. The test commands [ and [[ are names, not patterns.
func progIsPattern(w word) bool {
	return (w.brace || w.glob) && w.raw != "[" && w.raw != "[["
}

// isAssignWord reports whether s looks like NAME=value.
func isAssignWord(s string) bool {
	_, ok := assignName(s)
	return ok
}

// allStatic reports whether every word is known ahead of time.
func allStatic(ws []word) bool {
	for _, w := range ws {
		if !w.static() {
			return false
		}
	}
	return true
}

// rawJoined is a command's words as written, one space apart. This is the
// text an allow rule is matched against.
func rawJoined(ws []word) string {
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = w.raw
	}
	return strings.Join(parts, " ")
}

// denyTexts returns every way of writing the command that a deny or ask
// rule is matched against: as written, with quotes removed, with
// expansions dropped, with the program's directory dropped, and with its
// assignments in front.
func (e *Engine) denyTexts(c *simpleCmd) []string {
	if len(c.words) == 0 {
		return nil
	}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, o := range out {
			if o == s {
				return
			}
		}
		out = append(out, s)
	}
	raw := rawJoined(c.words)
	lits := litsOf(c.words)
	bare := make([]string, len(c.words))
	for i, w := range c.words {
		bare[i] = w.joined()
	}
	add(raw)
	add(strings.Join(lits, " "))
	add(strings.Join(bare, " "))
	if name, ok := e.progName(c.words[0]); ok {
		for _, n := range []string{name, baseName(name)} {
			add(strings.Join(append([]string{n}, lits[1:]...), " "))
			add(strings.Join(append([]string{n}, bare[1:]...), " "))
		}
	}
	if len(c.assigns) > 0 {
		var as []string
		for _, a := range c.assigns {
			as = append(as, a.w.raw)
		}
		add(strings.Join(as, " ") + " " + raw)
	}
	return out
}

// strictStrip takes one wrapper off the front of a command, when the
// wrapper is on the short list an allow rule sees through and is written
// in a form this package fully understands. It returns the words that are
// left.
func strictStrip(ws []word) ([]word, bool) {
	if len(ws) < 2 || !plainName(ws[0]) {
		return nil, false
	}
	rest := ws[1:]
	for _, w := range rest {
		if !w.static() {
			return nil, false
		}
	}
	isFlag := func(w word) bool { return strings.HasPrefix(w.joined(), "-") }
	switch ws[0].joined() {
	case "nohup", "builtin", "noglob", "xargs":
		if isFlag(rest[0]) {
			return nil, false
		}
		return rest, true
	case "command":
		if isFlag(rest[0]) {
			return nil, false
		}
		return rest, true
	case "time":
		if rest[0].joined() == "-p" {
			rest = rest[1:]
		}
		if len(rest) == 0 || isFlag(rest[0]) {
			return nil, false
		}
		return rest, true
	case "nice":
		switch l := rest[0].joined(); {
		case l == "-n" && len(rest) > 2 && isInt(rest[1].joined()):
			rest = rest[2:]
		case strings.HasPrefix(l, "-n") && isInt(l[2:]), strings.HasPrefix(l, "--adjustment=") && isInt(l[13:]), len(l) > 1 && l[0] == '-' && isInt(l[1:]):
			rest = rest[1:]
		}
		if len(rest) == 0 || isFlag(rest[0]) {
			return nil, false
		}
		return rest, true
	case "stdbuf":
		for len(rest) > 0 && isFlag(rest[0]) {
			l := rest[0].joined()
			ok := len(l) >= 3 && strings.IndexByte("ioe", l[1]) >= 0 && l[0] == '-' && l[1] != '-' ||
				strings.HasPrefix(l, "--input=") || strings.HasPrefix(l, "--output=") || strings.HasPrefix(l, "--error=")
			if !ok {
				return nil, false
			}
			rest = rest[1:]
		}
		if len(rest) == 0 {
			return nil, false
		}
		return rest, true
	case "timeout":
		for len(rest) > 0 && isFlag(rest[0]) {
			l := rest[0].joined()
			switch {
			case l == "--foreground" || l == "--preserve-status" || l == "-v" || l == "--verbose",
				strings.HasPrefix(l, "--signal=") || strings.HasPrefix(l, "--kill-after="):
				rest = rest[1:]
			case (l == "-s" || l == "--signal" || l == "-k" || l == "--kill-after") && len(rest) > 1:
				rest = rest[2:]
			default:
				return nil, false
			}
		}
		if len(rest) < 2 || !isDuration(rest[0].joined()) || isFlag(rest[1]) {
			return nil, false
		}
		return rest[1:], true
	}
	return nil, false
}

// isInt reports whether s is a whole number, with or without a sign.
func isInt(s string) bool {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isDuration reports whether s is a number with an optional unit, as
// timeout takes it.
func isDuration(s string) bool {
	if s != "" && strings.IndexByte("smhd", s[len(s)-1]) >= 0 {
		s = s[:len(s)-1]
	}
	whole, frac, _ := strings.Cut(s, ".")
	return isInt(whole) && !strings.HasPrefix(whole, "-") && !strings.HasPrefix(whole, "+") && (frac == "" || isInt(frac) && frac[0] >= '0' && frac[0] <= '9')
}

// prefixOnlyExact reports whether a command is one a wildcard rule never
// approves: a wrapper that runs whatever it is given, or a find that
// runs, deletes or writes. Only a rule that spells out the whole command
// approves those.
func prefixOnlyExact(ws []word) bool {
	if len(ws) == 0 {
		return false
	}
	switch baseName(ws[0].lit()) {
	case "watch", "setsid", "ionice", "flock":
		return true
	case "find":
		for _, w := range ws[1:] {
			l := w.lit()
			if strings.HasPrefix(l, "-exec") || strings.HasPrefix(l, "-ok") || strings.HasPrefix(l, "-fprint") ||
				l == "-delete" || l == "-rm" || l == "-fls" || l == "-files0-from" {
				return true
			}
		}
	}
	return false
}
