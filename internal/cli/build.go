package cli

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse reads a command line (the arguments after the program's name) with
// this table and returns what it asks for. See the package's Parse.
func (t *Table) Parse(args []string, env Env) (Invocation, error) {
	p, err := t.walk(args)
	if err != nil {
		return Invocation{}, err
	}
	inv, err := t.build(p, env)
	if err != nil {
		return Invocation{}, err
	}
	return inv, nil
}

// build turns a command line that has been taken apart into an Invocation.
// The order of its checks is the order a person would want to hear about
// them: help first; then a first word that looks like a mistyped command;
// then each value on its own; then anything that is not built yet (there is
// no use correcting how a flag is used if it cannot work anyway); then the
// number of arguments; then what the flags mean together.
func (t *Table) build(p *parsed, env Env) (Invocation, *Error) {
	c := p.cmd
	if p.help {
		return Invocation{Command: CmdHelp, HelpFor: c.path()}, nil
	}
	if c.needSub {
		var names []string
		for _, s := range c.subs {
			names = append(names, s.name)
		}
		return Invocation{}, &Error{Kind: ErrUsage, Typed: c.title(), Message: fmt.Sprintf(
			"%s needs one of its commands after it, so %s: %s. %q explains them. %s",
			c.title(), nothing(c), oneOf(names, "or"), c.title()+" --help", asPrompt(p.name))}
	}
	explicit := p.explicitPrint()
	if explicit {
		p.printLetters()
	}
	if p.given(idVersion) != nil {
		return Invocation{Command: CmdVersion}, nil
	}
	if c == t.root && !explicit {
		if err := p.mistyped(); err != nil {
			return Invocation{}, err
		}
	}
	inv := Invocation{Command: c.cmd}
	for k := range p.occs {
		o := &p.occs[k]
		v, err := p.check(o)
		if err != nil {
			return Invocation{}, err
		}
		if o.f.set != nil {
			o.f.set(&inv, v)
		}
	}
	if err := p.notBuilt(); err != nil {
		return Invocation{}, err
	}
	if err := p.arity(); err != nil {
		return Invocation{}, err
	}
	if c.take != nil {
		c.take(&inv, p.args)
	} else if len(p.args) > 0 {
		inv.Args = append([]string(nil), p.args...)
	}
	if c.finish != nil {
		if err := c.finish(&inv, p, env); err != nil {
			return Invocation{}, err
		}
	}
	return inv, nil
}

// explicitPrint reports whether the line asks for print mode in so many
// words: -p.
func (p *parsed) explicitPrint() bool {
	return p.given(idPrint) != nil
}

// printLetters gives each one-letter flag the meaning it has in print mode,
// where that differs: -v is --version on a plain `gnosh` and --verbose
// with -p.
func (p *parsed) printLetters() {
	for k := range p.occs {
		o := &p.occs[k]
		r := []rune(o.typed)
		if len(r) != 2 || r[1] == '-' {
			continue
		}
		for _, f := range p.cmd.flags {
			if f.printLetter == r[1] {
				o.f = f
			}
		}
	}
}

// mistyped reports a first word that is one slip away from a command's
// name, on a line that would make sense for that command: the word alone,
// or the word followed by one of the command's own commands. Anything else
// is a prompt. The slip is refused rather than sent to a model as a prompt,
// and the message says how to send it as one.
func (p *parsed) mistyped() *Error {
	if p.early == 0 || len(p.args) == 0 {
		return nil
	}
	word := p.args[0]
	if hasSpace(word) {
		return nil
	}
	near, name := nearCommand(p.t.root, word)
	if near == nil {
		return nil
	}
	if rest := p.args[1:]; len(rest) > 0 {
		if len(near.subs) == 0 {
			return nil
		}
		if near.sub(rest[0]) == nil {
			if s, _ := nearCommand(near, rest[0]); s == nil {
				return nil
			}
		}
	}
	return &Error{Kind: ErrUnknownCommand, Typed: word, Suggestion: name, Message: fmt.Sprintf(
		"%s is not a gnosh command, so nothing was started. Did you mean %s? If it is your prompt, put -- before it: gnosh -- %s",
		shown(word), name, shown(word))}
}

// check checks one flag's value by itself and returns it ready to record.
func (p *parsed) check(o *occurrence) (flagValue, *Error) {
	f := o.f
	c := p.cmd
	bad := func(format string, a ...any) (flagValue, *Error) {
		return flagValue{}, &Error{Kind: ErrBadValue, Typed: o.typed, Message: fmt.Sprintf(format, a...)}
	}
	switch f.kind {
	case kindBool:
		return flagValue{}, nil
	case kindList:
		var items []string
		for _, raw := range o.raw {
			if f.list != listRules {
				if blank(raw) {
					continue
				}
				items = append(items, raw)
				continue
			}
			its, problem := splitRules(raw)
			if problem != "" {
				return bad("%s %s: %s. Nothing was %s.", o.typed, shown(raw), problem, done(c))
			}
			items = append(items, its...)
		}
		if len(items) == 0 && !f.emptyOK {
			return bad("%s was given an empty list, so %s. Give it <%s>, or leave the flag out.",
				o.typed, nothing(c), f.arg)
		}
		if f.check != nil {
			for _, it := range items {
				if problem := f.check(it); problem != "" {
					return bad("%s %s %s, so %s.", o.typed, shown(it), problem, nothing(c))
				}
			}
		}
		return flagValue{items: items, has: true}, nil
	}
	if !o.has {
		return flagValue{}, nil
	}
	v := o.value
	if blank(v) && !f.emptyOK {
		leave := "leave the flag out"
		if f.kind != kindText {
			leave = "type " + o.typed + " by itself"
		}
		return bad("%s was given an empty value, so %s. Give it <%s>, or %s.", o.typed, nothing(c), f.arg, leave)
	}
	out := flagValue{text: v, has: true}
	if len(f.choices) > 0 {
		ch := f.choice(v)
		if ch == nil {
			names := f.choiceNames()
			msg := fmt.Sprintf("%s takes %s, and %s is none of them, so %s.", o.typed, oneOf(names, "or"), shown(v), nothing(c))
			if len(names) == 1 {
				msg = fmt.Sprintf("%s takes %s and no other value, and %s was given, so %s.", o.typed, names[0], shown(v), nothing(c))
			}
			e := &Error{Kind: ErrBadValue, Typed: o.typed}
			var all []string
			for _, known := range f.choices {
				if known.refuse == "" {
					all = append(all, known.name)
				}
			}
			if near := nearest(v, all, slips(len(v))); near != "" {
				e.Suggestion = near
				msg += fmt.Sprintf(" Did you mean %s?", near)
			}
			e.Message = msg
			return flagValue{}, e
		}
		if ch.refuse != "" {
			return bad("%s %s: %s, so %s. It takes %s.", o.typed, shown(v), ch.refuse, nothing(c), oneOf(f.choiceNames(), "or"))
		}
		if ch.means != "" {
			ch = f.choice(ch.means)
		}
		o.choice = ch
		out.text = ch.name
	}
	if f.number {
		n, err := strconv.Atoi(v)
		if err != nil || n < f.min {
			return bad("%s takes a whole number of %d or more, and %s is not one, so %s.", o.typed, f.min, shown(v), nothing(c))
		}
		out.n = n
	}
	if f.check != nil {
		if problem := f.check(v); problem != "" {
			return bad("%s %s %s, so %s.", o.typed, shown(v), problem, nothing(c))
		}
	}
	return out, nil
}

// done is nothing's last word alone: "started" or "done".
func done(c *command) string {
	if c.session {
		return "started"
	}
	return "done"
}

// notBuiltShown is how many not-built flags a message names before it
// counts the rest. Error.NotBuilt always has them all.
const notBuiltShown = 8

// notBuilt refuses a line that uses a command, a flag or a value whose
// feature is not built yet. For a flag given more than once only the value
// that counts, the last, is looked at.
func (p *parsed) notBuilt() *Error {
	c := p.cmd
	if c.status == NotBuilt {
		return &Error{Kind: ErrNotBuilt, Typed: c.title(), NotBuilt: []string{strings.Join(c.path(), " ")}, Message: fmt.Sprintf(
			"%s is not built yet in this build of gnosh, so nothing was done. %q says what it is for.",
			c.title(), c.title()+" --help")}
	}
	last := make(map[*flag]int, len(p.occs))
	for k := range p.occs {
		last[p.occs[k].f] = k
	}
	var typed, names []string
	for k := range p.occs {
		o := &p.occs[k]
		if last[o.f] != k {
			continue
		}
		switch {
		case o.f.status == NotBuilt:
			typed = append(typed, o.typed)
			names = append(names, o.f.id)
		case o.choice != nil && o.choice.status == NotBuilt:
			typed = append(typed, o.typed+" "+o.choice.name)
			names = append(names, o.f.id+"="+o.choice.name)
		}
	}
	switch len(names) {
	case 0:
		return nil
	case 1:
		return &Error{Kind: ErrNotBuilt, Typed: typed[0], NotBuilt: names, Message: fmt.Sprintf(
			"%s is not built yet in this build of gnosh, so %s. Leave it out for now.",
			typed[0], nothing(c))}
	}
	shownTyped := typed
	if len(shownTyped) > notBuiltShown {
		shownTyped = append(append([]string{}, typed[:notBuiltShown]...), fmt.Sprintf("%d more", len(typed)-notBuiltShown))
	}
	return &Error{Kind: ErrNotBuilt, Typed: typed[0], NotBuilt: names, Message: fmt.Sprintf(
		"%s are not built yet in this build of gnosh, so %s. Leave them out for now.",
		oneOf(shownTyped, "and"), nothing(c))}
}

// arity checks how many arguments were left for the command.
func (p *parsed) arity() *Error {
	c := p.cmd
	n := len(p.args)
	written := strings.TrimSpace(c.title() + " " + c.argsUsage)
	switch {
	case n < c.minArgs:
		return &Error{Kind: ErrUsage, Typed: c.title(), Message: fmt.Sprintf(
			"%s needs more after it, so %s. It is written: %s", c.title(), nothing(c), written)}
	case c.maxArgs == 0 && n > 0:
		return &Error{Kind: ErrUsage, Typed: p.args[0], Message: fmt.Sprintf(
			"%s was typed after %s, which takes nothing after it but flags, so %s. If the whole line is a prompt, put it in quotes or after --.",
			shown(p.args[0]), c.title(), nothing(c))}
	case c.maxArgs > 0 && n > c.maxArgs:
		return &Error{Kind: ErrUsage, Typed: p.args[c.maxArgs], Message: fmt.Sprintf(
			"%s is one argument too many for %s, so %s. It is written: %s. An argument with spaces in it needs quotes around it, and so does a prompt.",
			shown(p.args[c.maxArgs]), c.title(), nothing(c), written)}
	}
	if c.session {
		return nil
	}
	// The arguments a command names must say something. Words it passes on
	// to something else (mem's) are that thing's own.
	named := n
	if c.maxArgs < 0 {
		named = min(c.minArgs, n)
	}
	for _, a := range p.args[:named] {
		if blank(a) {
			return &Error{Kind: ErrBadValue, Typed: a, Message: fmt.Sprintf(
				"An empty argument was typed after %s, so %s. It is written: %s", c.title(), nothing(c), written)}
		}
	}
	return nil
}
