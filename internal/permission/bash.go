package permission

import (
	"sort"
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
	// vars are the variables the line gives a value (noteVars), and
	// pending the values among them that are made of other variables.
	// grew is set when a value turned up after the commands that use it
	// may have been read: inside a script the line runs.
	vars    map[string]*lineVar
	pending []pendingVal
	grew    bool
	// pwds are the directories the line may be in, once they are worked
	// out: what $PWD may be.
	pwds []string
	// tight and bare are what closedVar goes by, worked out once the
	// whole line is read (tightDone).
	tight     bool
	bare      map[string]bool
	tightDone bool
	// index is where each of the line's own commands stands in sc.cmds.
	index map[*simpleCmd]int
	// dirsCapped is set when the line goes to more directories than
	// maxLineDirs, and moves when it holds a cd or a pushd at all.
	dirsCapped bool
	moves      bool
	// bareShell is set when a shell is started with no script in sight:
	// its script is whatever reaches its standard input.
	bareShell bool
	// unread are the crude pieces of scripts inside the line that will
	// run and could not be read (the argument of an eval, of sh -c).
	unread []string
}

// shellNames are programs that run a script given as text.
var shellNames = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "ash": true, "fish": true, "su": true,
	"rbash": true, "mksh": true, "yash": true, "posh": true, "csh": true, "tcsh": true,
	"capsh": true, // hands what follows its -- to bash
}

// evaluators are builtins that evaluate their operands (see derive).
var evaluators = map[string]bool{
	"[[": true, "[": true, "test": true, "let": true, "declare": true, "typeset": true, "local": true,
	"readonly": true, "export": true, "read": true, "printf": true, "unset": true, "integer": true, "float": true,
	"wait": true, "getopts": true,
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
	// args lists the flags whose value is split into more arguments of
	// the wrapper's own: env -S '-i rm x' is env -i rm x.
	args string
	// joined: the wrapper hands its arguments to a shell as one line.
	joined bool
	// posMaybe: one more operand may or may not come before the command
	// (taskset's mask, script's file): both are tried.
	posMaybe bool
	// dashStarts: a word that starts with a dash may be the command
	// itself. bash's own time takes -p and nothing else: "time -x y"
	// runs a program named -x.
	dashStarts bool
}

// wrappers are programs that run the command made of their arguments.
// This list is used to look inside commands for what deny and ask rules
// name; it is deliberately wider than the list of wrappers an allow rule
// sees through (strictStrip).
var wrappers = map[string]wrapSpec{
	"env":         {valued: " -u --unset -C --chdir ", assigns: true, args: " -S --split-string "},
	"sudo":        {valued: " -u -g -h -p -C -D -R -T -U -r -t --user --group --host --prompt --chdir ", assigns: true},
	"doas":        {valued: " -u -C "},
	"timeout":     {valued: " -s --signal -k --kill-after ", pos: 1},
	"nice":        {valued: " -n --adjustment "},
	"ionice":      {valued: " -c -n -p --class --classdata "},
	"nohup":       {},
	"time":        {valued: " -f -o --format --output ", assigns: true, dashStarts: true},
	"stdbuf":      {valued: " -i -o -e --input --output --error "},
	"setsid":      {},
	"watch":       {valued: " -n --interval -d ", joined: true},
	"flock":       {valued: " -w --wait --timeout -E --conflict-exit-code ", pos: 1, script: " -c --command "},
	"xargs":       {valued: " -I -J -R -S -n -P -L -d -s -E -a --max-args --max-procs --max-lines --delimiter --arg-file "},
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
	"fakeroot": {}, "eatmydata": {}, "rlwrap": {}, "torify": {}, "valgrind": {}, "ccache": {}, "catchsegv": {},
	"dbus-run-session": {}, "setpriv": {}, "systemd-run": {}, "numactl": {}, "firejail": {}, "bwrap": {},
	"xvfb-run": {valued: " -s --server-args -n --server-num -e --error-file -f --auth-file -p --xauth-protocol -w --wait "},
	"sshpass":  {valued: " -p -f -d -P "},
	"runuser":  {valued: " -u --user -g --group -G --supp-group -s --shell ", script: " -c --command "},
	"taskset":  {valued: " -c --cpu-list -p --pid ", posMaybe: true},
	"chrt":     {valued: " -p --pid -T --sched-runtime -P --sched-period -D --sched-deadline ", posMaybe: true},
	"script":   {script: " -c --command ", posMaybe: true},
	"parallel": {valued: " -j --jobs -S --sshlogin "},
	"setarch":  {posMaybe: true}, "linux32": {}, "linux64": {}, "i386": {}, "x86_64": {}, "uname26": {},
	"runcon": {valued: " -u --user -r --role -t --type -l --range "},
	"screen": {valued: " -S -c -L -p -s -t -T -e -h "},
	// ssh runs the words after the host as one line, on a machine that
	// may be this one.
	"ssh": {valued: " -b -c -D -E -e -F -I -i -J -L -l -m -O -o -p -Q -R -S -W -w ", pos: 1, joined: true},
	// What macOS and the BSDs have of the kind.
	"apply": {}, "arch": {}, "lockf": {valued: " -t ", pos: 1},
	"xcrun":        {valued: " --sdk -sdk --toolchain -toolchain "},
	"sandbox-exec": {valued: " -f -n -p -D "},
}

// viewBash reads a command line and finds every command in it.
func (e *Engine) viewBash(text string) *bashView {
	v := e.readBash(text, nil)
	// What a line gives its variables inside a script it runs ("eval
	// f=.env; cat $f") is known only once that script is read. The line
	// is then read again with those values in hand.
	for i := 0; i < 2 && v.grew; i++ {
		v = e.readBash(text, v)
	}
	return v
}

// readBash is viewBash for one reading of the line; before is the
// reading that came before it, for the variables that one found.
func (e *Engine) readBash(text string, before *bashView) *bashView {
	v := &bashView{text: text, vars: map[string]*lineVar{}}
	if len(text) > maxCommandLen {
		v.sc = &script{why: "the command is longer than 10,000 characters"}
		v.frags = crudeFragments(text)
		return v
	}
	v.sc = parseScript(text)
	if !v.sc.ok {
		v.frags = crudeFragments(text)
	}
	if before != nil {
		v.vars, v.pending = before.vars, before.pending
	}
	e.noteScript(v, v.sc)
	for _, c := range v.sc.cmds {
		e.noteVars(v, c)
	}
	e.settleVars(v)
	v.grew = false
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
	// A shell with no (( )) runs its text as commands.
	for _, a := range v.sc.arith {
		e.addScript(v, a, nestSubshell, 0, scriptArith)
	}
	for _, w := range v.sc.loose {
		e.scanValue(v, w, nestCompound, 0)
	}
	if v.bareShell {
		// "{ sh; } <<< 'rm x'" and "echo 'rm x' | (sh)": a shell with no
		// script of its own reads the here-document of the group it
		// stands in, or the pipe into it. Every here-document in the
		// line, and what every echo prints, is read as a script that may
		// run; any other pipe is a script nobody can read.
		if v.sc.piped {
			for _, f := range v.sc.cmds {
				if len(f.words) > 0 && f.words[0].static() && allStatic(f.words[1:]) &&
					(f.words[0].joined() == "echo" || f.words[0].joined() == "printf") {
					e.addScript(v, strings.Join(litsOf(f.words[1:]), " "), f.nest|nestScript, 0, scriptMaybe)
				}
			}
			v.setOpaque("a shell reads its script from a pipe")
		}
		for _, c := range v.sc.cmds {
			for _, r := range c.redirs {
				switch {
				case (r.op == "<<" || r.op == "<<-") && r.bodyStatic:
					e.addScript(v, r.body, c.nest|nestScript, 0, scriptMaybe)
				case r.op == "<<<" && r.target.static():
					e.addScript(v, r.target.joined(), c.nest|nestScript, 0, scriptMaybe)
				case r.op == "<<" || r.op == "<<-" || r.op == "<<<" || r.op == "<" && r.target.hasSubst():
					v.setOpaque("a shell reads a script that is computed when the command runs")
				}
			}
		}
	}
	// (What the scripts inside the line give its variables.)
	e.settleVars(v)
	return v
}

// lineVal is one value a line gives a variable.
type lineVal struct {
	text string
	// glob says the shell matched the text against file names where the
	// value was made: the list of a for loop, the words of an array.
	glob bool
}

// lineVar is what a line says of a variable it sets. The values are
// used only to find what deny and ask rules name ("x=rm; $x -rf y",
// "for f in .env; do cat $f; done"): a line that sets a variable is
// never allowed by a rule, so nothing is let through on their account.
type lineVar struct {
	vals []lineVal
	// open says the variable may also hold something that is not known
	// ahead of time: a value the line computes, or the one it came with.
	open bool
}

// pendingVal is a value made of other variables, which is known once
// they are.
type pendingVal struct {
	name   string
	w      word
	assign bool
	list   bool
}

// maxFilled bounds how many texts one word is read as when the line
// gives its variables several values.
const maxFilled = 256

// give records a value the line gives a variable.
func (v *bashView) give(name string, val lineVal) {
	lv := v.vars[name]
	if lv == nil {
		lv = &lineVar{}
		v.vars[name] = lv
	}
	for _, have := range lv.vals {
		if have == val {
			return
		}
	}
	lv.vals = append(lv.vals, val)
	v.grew = true
}

// openVar records that the line gives a variable a value that is not
// known ahead of time.
func (v *bashView) openVar(name string) {
	lv := v.vars[name]
	if lv == nil {
		lv = &lineVar{}
		v.vars[name] = lv
	}
	if !lv.open {
		lv.open, v.grew = true, true
	}
}

// declares are the builtins whose arguments assign.
var declares = map[string]bool{"export": true, "declare": true, "typeset": true, "readonly": true, "local": true}

// noteScript records the variables a script's for loops go through.
func (e *Engine) noteScript(v *bashView, sc *script) {
	for _, l := range sc.loops {
		if l.args {
			v.openVar(l.name) // the words the script was started with
			continue
		}
		for _, w := range l.words {
			e.noteWord(v, l.name, w, false, true)
		}
	}
	for _, w := range sc.loose {
		e.noteSets(v, w)
	}
}

// noteVars records what a command gives the line's variables: by an
// assignment, as an argument of export and its kin, or by a
// ${name:=word} in one of its words.
func (e *Engine) noteVars(v *bashView, c *simpleCmd) {
	for _, a := range c.assigns {
		e.noteSets(v, a.w)
		if len(c.words) > 0 {
			// "X=1 cmd": the value is for cmd alone, and the variable is
			// as it was in the words of the command itself.
			v.openVar(a.name)
		}
		switch {
		case a.array:
			// name=(word ...) and name+=(word ...): each word is a value.
			for _, el := range a.elems {
				e.noteSets(v, el)
				if strings.HasPrefix(el.raw, "[") {
					v.openVar(a.name) // [i]=word: not followed
					continue
				}
				e.noteWord(v, a.name, el, false, true)
			}
		case strings.HasPrefix(a.w.raw, a.name+"+="):
			v.openVar(a.name) // text added to what is there: not followed
		default:
			e.noteWord(v, a.name, a.w, true, false)
		}
	}
	for _, w := range c.words {
		e.noteSets(v, w)
	}
	for _, r := range c.redirs {
		e.noteSets(v, r.target)
	}
	// "time F=x" is an assignment to bash, for which time is no program.
	if len(c.words) > 1 && c.words[0].static() && c.words[0].joined() == "time" {
		for _, w := range c.words[1:] {
			if l := w.lit(); l == "-p" || l == "--" || l == "!" || l == "time" {
				continue
			}
			name, ok := assignName(w.raw)
			if !ok {
				break
			}
			if strings.HasPrefix(w.raw, name+"=") {
				e.noteWord(v, name, w, true, false)
			}
			v.openVar(name)
		}
	}
	// export x=1 and its kin assign too, and so may a program whose name
	// is only known when the line runs.
	if len(c.words) > 1 && (!c.words[0].static() || declares[c.words[0].joined()]) {
		for _, w := range c.words[1:] {
			if name, ok := assignName(w.raw); ok {
				if strings.HasPrefix(w.raw, name+"+=") {
					v.openVar(name)
					continue
				}
				e.noteWord(v, name, w, true, true)
			}
		}
	}
}

// noteSets records the ${name:=word} forms in a word: a variable with
// no value is given the word, and keeps it for the rest of the line.
func (e *Engine) noteSets(v *bashView, w word) {
	for _, p := range w.parts {
		if p.kind == partVar && p.sets && p.name != "" {
			if p.hasDef {
				v.give(p.name, lineVal{text: p.defIn(e.home)})
			}
			v.openVar(p.name)
		}
	}
}

// assigned returns the value in the text of an assignment word:
// what follows name= or name[i]=.
func assigned(name, text string) (string, bool) {
	if strings.HasPrefix(text, name+"=") {
		return text[len(name)+1:], true
	}
	if strings.HasPrefix(text, name+"[") {
		if k := strings.Index(text, "]="); k >= 0 {
			return text[k+2:], true
		}
	}
	return "", false
}

// noteWord records the value a word gives a variable. assign says the
// word is an assignment (name=value) and not the value alone; list says
// the shell makes more words of the braces in it and, where it is the
// value alone, matches its patterns: an argument of export, a word of an
// array or of a for loop's list.
func (e *Engine) noteWord(v *bashView, name string, w word, assign, list bool) {
	if !w.static() {
		// Made of other variables, perhaps ones the line sets
		// ("a=.env; b=$a"): see settleVars.
		for _, have := range v.pending {
			if have.name == name && have.w.raw == w.raw && have.assign == assign && have.list == list {
				return
			}
		}
		v.pending = append(v.pending, pendingVal{name: name, w: w, assign: assign, list: list})
		return
	}
	texts := []string{w.value(e.home)}
	if list && w.brace {
		if w.quoted {
			v.openVar(name) // braces with quoted text among them: not told for sure
		}
		if alts, ok := braceExpand(w.joined()); ok {
			texts = texts[:0]
			for _, alt := range alts {
				part := litOf(alt)
				part.tilde = strings.HasPrefix(alt, "~") && !w.quoted
				texts = append(texts, part.value(e.home))
			}
		}
	}
	for _, t := range texts {
		val := t
		if assign {
			var ok bool
			if val, ok = assigned(name, t); !ok {
				v.openVar(name)
				continue
			}
		}
		v.give(name, lineVal{text: val, glob: list && !assign && w.glob && hasPathMagic(val)})
	}
}

// settleVars works out the values that are made of other variables the
// line sets, a few links deep, and marks the rest as not known.
func (e *Engine) settleVars(v *bashView) {
	for round := 0; round < 3; round++ {
		for _, pv := range v.pending {
			vals, _, ok := e.filledAll(v, pv.w)
			if !ok {
				continue
			}
			for _, fv := range vals {
				val := fv.text
				if pv.assign {
					if val, ok = assigned(pv.name, fv.text); !ok {
						continue
					}
				}
				texts := []string{val}
				switch {
				case pv.list && !pv.assign && !pv.w.quoted:
					// A word of a list that nothing quotes is cut into
					// as many words as its value has.
					texts = e.fields(v, val)
				case pv.list && pv.assign && !pv.w.quoted:
					// An argument of export: a plain sh cuts it too, and
					// the variable gets what stands before the first cut.
					if f := e.fields(v, val); len(f) > 1 {
						texts = append(texts, f[0])
					}
				}
				for _, t := range texts {
					v.give(pv.name, lineVal{text: t, glob: fv.glob || pv.list && !pv.assign && !pv.w.quoted && hasPathMagic(t)})
				}
			}
		}
	}
	for _, pv := range v.pending {
		if _, _, ok := e.filledAll(v, pv.w); !ok || !e.allSet(v, pv.w) {
			v.openVar(pv.name)
		}
	}
}

// allSet reports whether every variable in the word is one the line
// gave its values, all of them known ahead of time.
func (e *Engine) allSet(v *bashView, w word) bool {
	for _, p := range w.parts {
		switch {
		case p.kind == partLit:
		case p.kind == partVar && p.name != "":
			if lv := v.vars[p.name]; lv == nil || lv.open || len(lv.vals) == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// closedWord reports whether the texts filledAll gives for the word are
// all it can be where the command at index at of the line's own
// commands uses it: the line gave each variable in it every value it
// has, in ways this package follows, nothing else on the line can have
// given it another (closedVar), and one of those values is sure to be in
// place by then (sureSet). Only then is a file the word names known for
// sure. With at set to anyTime the last of these is not asked: the
// answer is then about the values, and not about when they are there.
// It is for use once the whole line is read.
func (e *Engine) closedWord(v *bashView, w word, at int) bool {
	if _, _, ok := e.filledAll(v, w); !ok {
		return false
	}
	for _, p := range w.parts {
		switch {
		case p.kind == partLit:
		case p.kind == partVar && p.name != "" && v.vars[p.name] != nil:
			if !e.closedVar(v, p.name) || at != anyTime && !sureSet(v, p.name, at) {
				return false
			}
		case p.kind == partVar && (p.name == "HOME" && e.home != "" || p.name == "PWD" && len(v.pwds) == 1 || p.name == "IFS"):
			// As gnosh was told they are, unless the line touches them.
			if e.closedVar(v, p.name); !v.tight || v.bare[p.name] {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// The places closedWord can be asked about that are not a command of
// the line's own: anyTime for the values alone, noPlace for a command
// found inside another, of which nobody knows what runs before it.
const (
	anyTime = -1
	noPlace = -2
)

// sureSet reports whether a variable is sure to hold a value the line
// gave it when the command at index at runs: an assignment that stands
// on its own, outside every group, loop, pipe and background job, comes
// before the command; or the command is in the body of a for loop that
// goes through the variable. (A value from "X=1 cmd", from a branch
// that may not be taken or from a subshell may not be there, and the
// variable is then what it was when the line began.)
func sureSet(v *bashView, name string, at int) bool {
	if at < 0 || v.sc.piped || v.sc.background {
		return false
	}
	// A plain sh reads "F=a &>f" another way than bash: it puts the
	// assignment in the background. Nothing is sure on such a line.
	if strings.Contains(v.text, "&>") {
		return false
	}
	for _, l := range v.sc.loops {
		if l.name == name && !l.args && l.from <= at && at < l.to {
			return true
		}
	}
	for i, c := range v.sc.cmds {
		if i >= at {
			break
		}
		if c.nest != 0 || c.sep != sepStart || c.bg || len(c.redirs) > 0 {
			continue
		}
		for _, a := range c.assigns {
			if a.name == name && len(c.words) == 0 && !strings.HasPrefix(a.w.raw, name+"+=") {
				return true
			}
		}
		// (export and readonly are every shell's; declare is bash's. A
		// function of the same name would stand in their way.)
		if len(c.words) > 1 && !v.sc.funcs && c.words[0].static() && (c.words[0].joined() == "export" || c.words[0].joined() == "readonly") {
			for _, w := range c.words[1:] {
				if n, ok := assignName(w.raw); ok && n == name && strings.HasPrefix(w.raw, name+"=") {
					return true
				}
			}
		}
	}
	return false
}

// closedVar reports whether the values the line gave a variable are all
// it can hold: the line was read to its end, every program in it is
// known, none of them is a file read into this shell, no builtin that
// sets variables is handed a name that is only known when the line
// runs, the field separator is left alone, and the variable's name
// stands nowhere but where it is assigned and where it is expanded
// ("read F", "printf -v F", "let F=1", "unset F" and "declare -n r=F"
// all hold the bare name).
func (e *Engine) closedVar(v *bashView, name string) bool {
	if !v.tightDone {
		v.tight, v.bare, v.tightDone = e.tightLine(v), bareNames(v), true
	}
	lv := v.vars[name]
	return v.tight && lv != nil && len(lv.vals) > 0 && !lv.open && !v.bare[name] && !v.bare["IFS"] && v.vars["IFS"] == nil && !shellOwned[name]
}

// shellOwned are variables the shell gives a value of its own, whatever
// a line assigns to them.
var shellOwned = map[string]bool{
	"RANDOM": true, "SRANDOM": true, "SECONDS": true, "LINENO": true, "BASHPID": true, "PPID": true, "UID": true, "EUID": true,
	"GROUPS": true, "FUNCNAME": true, "PIPESTATUS": true, "BASH_COMMAND": true, "BASH_SUBSHELL": true, "EPOCHSECONDS": true,
	"EPOCHREALTIME": true, "OPTIND": true, "OPTARG": true, "REPLY": true, "_": true, "PWD": true, "OLDPWD": true, "HISTCMD": true,
	"SHLVL": true, "BASH_ARGV0": true, "BASH_REMATCH": true, "DIRSTACK": true, "BASHOPTS": true, "SHELLOPTS": true, "COLUMNS": true,
	"LINES": true, "IFS": true,
}

// tightLine is the part of closedVar that is about the whole line.
func (e *Engine) tightLine(v *bashView) bool {
	if !v.sc.ok || v.opaque != "" || v.tooMany || len(v.unread) > 0 || v.sc.blindSet {
		return false
	}
	for _, u := range v.units {
		ws := u.cmd.words
		if len(ws) == 0 {
			continue
		}
		name, known := e.progName(ws[0])
		if !known || progIsPattern(ws[0]) {
			return false
		}
		k := progKey(baseName(name))
		if k == "source" || k == "." || k == "enable" || k == "alias" {
			// A file read into this shell sets what it likes, and an
			// alias makes another command of the words that follow it.
			return false
		}
		if evaluators[k] || declares[k] || k == "mapfile" || k == "readarray" {
			for _, w := range ws[1:] {
				if _, _, ok := e.filledAll(v, w); !w.static() && !ok {
					return false
				}
			}
		}
	}
	return true
}

// bareNames returns every name that stands on the line as text of its
// own: not as the variable an assignment sets, and not as one that is
// expanded ($name, ${name}).
func bareNames(v *bashView) map[string]bool {
	out := map[string]bool{}
	note := func(text string) {
		for i := 0; i < len(text); {
			if !isNameStart(text[i]) {
				i++
				continue
			}
			j := i
			for j < len(text) && isNameChar(text[j]) {
				j++
			}
			out[text[i:j]] = true
			i = j
		}
	}
	word := func(w word, assigns bool) {
		text := w.raw
		if w.static() {
			text = w.joined()
		}
		if name, ok := assignName(w.raw); ok && assigns {
			if _, val, cut := strings.Cut(text, "="); cut && strings.HasPrefix(text, name) {
				text = val
			}
		}
		if !w.static() {
			text = lessExpansions(text)
		}
		// A flag's own letters are not a name ("[ -f x ]"); what is
		// written onto it may be ("printf -vF x", "--var=F").
		switch {
		case strings.HasPrefix(text, "--"):
			_, text, _ = strings.Cut(text, "=")
		case strings.HasPrefix(text, "-") && len(text) > 2:
			text = text[2:]
		case strings.HasPrefix(text, "-"):
			text = ""
		}
		note(text)
	}
	for _, w := range v.sc.loose {
		word(w, false)
	}
	for _, a := range v.sc.arith {
		note(lessExpansions(a))
	}
	for _, u := range v.units {
		c := u.cmd
		for _, a := range c.assigns {
			word(a.w, true)
			for _, el := range a.elems {
				word(el, false)
			}
		}
		for i, w := range c.words {
			if i > 0 {
				word(w, !c.words[0].static() || declares[c.words[0].joined()])
			}
		}
		for _, r := range c.redirs {
			word(r.target, false)
		}
	}
	return out
}

// lessExpansions returns shell text with each $name and ${name taken
// out: what is left of a name there is text of its own.
func lessExpansions(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			b.WriteByte(s[i])
			continue
		}
		j := i + 1
		if j < len(s) && s[j] == '{' {
			j++
			if j < len(s) && (s[j] == '#' || s[j] == '!') {
				j++
			}
		}
		k := j
		for k < len(s) && isNameChar(s[k]) {
			k++
		}
		if k == j {
			b.WriteByte(s[i])
			continue
		}
		b.WriteByte(' ')
		i = k - 1
	}
	return b.String()
}

// filledAll returns every text the word may be once the variables the
// line itself set are filled in, when that makes the whole word known.
// strict says there is one such text and no other: each variable was
// given one value, known ahead of time.
func (e *Engine) filledAll(v *bashView, w word) (out []lineVal, strict, ok bool) {
	if w.static() || w.brace || w.eqHead {
		return nil, false, false
	}
	// A ~ at the head of the word is the home directory where a slash
	// follows it ("~/$d/x"), and stays a ~ where a variable does
	// ("~$d"). Somebody else's home (~name) is not known.
	home := false
	if w.tilde {
		first := ""
		if w.parts[0].kind == partLit {
			first = w.parts[0].lit
		}
		switch {
		case first == "~" && len(w.parts) > 1:
		case strings.HasPrefix(first, "~/") && e.home != "":
			home = true
		default:
			return nil, false, false
		}
	}
	defer func() {
		if home {
			for i := range out {
				out[i].text = e.home + out[i].text[1:]
			}
		}
	}()
	out, strict = []lineVal{{}}, true
	for _, p := range w.parts {
		var alts []lineVal
		switch {
		case p.kind == partLit:
			alts = []lineVal{{text: p.lit}}
		case p.kind == partVar && p.name != "":
			var vals []lineVal
			lv := v.vars[p.name]
			switch {
			case lv != nil && len(lv.vals) > 0:
				vals = lv.vals
				strict = strict && len(lv.vals) == 1 && !lv.open && !lv.vals[0].glob
			case lv != nil:
				return nil, false, false
			case p.name == "HOME" && e.home != "":
				vals = []lineVal{{text: e.home}}
			case p.name == "PWD" && len(v.pwds) > 0:
				// (After a cd it is wherever the line may be by then.)
				for _, d := range v.pwds {
					vals = append(vals, lineVal{text: d})
				}
			case p.name == "PWD":
				vals = []lineVal{{text: e.cur}}
			case p.name == "IFS":
				// A shell starts with a blank, a tab and a line break in
				// it, whatever its environment holds: "rm${IFS}-rf${IFS}x"
				// is rm -rf x.
				vals = []lineVal{{text: " \t\n"}}
			case p.name == "OLDPWD" && v.moves && len(v.pwds) > 0:
				// After a cd it is a directory the line was in before.
				for _, d := range v.pwds {
					vals = append(vals, lineVal{text: d})
				}
				strict = false
			default:
				return nil, false, false
			}
			// What the form makes of each value: ${v:+word} is the word,
			// ${v%.txt} what is left of the value.
			for _, val := range vals {
				texts, known := p.of(val.text, e.home)
				if !known {
					return nil, false, false
				}
			texts:
				for _, t := range texts {
					for _, have := range alts {
						if have.text == t && have.glob == val.glob {
							continue texts
						}
					}
					alts = append(alts, lineVal{text: t, glob: val.glob})
				}
			}
			strict = strict && len(alts) == 1
		default:
			return nil, false, false
		}
		if len(out)*len(alts) > maxFilled {
			return nil, false, false
		}
		next := make([]lineVal, 0, len(out)*len(alts))
		for _, o := range out {
			for _, a := range alts {
				next = append(next, lineVal{text: o.text + a.text, glob: o.glob || a.glob})
			}
		}
		out = next
	}
	return out, strict, true
}

// fields cuts a value that nothing quotes into words, as the shell
// does: at blanks, tabs and line breaks, and at whatever else the line
// puts in IFS ("IFS=_; x=rm_-rf_y; $x" is rm -rf y). Where the line set
// IFS the shell cuts at fewer places than this; more words than it makes
// are more to hold against the rules, never fewer.
func (e *Engine) fields(v *bashView, s string) []string {
	seps := " \t\n"
	if lv := v.vars["IFS"]; lv != nil {
		for _, val := range lv.vals {
			seps += val.text
		}
	}
	return strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(seps, r) })
}

// tooManyFills reports whether filledAll gave up on a word because the
// line gives its variables more values than maxFilled ways of choosing
// among them, and not because one of them is not known.
func (e *Engine) tooManyFills(v *bashView, w word) bool {
	if w.static() || w.brace || w.eqHead {
		return false
	}
	ways := 1
	for _, p := range w.parts {
		if p.kind != partVar || p.name == "" {
			continue
		}
		lv := v.vars[p.name]
		if lv == nil || len(lv.vals) == 0 {
			continue
		}
		n := 0
		for _, val := range lv.vals {
			texts, _ := p.of(val.text, e.home)
			n += len(texts)
		}
		if n == 0 {
			n = 1
		}
		if ways *= n; ways > maxFilled {
			return true
		}
	}
	return false
}

// filled returns the word's text with every variable the line itself set
// filled in, when that makes the whole word known and there is one text
// it can be.
func (e *Engine) filled(v *bashView, w word) (string, bool) {
	if w.glob {
		return "", false
	}
	vals, strict, ok := e.filledAll(v, w)
	if !ok || !strict {
		return "", false
	}
	return vals[0].text, true
}

// braceWords returns the command's words after bash's brace expansion,
// when some word holds braces that expand. sure is false when a word
// with braces also has quoted text, whose commas and braces count for
// nothing to the shell but cannot be told apart here.
func braceWords(ws []word) (out []word, sure, ok bool) {
	sure = true
	for _, w := range ws {
		if !w.brace || !w.static() {
			out = append(out, w)
			continue
		}
		alts, fine := braceExpand(w.joined())
		if !fine || len(alts) == 1 && alts[0] == w.joined() {
			out = append(out, w)
			continue
		}
		ok = true
		sure = sure && !w.quoted
		for _, alt := range alts {
			if alt == "" && !w.quoted {
				continue // an empty word that nothing quotes is no word
			}
			part := litOf(alt)
			part.glob, part.tilde, part.quoted = w.glob && hasPathMagic(alt), strings.HasPrefix(alt, "~") && !w.quoted, w.quoted
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return nil, false, false
	}
	return out, sure, ok
}

// filledArgs returns the command with each argument that the line's own
// variables make known replaced by its value: one word when the
// argument was quoted, and otherwise as many as the value has. Where
// the line gives a variable several values (a for loop's list), there
// is one command for each way of choosing among them, up to four; past
// that, one command that holds them all, which names every file and
// every flag the others would. It returns nothing when no argument
// changes.
func (e *Engine) filledArgs(v *bashView, ws []word) [][]word {
	per := make([][][]word, len(ws)) // for each word, the words it may stand for
	per[0] = [][]word{{ws[0]}}
	changed, ways := false, 1
	for i, w := range ws[1:] {
		vals, _, ok := e.filledAll(v, w)
		if !ok || w.glob {
			per[i+1] = [][]word{{w}}
			continue
		}
		changed = true
		for _, val := range vals {
			var alt []word
			if w.quoted {
				part := litOf(val.text)
				part.glob = val.glob && hasPathMagic(val.text)
				alt = append(alt, part)
			} else {
				// Not quoted: the shell cuts the value into words and
				// matches the patterns in them.
				for _, f := range e.fields(v, val.text) {
					part := litOf(f)
					part.glob = hasPathMagic(f)
					alt = append(alt, part)
				}
			}
			per[i+1] = append(per[i+1], alt)
		}
		if ways *= len(vals); ways > 4 {
			ways = 5
		}
	}
	if !changed {
		return nil
	}
	if ways > 4 {
		var all []word
		for _, alts := range per {
			for _, alt := range alts {
				all = append(all, alt...)
			}
		}
		if len(all) > 2048 {
			return nil
		}
		return [][]word{all}
	}
	out := [][]word{nil}
	for _, alts := range per {
		var next [][]word
		for _, o := range out {
			for _, alt := range alts {
				next = append(next, append(append([]word(nil), o...), alt...))
			}
		}
		out = next
	}
	return out
}

// setOpaque records the first thing that could not be read.
func (v *bashView) setOpaque(why string) {
	if v.opaque == "" {
		v.opaque = why
	}
}

// How addScript takes a piece of text.
const (
	// scriptText: the text is a script; every command in it runs.
	scriptText = iota
	// scriptMaybe: the text may be run by a shell that reads it another
	// way (the inside of (( )) under dash). Its commands are held against
	// the rules; that it cannot be read means nothing.
	scriptMaybe
	// scriptOperand: the text is an operand a builtin evaluates. Only the
	// commands in its substitutions run; if it cannot be read, what runs
	// is not known.
	scriptOperand
	// scriptData: the text is data that the shell may yet evaluate (a
	// quoted word that looks like a substitution). Only the commands in
	// its substitutions count, and that it cannot be read means nothing.
	scriptData
	// scriptValue: the text is a value the line gives a variable, which
	// the shell may evaluate where the variable is used as a number, as
	// a prompt or as a name (${x@P}, $((x)), ${!x}). Like scriptData,
	// except that a value which cannot be read is one of which nobody
	// knows what an evaluation runs: bash makes "tee pwne" of the
	// unfinished "$(tee pwned".
	scriptValue
	// scriptArith: the inside of (( )), as a shell without that form
	// would run it. Like scriptMaybe, and its words are taken as they
	// are written: the * of a product is not a file name pattern worth
	// asking about.
	scriptArith
)

// addScript reads text that a command hands to a shell, or that the
// shell may evaluate, and adds the commands it runs.
func (e *Engine) addScript(v *bashView, text string, nest nesting, depth int, how int) {
	sub := parseScript(text)
	if !sub.ok {
		crude := crudeFragments(text)
		if how == scriptText || how == scriptOperand || how == scriptValue {
			v.setOpaque("a script inside the command could not be read: " + sub.why)
			v.unread = append(v.unread, crude...)
		}
		v.frags = append(v.frags, crude...)
	}
	e.noteScript(v, sub)
	if sub.subst && (how == scriptText || how == scriptOperand) {
		// A substitution in an operand a builtin evaluates is a
		// substitution: "test -v 'a[$(cmd)]'" runs cmd, so no rule for
		// test lets the line run unasked.
		v.sc.subst = true
	}
	var prev *simpleCmd
	for _, c := range sub.cmds {
		if (how == scriptOperand || how == scriptData || how == scriptValue) && c.nest&nestSubst == 0 {
			continue
		}
		if how == scriptArith {
			// The first word is taken as it is written (total[i] is an
			// array's element), with a note that a plain sh would take it
			// for a pattern; the * of a product stays what it is to that
			// sh, something that turns into file names.
			if len(c.words) > 0 && (c.words[0].glob || c.words[0].brace) {
				c.nameGlob = c.words[0].glob
				c.words[0].glob, c.words[0].brace = false, false
			}
			// "(( $n > 0 ))" names no program to bash, and to a shell
			// that would run it as a command it names whatever n holds.
			// That counts where the line itself set n, and not otherwise:
			// arithmetic on a variable is too common to ask about.
			// (A name that is part text, or what a command prints, is kept:
			// "(( r${x}m y ))" and "(( $(echo rm) y ))" run rm under sh.)
			// Where n holds nothing, that shell takes the next word for
			// the program: "(( $n rm y ))" runs rm y.
			if len(c.words) > 0 && !c.words[0].static() {
				name, ok := e.filled(v, c.words[0])
				switch {
				case ok && name != "":
					c.words[0] = litOf(name)
				case onlyVars(c.words[0]):
					for len(c.words) > 0 && !c.words[0].static() && onlyVars(c.words[0]) {
						c.words = c.words[1:]
					}
					if len(c.words) == 0 {
						continue
					}
					if c.words[0].glob || c.words[0].brace {
						c.nameGlob = c.words[0].glob
						c.words[0].glob, c.words[0].brace = false, false
					}
					// (An == there is a word like any other to that sh.)
					c.words[0].eqHead = false
				}
			}
		}
		c.nest |= nest | nestScript
		e.addDerived(v, c, prev, depth)
		prev = c
	}
	for _, a := range sub.arith {
		e.addScript(v, a, nest|nestSubshell, depth+1, scriptArith)
	}
}

// onlyVars reports whether a word is made of variables and nothing else.
func onlyVars(w word) bool {
	for _, p := range w.parts {
		if p.kind != partVar && !(p.kind == partLit && p.lit == "") {
			return false
		}
	}
	return true
}

// commandVars are environment variables whose value is a command line
// that some program runs: a pager, an editor, a diff or ssh command.
var commandVars = map[string]bool{
	"PAGER": true, "GIT_PAGER": true, "MANPAGER": true, "EDITOR": true, "VISUAL": true, "GIT_EDITOR": true,
	"GIT_SEQUENCE_EDITOR": true, "FCEDIT": true, "BROWSER": true, "GIT_EXTERNAL_DIFF": true,
	"GIT_SSH_COMMAND": true, "GIT_SSH": true, "GIT_ASKPASS": true, "SSH_ASKPASS": true, "SUDO_ASKPASS": true,
	"GIT_PROXY_COMMAND": true, "PROMPT_COMMAND": true, "LESSOPEN": true, "LESSCLOSE": true, "LESSEDIT": true,
	"SHELL": true, "RSYNC_RSH": true, "CC": true, "CXX": true,
}

// commandVar reads the value of an assignment to one of commandVars as
// the command it is: "GIT_EXTERNAL_DIFF='rm -rf x' git diff" runs rm.
// This serves deny and ask rules only; a line that sets a variable is
// never allowed by a rule.
func (e *Engine) commandVar(v *bashView, w word, nest nesting, depth int) {
	name, ok := assignName(w.raw)
	if !ok {
		return
	}
	switch name {
	case "BASH_CMDS", "BASH_ALIASES":
		// bash's own tables of where a command is and what an alias
		// stands for: BASH_CMDS[ls]=/bin/rm makes ls run rm. The value is
		// held against the rules, and the line is one whose later commands
		// are not what they are written as.
		v.setOpaque("the line writes to " + name + ", the shell's own table of what a name runs")
		if _, val, cut := strings.Cut(w.joined(), "="); cut && w.static() && val != "" {
			if name == "BASH_ALIASES" {
				e.addScript(v, val, nest, depth, scriptMaybe)
			} else {
				e.addDerived(v, &simpleCmd{words: []word{litOf(val)}, nest: nest}, nil, depth)
			}
		}
		return
	}
	if !commandVars[name] || !w.static() {
		return
	}
	_, val, _ := strings.Cut(w.joined(), "=")
	if val = strings.TrimLeft(val, "| "); val != "" {
		e.addScript(v, val, nest, depth, scriptMaybe)
	}
}

// scanWord looks at a word whose text is known for a command written
// inside it. The shell evaluates more than it seems to: a variable used
// as a number, an array subscript, ${x@P} and ${!x} all run the $(...) in
// a value that was set in single quotes. So quoted text that looks like a
// substitution is read as one, wherever it stands.
func (e *Engine) scanWord(v *bashView, w word, nest nesting, depth int) {
	e.scanText(v, w, nest, depth, scriptData)
}

// scanValue is scanWord for a word that gives a variable its value: an
// assignment, a word of an array or of a for loop's list.
func (e *Engine) scanValue(v *bashView, w word, nest nesting, depth int) {
	e.scanText(v, w, nest, depth, scriptValue)
}

func (e *Engine) scanText(v *bashView, w word, nest nesting, depth int, how int) {
	if depth > maxShellDepth {
		return
	}
	// The pieces of the word that are plain text, whatever stands between
	// them: ${x}'a[$(cmd)]' is evaluated like 'a[$(cmd)]'.
	if l := w.joined(); strings.Contains(l, "$(") || strings.Contains(l, "`") {
		e.addScript(v, l, nest|nestScript, depth, how)
	}
}

// addWordText reads text that the shell will expand as a word, not run
// as a script: the commands in its substitutions run, and nothing else
// in it means anything. Text that cannot be read that way is held
// against the rules crudely, and the line is one that was not followed.
func (e *Engine) addWordText(v *bashView, text string, nest nesting, depth int) {
	p := &parser{src: text, sc: &script{ok: true}}
	for p.pos < len(text) && !p.failed {
		c := text[p.pos]
		if c < 0x20 && c != '\t' || c == 0x7f {
			p.fail("a control character")
			break
		}
		procSubst := (c == '<' || c == '>') && p.pos+1 < len(text) && text[p.pos+1] == '('
		if isWordEnd(c) && !procSubst {
			p.pos++ // an operator's character is only text here
			continue
		}
		if _, ok := p.readWord(); !ok {
			p.fail("text gnosh cannot read as a word")
		}
	}
	if p.failed || len(p.docs) > 0 {
		e.addScript(v, text, nest, depth, scriptOperand)
		return
	}
	if p.sc.subst {
		v.sc.subst = true
	}
	var prev *simpleCmd
	for _, c := range p.sc.cmds {
		c.nest |= nest | nestScript
		e.addDerived(v, c, prev, depth)
		prev = c
	}
	for _, a := range p.sc.arith {
		e.addScript(v, a, nest|nestSubshell, depth+1, scriptArith)
	}
}

// addDerived records a command found inside another and looks inside it
// in turn.
func (e *Engine) addDerived(v *bashView, c *simpleCmd, prev *simpleCmd, depth int) {
	// "time ! cmd" and "! cmd" run cmd, and so does "X=1 cmd" wherever a
	// shell reads those words as a command of their own. ("time X=1" is
	// an assignment to bash: the value may be the variable's.)
	for len(c.words) > 0 && (c.words[0].raw == "!" && len(c.words) > 1 || isAssignWord(c.words[0].raw)) {
		if name, ok := assignName(c.words[0].raw); ok {
			if strings.HasPrefix(c.words[0].raw, name+"=") {
				e.noteWord(v, name, c.words[0], true, false)
			}
			v.openVar(name)
		}
		if len(c.words) == 1 {
			break
		}
		c.words = c.words[1:]
	}
	if len(c.words) == 1 && c.words[0].raw == "!" {
		c.words = nil
	}
	if len(v.units) >= maxUnits+len(v.sc.cmds) {
		if !v.tooMany {
			v.tooMany = true
			v.setOpaque("it holds more nested commands than gnosh will follow")
			crude := crudeFragments(v.text)
			v.frags = append(v.frags, crude...)
			v.unread = append(v.unread, crude...)
		}
		return
	}
	v.units = append(v.units, &unit{cmd: c, derived: true})
	e.noteVars(v, c)
	e.derive(v, c, prev, depth+1)
}

// progName returns the program a command names, with $HOME and a leading
// ~ filled in, and whether the name is known ahead of time.
func (e *Engine) progName(w word) (string, bool) {
	if w.static() {
		name := w.value(e.home)
		return name, !magicProg(name)
	}
	var b strings.Builder
	for _, p := range w.parts {
		switch {
		case p.kind == partLit:
			b.WriteString(p.lit)
		case p.kind == partVar && p.plainly() && p.name == "HOME" && e.home != "":
			// (Not ${HOME:+rm}, which is rm.)
			b.WriteString(e.home)
		case p.kind == partVar && p.plainly() && p.name == "PWD":
			b.WriteString(e.cur)
		default:
			return "", false
		}
	}
	return b.String(), !w.eqHead && !w.tilde
}

// fedScript reads the script a shell (or "source /dev/stdin") is fed on
// its standard input: a here-document, a here-string, or a pipe from
// echo, printf or a cat that passes a here-document along. It reports
// whether there was one.
func (e *Engine) fedScript(v *bashView, c, prev *simpleCmd, nest nesting, depth int, pipe bool) bool {
	found := false
	for _, r := range c.redirs {
		switch r.op {
		case "<<", "<<-":
			found = true
			if r.bodyStatic {
				e.addScript(v, r.body, nest, depth, scriptText)
			} else {
				v.setOpaque("a shell is fed a script that is computed when the command runs")
				e.addScript(v, r.body, nest, depth, scriptMaybe)
			}
		case "<<<":
			found = true
			if r.target.static() {
				e.addScript(v, r.target.joined(), nest, depth, scriptText)
			} else {
				v.setOpaque("a shell is fed a script that is computed when the command runs")
				e.addScript(v, maybeText([]word{r.target}), nest, depth, scriptMaybe)
			}
		case "<":
			if r.target.hasSubst() {
				found = true
				v.setOpaque("a shell is fed the script a command prints")
			}
		}
	}
	if found || !pipe || c.sep != sepPipe {
		return found
	}
	feeder := ""
	if prev != nil && len(prev.words) > 0 && prev.words[0].static() {
		feeder = prev.words[0].joined()
	}
	switch {
	case (feeder == "echo" || feeder == "printf") && allStatic(prev.words[1:]):
		text, sure := printed(feeder, prev.words[1:])
		if sure {
			e.addScript(v, text, nest, depth, scriptText)
		} else {
			// What is printed is built from a format or from escapes. The
			// words are still held against the rules as they stand, and
			// the script is one nobody can read ahead of time.
			e.addScript(v, text, nest, depth, scriptMaybe)
			v.setOpaque("a shell is fed text that " + feeder + " builds from a format or from escapes")
		}
	case feeder == "cat" && len(prev.words) == 1 && len(prev.redirs) > 0:
		for _, r := range prev.redirs {
			switch {
			case (r.op == "<<" || r.op == "<<-") && r.bodyStatic:
				e.addScript(v, r.body, nest, depth, scriptText)
			case r.op == "<<<" && r.target.static():
				e.addScript(v, r.target.joined(), nest, depth, scriptText)
			default:
				v.setOpaque("a shell is fed its script through a pipe")
			}
		}
	default:
		v.setOpaque("a shell is fed its script through a pipe")
	}
	return true
}

// scriptAfter returns the index of the word a shell takes for the script
// of its -c: the first one after the options that stand from args[i] on.
// A lone - or -- ends the options, a lone + is an option with nothing in
// it, and with values each o and O among an option's letters takes the
// next word for its value (set -o name, shopt -O name).
func scriptAfter(args []word, i int, values bool) int {
	k := i
	for k < len(args) && args[k].static() {
		o := args[k].joined()
		if k > i {
			if o == "--" || o == "-" {
				return k + 1
			}
			if o == "" || o[0] != '-' && o[0] != '+' {
				return k
			}
		}
		k++
		switch {
		case o == "--rcfile" || o == "--init-file":
			k++
		case values && !strings.HasPrefix(o, "--"):
			k += strings.Count(o, "o") + strings.Count(o, "O")
		}
	}
	return k
}

// printed returns what "echo words" or "printf words" writes, and
// whether that is sure to be the words as they stand. It is for echo
// given no flag but -n and no backslash (sh's echo turns \0162 into a
// letter, and so does bash's with -e or xpg_echo), and for printf given
// a format with no % and no backslash, which it prints once whatever
// follows it.
func printed(feeder string, args []word) (text string, sure bool) {
	lits := litsOf(args)
	if feeder == "printf" {
		if len(lits) == 0 {
			return "", true
		}
		if strings.HasPrefix(lits[0], "-") || strings.ContainsAny(lits[0], "%\\") {
			return strings.Join(lits, " "), false
		}
		return lits[0], true
	}
	for len(lits) > 0 && lits[0] == "-n" {
		lits = lits[1:]
	}
	text = strings.Join(lits, " ")
	return text, !(len(lits) > 0 && strings.HasPrefix(lits[0], "-")) && !strings.Contains(text, "\\")
}

// maybeText is the text of words that are not all known ahead of time,
// as a script to hold against the rules: what is written stays as it
// is, and each piece the shell would fill in becomes a variable nobody
// has set. "eval 'ls > '$F" cannot be read for what it will run, but
// what stands in it (an ls, a redirection) is still held to the guards.
func maybeText(ws []word) string {
	var b strings.Builder
	for i, w := range ws {
		if i > 0 {
			b.WriteByte(' ')
		}
		for _, p := range w.parts {
			if p.kind == partLit {
				b.WriteString(p.lit)
			} else {
				b.WriteString("${GNOSH_UNKNOWN}")
			}
		}
	}
	return b.String()
}

// flagValue hands the value of the flag (a single-letter one, written
// -C value or -Cvalue) to fn, for every time the flag is given.
func flagValue(args []word, flag byte, fn func(w word, ok bool)) {
	for i := 0; i < len(args); i++ {
		// A word that is not all known is read by what is written of it:
		// "$x-C'rm x'" hands the flag its value whatever x holds, and that
		// value is then not known for sure (ok is false).
		l := args[i].joined()
		if len(l) < 2 || l[0] != '-' || l[1] == '-' || strings.IndexByte(l, flag) < 0 {
			continue
		}
		if rest := l[strings.IndexByte(l, flag)+1:]; rest != "" {
			fn(litOf(rest), args[i].static())
		} else if i+1 < len(args) {
			fn(args[i+1], args[i+1].static() && args[i].static())
			i++
		}
	}
}

// litOf makes a word of plain text.
func litOf(s string) word { return word{raw: s, parts: []wordPart{{lit: s}}} }

// derive looks inside c for the commands it would run: the command after
// a wrapper, the script handed to a shell or to eval.
func (e *Engine) derive(v *bashView, c *simpleCmd, prev *simpleCmd, depth int) {
	if v.tooMany {
		return
	}
	if depth > maxShellDepth {
		v.setOpaque("commands are nested too deeply to read")
		return
	}
	nest := c.nest | nestScript
	for _, a := range c.assigns {
		e.scanValue(v, a.w, nest, depth)
		for _, el := range a.elems {
			e.scanValue(v, el, nest, depth)
		}
		e.commandVar(v, a.w, nest, depth)
	}
	for _, r := range c.redirs {
		// bash expands the word after >& a second time when it is not a
		// descriptor: "ls >&'$(cmd)'" runs cmd, quotes or no quotes. What
		// the first expansion leaves of such a word is read as text the
		// shell evaluates; any other target that holds the text of a
		// substitution is read as one that may be.
		if l := r.target.joined(); r.op == ">&" && !isFdTarget(r.target) && strings.ContainsAny(l, "$`<>") {
			e.addWordText(v, l, nest, depth)
		} else {
			e.scanWord(v, r.target, nest, depth)
		}
	}
	// The other reading of a [[ ]] test: see testCommand.
	if len(c.pieces) > 0 || len(c.altRedirs) > 0 {
		pieces, alt := c.pieces, c.altRedirs
		c.pieces, c.altRedirs = nil, nil
		for _, piece := range pieces {
			e.addDerived(v, &simpleCmd{words: piece, nest: nest}, nil, depth)
		}
		if len(alt) > 0 && len(v.units) < maxUnits+len(v.sc.cmds) {
			v.units = append(v.units, &unit{cmd: &simpleCmd{redirs: alt, nest: nest}, derived: true})
		}
		c.pieces, c.altRedirs = pieces, alt
	}
	if len(c.words) == 0 {
		return
	}
	// An argument that holds a variable the line itself set is also read
	// as what it was set to, in as many words as the shell would make of
	// it: "o=-exec; find . $o rm {} \\;" and "f=-c; sh $f 'rm x'".
	for _, alt := range e.filledArgs(v, c.words) {
		e.addDerived(v, &simpleCmd{words: alt, redirs: c.redirs, sep: c.sep, nest: c.nest}, prev, depth)
	}
	// Braces are words bash makes before it looks at anything else, the
	// program's name and its flags among them: "bash {-c,'rm x'}" is
	// bash -c 'rm x'. The command is read again as those words. (With a
	// quote among the braces the words cannot be told for sure.)
	if alt, sure, ok := braceWords(c.words); ok {
		if !sure {
			v.setOpaque("braces with quoted text in them make words gnosh cannot tell for sure")
		}
		e.addDerived(v, &simpleCmd{words: alt, redirs: c.redirs, sep: c.sep, nest: c.nest}, prev, depth)
	}
	full, ok := e.progName(c.words[0])
	if !ok || progIsPattern(c.words[0]) {
		// What runs is not known ahead of time. Whether a deny or ask
		// rule could still apply is worked out from the command's shape
		// (couldHit). A name the line itself set ("x=rm; $x y") is tried
		// as that name as well.
		names, _, _ := e.filledAll(v, c.words[0])
		if c.words[0].glob || len(names) > 16 {
			names = nil
		}
		for _, nv := range names {
			name := nv.text
			if name == "" {
				continue
			}
			ws := append([]word{litOf(name)}, c.words[1:]...)
			if fields := e.fields(v, name); len(fields) > 1 {
				// An unquoted variable is split into words, and the
				// patterns in those are matched.
				ws = nil
				for i, f := range fields {
					part := litOf(f)
					part.glob = i > 0 && hasPathMagic(f)
					ws = append(ws, part)
				}
				ws = append(ws, c.words[1:]...)
			}
			e.addDerived(v, &simpleCmd{words: ws, redirs: c.redirs, nest: nest}, prev, depth)
		}
		// A name of which enough is written to tell what it could be is
		// read as each of the programs this file knows something about
		// that it could be: "alias${x} zz='rm x'" is alias when x is not
		// set, and so read.
		for _, name := range e.couldBe(c.words[0]) {
			ws := append([]word{litOf(name)}, c.words[1:]...)
			e.addDerived(v, &simpleCmd{words: ws, redirs: c.redirs, sep: c.sep, nest: c.nest}, prev, depth)
		}
		// A program that is not known may be a shell or a wrapper
		// ("$SH -c 'rm x'", "$RUN rm x"): its arguments are held against
		// the rules as the script and as the command they would then be.
		// It may be a shell that reads its script from a pipe or a
		// here-document: "echo 'rm x' | ba${x}sh".
		if e.couldBeShell(c.words[0]) {
			e.fedScript(v, c, prev, nest, depth, true)
		} else {
			// Whatever it is, the text it is fed is held against the
			// rules: it may be one that runs what it reads.
			for _, r := range c.redirs {
				switch {
				case r.op == "<<" || r.op == "<<-":
					e.addScript(v, r.body, nest, depth, scriptMaybe)
				case r.op == "<<<":
					e.addScript(v, maybeText([]word{r.target}), nest, depth, scriptMaybe)
				}
			}
		}
		// It may also be xargs, which adds the words it reads to the
		// command, where its mark stands or at the end: with something on
		// its standard input, those are words nobody knows.
		fed := c.sep == sepPipe || len(c.redirs) > 0
		marks := []string{"{}"}
		for i := 1; i < len(c.words) && i <= 8; i++ {
			e.addDerived(v, &simpleCmd{words: c.words[i:], nest: nest}, nil, depth)
			if a := c.words[i]; a.static() {
				// The word may be a script, and so may what a flag or a
				// name holds: -c'rm x', --command='rm x', zz='rm x'.
				l := a.joined()
				_, val, eq := strings.Cut(l, "=")
				switch {
				case !strings.HasPrefix(l, "-"):
					e.addScript(v, l, nest, depth, scriptMaybe)
				case !strings.HasPrefix(l, "--") && len(l) > 2:
					e.addScript(v, l[2:], nest, depth, scriptMaybe)
				}
				if eq && val != "" {
					e.addScript(v, val, nest, depth, scriptMaybe)
				}
			}
			if fed {
				e.addDerived(v, &simpleCmd{words: xargsAdds(c.words[i:], marks), nest: nest}, nil, depth)
			}
			if l := c.words[i].lit(); strings.HasPrefix(l, "-I") || strings.HasPrefix(l, "-i") {
				if len(l) > 2 {
					marks = append(marks, l[2:])
				} else if i+1 < len(c.words) {
					marks = append(marks, c.words[i+1].lit())
				}
			}
		}
		return
	}
	// The tables below go by the program's key: BASH is bash, and XARGS
	// xargs, where the file system takes no notice of case.
	name := progKey(baseName(full))
	args := c.words[1:]

	isScript := true
	switch {
	case shellNames[name]:
		hasScript, operand := false, false
		for i := 0; i < len(args); i++ {
			a := args[i]
			if !a.static() || a.glob && !operand {
				// (A pattern before the script may be a flag, were there a
				// file of that name.)
				v.setOpaque("a shell is started with arguments computed when the command runs")
				break
			}
			l := a.joined()
			// A c among the letters of a word that starts with - or with +
			// ("bash +c 'x'" runs x as well).
			cluster := len(l) > 1 && (l[0] == '-' || l[0] == '+') && !strings.HasPrefix(l, "--")
			if cluster && strings.Contains(l[1:], "c") || l == "--command" {
				hasScript = true
				// The script is the first word after the options that
				// follow: "bash -c -- 'x'", "sh -c -e 'x'", "bash -c - 'x'"
				// and "bash -c -eo pipefail 'x'" all run x. Whether an o
				// takes the next word for its value depends on the shell,
				// so both places are read.
				k1, k2 := scriptAfter(args, i, true), scriptAfter(args, i, false)
				for n, k := range []int{k1, k2} {
					if k >= len(args) || n == 1 && k == k1 {
						continue
					}
					switch {
					case !allKnown(args[k : k+1]):
						v.setOpaque("a shell is handed a script computed when the command runs")
						e.addScript(v, maybeText(args[k:k+1]), nest, depth, scriptMaybe)
					case n == 0:
						e.addScript(v, args[k].joined(), nest, depth, scriptText)
					default:
						e.addScript(v, args[k].joined(), nest, depth, scriptMaybe)
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
		if name == "su" {
			// su takes the script attached to the flag as well: -c'x'.
			flagValue(args, 'c', func(w word, ok bool) {
				if ok {
					e.addScript(v, w.joined(), nest, depth, scriptMaybe)
				}
			})
		}
		if e.fedScript(v, c, prev, nest, depth, !hasScript && !operand && name != "su") {
			hasScript = true
		}
		if !hasScript {
			v.bareShell = true
		}
	case name == "source" || name == ".":
		// A file that is sourced is a program of its own, which no rule
		// reads. Text the line itself supplies is read.
		switch {
		case len(args) == 0:
		case args[0].hasSubst():
			v.setOpaque("a script that a command prints is run in this shell")
		case !args[0].static() || args[0].glob || args[0].brace:
			// A name that is not known ahead of time (a variable, a
			// pattern, braces) may be /dev/stdin: what is fed to the
			// command is then its script.
			e.fedScript(v, c, prev, nest, depth, true)
		default:
			f := args[0].joined()
			if strings.HasPrefix(f, "/") {
				f = oneDisk(cleanPath(f))
			}
			if f == "/dev/stdin" || strings.HasPrefix(f, "/dev/fd/") || strings.HasPrefix(f, "/proc/self/fd/") || strings.HasPrefix(f, "/proc/thread-self/fd/") {
				if !e.fedScript(v, c, prev, nest, depth, true) {
					v.bareShell = true
				}
			}
		}
	case name == "eval":
		if len(args) > 0 && args[0].lit() == "--" {
			args = args[1:] // bash's eval ends its options there
		}
		if allKnown(args) {
			e.addScript(v, strings.Join(litsOf(args), " "), nest, depth, scriptText)
		} else {
			v.setOpaque("eval runs text that is computed when the command runs")
			e.addScript(v, maybeText(args), nest, depth, scriptMaybe)
		}
	case name == "alias":
		// What an alias stands for runs wherever its name is written
		// later in the line.
		for _, a := range args {
			if !a.static() {
				v.setOpaque("an alias is given text computed when the command runs")
				continue
			}
			if _, val, ok := strings.Cut(a.joined(), "="); ok && val != "" {
				e.addScript(v, val, nest, depth, scriptText)
			}
		}
	case name == "mapfile" || name == "readarray" || name == "compgen" || name == "complete":
		// -C names a command the builtin runs.
		flagValue(args, 'C', func(w word, ok bool) {
			if ok {
				e.addScript(v, w.joined(), nest, depth, scriptText)
			} else {
				v.setOpaque(name + " -C runs text that is computed when the command runs")
				e.addScript(v, maybeText([]word{w}), nest, depth, scriptMaybe)
			}
		})
		for _, a := range args {
			if !a.static() || a.glob {
				// Whatever the word turns into may be the flag and the
				// command it names.
				v.setOpaque(name + " is given a word computed when the command runs, which may name a command for it to call")
				break
			}
		}
		isScript = false
	case name == "hash":
		// hash -p file name: the file runs whenever name is written.
		flagValue(args, 'p', func(w word, ok bool) {
			if ok {
				e.addDerived(v, &simpleCmd{words: []word{w}, nest: nest}, nil, depth)
			} else {
				v.setOpaque("hash -p names a program computed when the command runs")
			}
		})
		for _, a := range args {
			if !a.static() || a.glob {
				v.setOpaque("hash is given a word computed when the command runs, which may tell the shell where a program is")
				break
			}
		}
		isScript = false
	case name == "enable":
		flagValue(args, 'f', func(word, bool) {
			v.setOpaque("enable -f loads a program into the shell")
		})
		for _, a := range args {
			if !a.static() || a.glob {
				v.setOpaque("enable is given a word computed when the command runs")
				break
			}
		}
		isScript = false
	case name == "set" || name == "shopt":
		v.shellOptions(name, args)
		isScript = false
	case evaluators[name]:
		// These builtins evaluate their operands as arithmetic or as
		// variable names with subscripts, and a subscript can hold a
		// command: [[ 'a[$(cmd)]' -eq 1 ]]. Quoted text that looks like a
		// substitution is read as one.
		for _, a := range args {
			if l := a.joined(); strings.Contains(l, "$(") || strings.Contains(l, "`") {
				e.addScript(v, l, nest, depth, scriptOperand)
			}
		}
	case name == "trap":
		// The script is the first argument that is not an option.
		for _, a := range args {
			if !a.static() || a.glob {
				v.setOpaque("trap runs text that is computed when the command runs")
				e.addScript(v, maybeText([]word{a}), nest, depth, scriptMaybe)
				break
			}
			if !strings.HasPrefix(a.joined(), "-") {
				if s := a.joined(); s != "" {
					e.addScript(v, s, nest, depth, scriptText)
				}
				break
			}
		}
	case name == "find":
		isScript = false
		// runs adds the command find would run from args[from:to].
		runs := func(from, to int) {
			if to <= from {
				return
			}
			// The names of the files found stand where {} does: words
			// nobody knows ahead of time.
			run := append([]word(nil), args[from:to]...)
			for k := 1; k < len(run); k++ {
				if strings.Contains(run[k].lit(), "{}") {
					run[k] = word{raw: run[k].raw, parts: []wordPart{{kind: partVar}}}
				}
			}
			e.addDerived(v, &simpleCmd{words: run, nest: nest}, nil, depth)
			if buildsCommand(args[from:to], "{}") {
				v.setOpaque("find runs the names of the files it finds as commands")
			}
		}
		// end returns where the command that starts at args[from] ends: at
		// a ; or a +.
		end := func(from int) int {
			j := from
			for j < len(args) && args[j].lit() != ";" && args[j].lit() != "+" {
				j++
			}
			return j
		}
		for i := 0; i < len(args); i++ {
			// A word that is not all known is read by what is written of
			// it ("$x-exec"). One that is not known at all may be -exec:
			// the words after it, up to a ; or a +, may then be a command.
			// And one written without quotes may turn into several words,
			// an -exec and its command among them.
			if args[i].static() && args[i].glob && strings.HasPrefix(args[i].joined(), "-") {
				// A pattern that starts with a dash may be -exec, were
				// there a file of that name.
				v.setOpaque("find is given a file name pattern that may turn into one of its own words")
			}
			if !args[i].static() {
				if !args[i].quoted {
					v.setOpaque("find is given words computed when the command runs, which may tell it to run a command")
				}
				if args[i].joined() == "" {
					if j := end(i + 1); j < len(args) {
						runs(i+1, j)
					}
					continue
				}
			}
			switch args[i].joined() {
			case "-exec", "-execdir", "-ok", "-okdir":
				j := end(i + 1)
				runs(i+1, j)
				i = j
			}
		}
	default:
		isScript = false
		spec, ok := wrappers[name]
		if !ok && (strings.HasPrefix(name, "ld-") || strings.HasPrefix(name, "ld.so")) && strings.Contains(name, ".so") {
			// The dynamic loader runs the program it is given:
			// /lib64/ld-linux-x86-64.so.2 /bin/rm x.
			spec, ok = wrapSpec{valued: " --library-path --preload --argv0 --audit --glibc-hwcaps-prepend --glibc-hwcaps-mask "}, true
		}
		if !ok || name == "command" && len(args) > 0 && (args[0].lit() == "-v" || args[0].lit() == "-V") {
			break
		}
		for _, start := range e.wrappedAt(v, spec, args, nest, depth) {
			inner := args[start:]
			if name == "xargs" {
				// xargs adds the words it reads to the command: after its
				// last word, or wherever the mark of -I stands. Nobody
				// knows those words ahead of time, and a rule may name
				// what they make: "echo push | xargs git".
				inner = xargsAdds(inner, xargsMarks(args[:start]))
			}
			if name == "parallel" {
				// parallel does the same with the words after :::, or with
				// what it reads.
				for k, w := range inner {
					if strings.HasPrefix(w.lit(), ":::") {
						inner = inner[:k]
						break
					}
				}
				if len(inner) == 0 {
					continue
				}
				inner = xargsAdds(inner, []string{"{}"})
			}
			// The wrapped command has the wrapper's standard input: a
			// here-document or a pipe into "sudo sh" is sh's script.
			e.addDerived(v, &simpleCmd{words: inner, redirs: c.redirs, sep: c.sep, nest: nest}, prev, depth)
			if spec.joined && allStatic(args[start:]) {
				e.addScript(v, strings.Join(litsOf(args[start:]), " "), nest, depth, scriptText)
			}
			if name == "xargs" {
				// xargs adds what it reads to the command. Given to a
				// program that runs its arguments, or put where the
				// program's name is, that is a command nobody wrote down.
				inner, _ := e.progName(args[start])
				prog := progKey(baseName(inner))
				_, runs := wrappers[prog]
				for _, mark := range xargsMarks(args[:start]) {
					runs = runs || buildsCommand(args[start:], mark)
				}
				if shellNames[prog] || prog == "eval" || runs {
					v.setOpaque("xargs hands what it reads to a program that runs it")
				}
			}
		}
	}
	if !isScript {
		for _, a := range args {
			e.scanWord(v, a, nest, depth)
		}
	}
}

// readNames are the programs this package reads the arguments of in a
// way of their own: the builtins derive knows, those that take the line
// to another directory or give a variable its value, the shells and the
// wrappers. They are in the order of the alphabet, so that the same
// line is always read the same way.
var readNames = func() []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	for _, name := range []string{"eval", "source", ".", "alias", "trap", "mapfile", "readarray", "compgen", "complete",
		"hash", "enable", "set", "shopt", "find", "cd", "pushd", "popd"} {
		add(name)
	}
	for _, names := range []map[string]bool{shellNames, evaluators, declares} {
		for name := range names {
			add(name)
		}
	}
	for name := range wrappers {
		add(name)
	}
	sort.Strings(out)
	return out
}()

// couldBe returns the programs this package has a reading for whose
// name the word could turn out to be, when part of the name is written:
// "alias${x}" is alias when x is not set. A name that is all variable
// could be any of them, and is left to the checks that say so.
func (e *Engine) couldBe(w word) []string {
	if progIsPattern(w) || w.eqHead || w.tilde || w.static() {
		return nil
	}
	var head []tok
	lits := 0
	for _, p := range w.parts {
		if p.kind == partLit {
			head = appendLit(head, p.lit)
			lits += len(p.lit)
		} else {
			head = appendHole(head)
		}
	}
	if lits == 0 {
		return nil
	}
	head = foldHead(head)
	// The program by its name alone is what follows the last / that is
	// written, as it is for the rules (skeletons).
	base := head
	for i := len(head) - 1; i >= 0; i-- {
		if !head[i].hole && head[i].b == '/' {
			base = head[i+1:]
			break
		}
	}
	var out []string
	for _, name := range readNames {
		if couldMatch(base, appendLit(nil, name)) {
			out = append(out, name)
		}
	}
	return out
}

// couldBeShell reports whether a program name that is not known ahead of
// time could turn out to be a shell's: its known pieces fit one of the
// names, with or without a directory in front.
func (e *Engine) couldBeShell(w word) bool {
	var head []tok
	if progIsPattern(w) || w.eqHead || w.tilde || w.static() {
		// (A name that is all there and still not known is one that
		// names a program by a number: see magicProg.)
		head = appendHole(head)
	} else {
		for _, p := range w.parts {
			if p.kind == partLit {
				head = appendLit(head, p.lit)
			} else {
				head = appendHole(head)
			}
		}
	}
	head = foldHead(head)
	for name := range shellNames {
		if couldMatch(head, appendLit(nil, name)) || couldMatch(head, appendLit(appendHole(nil), "/"+name)) {
			return true
		}
	}
	// source and . read a script too, when their file is /dev/stdin.
	return couldMatch(head, appendLit(nil, "source")) || couldMatch(head, appendLit(nil, "."))
}

// xargsMarks returns the marks that stand, in the command xargs runs,
// for what it reads: {} and whatever -I, -J (the BSDs'), -i or
// --replace names.
func xargsMarks(flags []word) []string {
	marks := []string{"{}"}
	for _, f := range []byte{'I', 'J', 'i'} {
		flagValue(flags, f, func(w word, ok bool) {
			if ok && w.joined() != "" {
				marks = append(marks, w.joined())
			}
		})
	}
	for _, w := range flags {
		if v, ok := strings.CutPrefix(w.lit(), "--replace="); ok && v != "" {
			marks = append(marks, v)
		}
	}
	return marks
}

// xargsAdds returns the command xargs would run from ws: every word that
// holds one of the marks, and one more word at the end, is something
// only known when the line runs.
func xargsAdds(ws []word, marks []string) []word {
	unknown := word{raw: "{}", parts: []wordPart{{kind: partSubst}}}
	out := append([]word(nil), ws...)
	for k := 1; k < len(out); k++ {
		for _, m := range marks {
			if m != "" && strings.Contains(out[k].lit(), m) {
				out[k] = unknown
			}
		}
	}
	return append(out, unknown)
}

// buildsCommand reports whether the words run something made from the
// text that stands for repl: repl as the program's name, or inside the
// script of a shell.
func buildsCommand(ws []word, repl string) bool {
	if len(ws) == 0 || repl == "" {
		return false
	}
	if strings.Contains(ws[0].lit(), repl) {
		return true
	}
	if prog := progKey(baseName(ws[0].lit())); !shellNames[prog] && prog != "eval" {
		return false
	}
	for _, w := range ws[1:] {
		if strings.Contains(w.lit(), repl) {
			return true
		}
	}
	return false
}

// wrappedAt returns every place among a wrapper's arguments where the
// command it runs may start. A flag the table does not know may or may
// not take the next word as its value, so both are followed.
func (e *Engine) wrappedAt(v *bashView, spec wrapSpec, args []word, nest nesting, depth int) []int {
	type state struct{ i, pos int }
	seen := map[state]bool{}
	isStart := map[int]bool{}
	work := []state{{0, spec.pos}}
	if spec.posMaybe {
		work = append(work, state{0, spec.pos + 1})
	}
	for len(work) > 0 {
		cur := work[len(work)-1]
		work = work[:len(work)-1]
		i, pos := cur.i, cur.pos
	scan:
		for ; i < len(args); i++ {
			if seen[state{i, pos}] {
				break
			}
			if len(seen) >= 512 {
				v.setOpaque("a wrapper is given more flags than gnosh will follow")
				work = nil
				break
			}
			seen[state{i, pos}] = true
			a := args[i]
			if !a.static() || a.glob {
				// (A pattern among a wrapper's own words may be a flag,
				// were there a file of that name.) The word may be the
				// command itself, a flag, or a flag and its value: what
				// stands at each of those places is read as the command
				// ("sudo $RM -rf /", "env rm${s}-rf${s}/").
				v.setOpaque("a wrapper is given arguments computed when the command runs")
				for k := i + pos; k <= i+pos+2 && k < len(args); k++ {
					isStart[k] = true
				}
				break
			}
			l := a.joined()
			switch {
			case l == "--":
				if i+1+pos < len(args) {
					isStart[i+1+pos] = true
				}
				break scan
			case l == "-":
				// env's "-" is its -i. To time and the rest it is no flag:
				// "time - x" runs a program named "-".
				isStart[i] = true
			case strings.HasPrefix(l, "-"):
				if spec.dashStarts && l != "-p" {
					isStart[i] = true
				}
				flag, val, attached := strings.Cut(l, "=")
				// kind says what a flag's value is: a script for a shell
				// (s), more of the wrapper's own arguments (a), or a value
				// that runs nothing (v).
				kind := func(f string) byte {
					switch {
					case strings.Contains(spec.script, " "+f+" "):
						return 's'
					case strings.Contains(spec.args, " "+f+" "):
						return 'a'
					case strings.Contains(spec.valued, " "+f+" "):
						return 'v'
					}
					return 0
				}
				// hand gives a flag's value to what reads it; after are the
				// words that follow the value. spliced is set when the value
				// was more arguments for the wrapper: the words after it
				// were then read along with them, and the scan ends here.
				spliced := false
				hand := func(k byte, w word, after []word) {
					switch {
					case k == 'v':
					case !w.static():
						v.setOpaque("a wrapper is handed a script computed when the command runs")
						e.addScript(v, maybeText([]word{w}), nest, depth, scriptMaybe)
					case k == 's':
						e.addScript(v, w.joined(), nest, depth, scriptText)
					default:
						e.wrapperArgs(v, spec, w.joined(), after, nest, depth)
						spliced = true
					}
				}
				// next hands over the word after the flag.
				next := func(k byte) {
					if i+1 < len(args) {
						hand(k, args[i+1], args[i+2:])
					}
					i++
				}
				long := strings.HasPrefix(l, "--")
				switch k := kind(flag); {
				case long && k != 0 && attached:
					hand(k, litOf(val), args[i+1:]) // --split-string='rm x'
				case long && k != 0:
					next(k)
				case long && attached:
				case long:
					work = append(work, state{i + 2, pos})
				default:
					// Letters in one word: the first that takes a value takes
					// the rest of the word (-S'rm x', -iS'rm x') or, when it
					// is the last, the next word.
					found := false
					for c := 1; c < len(l) && !found; c++ {
						if k := kind("-" + l[c:c+1]); k != 0 {
							found = true
							if c+1 < len(l) {
								hand(k, litOf(l[c+1:]), args[i+1:])
							} else {
								next(k)
							}
						}
					}
					if !found {
						work = append(work, state{i + 2, pos})
					}
				}
				if spliced {
					break scan
				}
			case spec.assigns && strings.IndexByte(l, '=') > 0:
				// env takes any word with an = in it for an assignment,
				// whatever stands before the = ("a.b=1").
				e.commandVar(v, a, nest, depth)
			case pos > 0:
				pos--
			default:
				isStart[i] = true
				break scan
			}
		}
	}
	var out []int
	for i := range args {
		if isStart[i] {
			out = append(out, i)
		}
	}
	return out
}

// wrapperArgs reads a string that a wrapper splits into more arguments of
// its own (env -S): flags and assignments first, and then the command,
// which goes on with the words that follow the string (after). It is
// also held against the rules the way a shell would read it.
func (e *Engine) wrapperArgs(v *bashView, spec wrapSpec, text string, after []word, nest nesting, depth int) {
	if v.tooMany {
		return
	}
	fields, ok := splitArgs(text)
	if !ok || depth > maxShellDepth {
		v.setOpaque("env -S is given a string gnosh cannot split the way env does")
	}
	fields = append(fields, after...)
	if depth <= maxShellDepth {
		for _, start := range e.wrappedAt(v, spec, fields, nest, depth+1) {
			e.addDerived(v, &simpleCmd{words: fields[start:], nest: nest}, nil, depth)
		}
	}
	e.addScript(v, text, nest, depth, scriptMaybe)
}

// splitArgs cuts a string into words the way env -S does: at blanks,
// with quotes and backslashes taken off, a # at the start of a word
// ending the string and ${NAME} standing for something not known here.
// It reports false for a string it cannot cut.
func splitArgs(s string) (out []word, ok bool) {
	var cur strings.Builder
	have, dynamic := false, false
	flush := func() {
		if have {
			w := litOf(cur.String())
			if dynamic {
				w.parts = append(w.parts, wordPart{kind: partVar})
			}
			out = append(out, w)
		}
		cur.Reset()
		have, dynamic = false, false
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			flush()
		case c == '#' && !have:
			return out, true
		case c == '\'':
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				return out, false
			}
			cur.WriteString(s[i+1 : i+1+end])
			have = true
			i += end + 1
		case c == '"':
			have = true
			for i++; i < len(s) && s[i] != '"'; i++ {
				switch {
				case s[i] == '\\' && i+1 < len(s):
					i++
					cur.WriteByte(s[i])
				case s[i] == '$':
					dynamic = true
				default:
					cur.WriteByte(s[i])
				}
			}
			if i >= len(s) {
				return out, false
			}
		case c == '\\':
			if i+1 >= len(s) {
				return out, false
			}
			i++
			if s[i] == 'c' {
				// \c ends the string there.
				flush()
				return out, true
			}
			if s[i] == '_' {
				flush()
				continue
			}
			cur.WriteByte(s[i])
			have = true
		case c == '$':
			have, dynamic = true, true
		default:
			cur.WriteByte(c)
			have = true
		}
	}
	flush()
	return out, true
}

// safeSetFlags and safeSetNames are the options of set that change
// nothing about how the rest of a line is read. The others do: with
// history and histexpand on, "!!:s/echo/rm/" runs a command the line
// never spells out, and -k turns arguments into assignments.
const safeSetFlags = "euxfCEaTbmhvBPtn"

var safeSetNames = map[string]bool{
	"errexit": true, "nounset": true, "xtrace": true, "noglob": true, "noclobber": true, "pipefail": true,
	"errtrace": true, "functrace": true, "allexport": true, "notify": true, "monitor": true, "hashall": true,
	"verbose": true, "braceexpand": true, "physical": true, "onecmd": true, "noexec": true, "ignoreeof": true,
}

// safeShopts are the shopt options that change neither how a line is
// read nor which files a pattern matches.
var safeShopts = map[string]bool{
	"lastpipe": true, "inherit_errexit": true, "checkwinsize": true, "huponexit": true, "cmdhist": true,
	"lithist": true, "histappend": true, "xpg_echo": true, "gnu_errfmt": true, "shift_verbose": true,
	"execfail": true, "checkhash": true, "checkjobs": true, "nocasematch": true,
}

// shellOptions judges a set or shopt command: one that switches on (or
// off) an option outside the lists above makes the rest of the line
// something this package cannot claim to have read.
func (v *bashView) shellOptions(name string, args []word) {
	bad := func(what string) {
		v.setOpaque(name + " changes how the shell reads the rest of the line (" + what + ")")
	}
	for i := 0; i < len(args); i++ {
		if !args[i].static() {
			bad(args[i].raw)
			return
		}
		l := args[i].joined()
		if name == "shopt" {
			switch {
			case l == "-o":
				// The names that follow are set's.
				for _, a := range args[i+1:] {
					if a.static() && !strings.HasPrefix(a.joined(), "-") && !safeSetNames[a.joined()] {
						bad(a.raw)
					}
				}
				return
			case strings.HasPrefix(l, "-"):
			case !safeShopts[l]:
				bad(l)
			}
			continue
		}
		if l == "--" || l == "-" || l == "" || l[0] != '-' && l[0] != '+' {
			return // what follows are the positional parameters
		}
		for k := 1; k < len(l); k++ {
			switch {
			case l[k] == 'o':
				i++
				if i < len(args) && !(args[i].static() && safeSetNames[args[i].joined()]) {
					bad(args[i].raw)
				}
			case strings.IndexByte(safeSetFlags, l[k]) < 0:
				bad(l)
			}
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

// allKnown reports whether every word is known ahead of time and is one
// word: nothing for the shell to expand into file names or alternatives.
func allKnown(ws []word) bool {
	for _, w := range ws {
		if !w.static() || w.brace || w.glob {
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

// filledText is the command with the variables the line itself set
// filled in, when that makes an argument known that was not: one more
// text for deny and ask rules to match ("x=push; git $x").
func (e *Engine) filledText(v *bashView, c *simpleCmd) string {
	if len(c.words) < 2 {
		return ""
	}
	name, ok := e.progName(c.words[0])
	if !ok {
		return ""
	}
	parts := []string{baseName(name)}
	any := false
	for _, w := range c.words[1:] {
		if w.static() {
			parts = append(parts, w.lit())
			continue
		}
		val, ok := e.filled(v, w)
		if !ok {
			return ""
		}
		any = true
		parts = append(parts, val)
	}
	if !any {
		return ""
	}
	return strings.Join(parts, " ")
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
			// (A word that is only known when the line runs may be any of
			// these.)
			l := w.joined()
			if !w.static() || strings.HasPrefix(l, "-exec") || strings.HasPrefix(l, "-ok") || strings.HasPrefix(l, "-fprint") ||
				l == "-delete" || l == "-rm" || l == "-fls" || l == "-files0-from" {
				return true
			}
		}
	}
	return false
}
