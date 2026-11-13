package config

import (
	"encoding/json"
	"fmt"
	"math"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// op is one thing a config file says, ready to be put in force. Reading a
// file gives a list of ops; putting them in force, in order, gives the
// Config. Keeping the two steps apart is what lets an untrusted folder's
// file be read by the same reader and then held back op by op.
type op struct {
	// kind sorts what the op would do, in the trust package's words.
	kind string
	// text is the setting in a few words, for the trust question.
	text string
	// origin is the line that says it.
	origin Origin
	// feature is the part of gnosh the setting is for, or "" when it needs
	// none. An op for a part the binary does not have stops startup.
	feature Feature
	// what names the setting in the "not built" message: "a hook".
	what string
	// apply puts the setting in force.
	apply func(b *builder)
}

// source is the file, flag or variable being read, and what a reader needs
// to know about where it sits.
type source struct {
	// file is the path, or a flag's name for settings given on the command
	// line.
	file  string
	layer Layer
	// scope is "user", "project", "local" or "command line".
	scope string
	// baseDir is the directory the file is in.
	baseDir string
	// ruleDir is what a permission rule's "/path" is under.
	ruleDir string
	home    string
	cwd     string
	// tools is every tool name a permission rule may use; nil when the
	// calling program gave none.
	tools []string

	probs   *problems
	secrets *secrets
	ops     []op
}

// at is the Origin of a line of the source.
func (s *source) at(line int) Origin {
	return Origin{Layer: s.layer, File: s.file, Line: line}
}

// errorf records a mistake on a line of the source.
func (s *source) errorf(line int, format string, args ...any) {
	s.probs.errorf(s.file, line, format, args...)
}

// add records an op.
func (s *source) add(o op) { s.ops = append(s.ops, o) }

// readRC reads the text of a gnoshrc into ops. Every mistake is recorded
// with its line; a line with a mistake gives no op.
func readRC(s *source, text []byte, getenv func(string) (string, bool)) {
	for _, ln := range lexRC(s.file, text, getenv, s.probs) {
		name := ln.words[0].text
		d := findDirective(name)
		if d == nil {
			if why, ok := plannedDirectives[name]; ok {
				s.errorf(ln.num, "%s.", why)
				continue
			}
			s.errorf(ln.num, "%s is not a directive gnosh knows. The directives are: %s.", showName(name), directiveNames())
			continue
		}
		rest := ln.words[1:]
		switch d.name {
		case "option":
			s.option(ln, rest)
			continue
		case "permissions":
			s.permissions(ln, d, rest)
			continue
		}
		if len(rest) == 0 {
			s.errorf(ln.num, "%s needs to be followed by one of: %s.", d.name, d.subNames())
			continue
		}
		sub := d.findSub(rest[0].text)
		if sub == nil {
			if d.name == "model" {
				s.errorf(ln.num, "model %s is not something gnosh knows. A model line is model add, model remove, model large or model small: there are two slots, large and small, and no other.", showName(rest[0].text))
			} else {
				s.errorf(ln.num, "%s %s is not something gnosh knows. After %s comes one of: %s.", d.name, showName(rest[0].text), d.name, d.subNames())
			}
			continue
		}
		p, ok := s.flags(ln, rest[1:], sub.flags, d.name+" "+sub.name)
		if !ok {
			continue
		}
		switch d.name + " " + sub.name {
		case "provider add":
			s.providerAdd(ln, p)
		case "provider remove":
			s.remove(ln, p, "provider", KindRemoval, "", func(b *builder, id string, o Origin) { b.removeProvider(id, o) })
		case "model add":
			s.modelAdd(ln, p)
		case "model remove":
			s.modelRemove(ln, p)
		case "model large", "model small":
			s.modelSlot(ln, p, sub.name)
		case "host add":
			s.hostAdd(ln, p)
		case "host remove":
			s.remove(ln, p, "host", KindRemoval, FeatureHosts, func(b *builder, id string, o Origin) { b.removeHost(id, o) })
		case "hook add":
			s.hookAdd(ln, p)
		case "hook remove":
			s.hookRemove(ln, p)
		case "mcp add":
			s.mcpAdd(ln, p)
		case "mcp remove":
			s.remove(ln, p, "mcp", KindRemoval, FeatureMCP, func(b *builder, id string, o Origin) { b.removeMCP(id, o) })
		case "lsp add":
			s.lspAdd(ln, p)
		case "lsp remove":
			s.remove(ln, p, "lsp", KindRemoval, FeatureLSP, func(b *builder, id string, o Origin) { b.removeLSP(id, o) })
		}
	}
}

// flagUse is one flag as a line gave it.
type flagUse struct {
	spec *flagSpec
	// vals are the flag's value words: none for a truth flag written
	// without a value, two for a key and a value, one otherwise.
	vals []rcWord
}

// parsedLine is a line's words sorted into the ones that stand alone and
// the flags.
type parsedLine struct {
	line  int
	form  string // "provider add", for messages
	args  []rcWord
	flags []flagUse
}

// isFlag reports whether a word is written as a flag: two dashes, outside
// quotes. A value that begins with two dashes is put in quotes.
func isFlag(w rcWord) bool {
	return !w.quoted && strings.HasPrefix(w.raw, "--")
}

// flags sorts the words after a directive's name into arguments and flags,
// and checks each flag against the directive's list.
func (s *source) flags(ln rcLine, words []rcWord, specs []flagSpec, form string) (parsedLine, bool) {
	p := parsedLine{line: ln.num, form: form}
	hasSecret := slices.ContainsFunc(specs, func(f flagSpec) bool { return f.secret })
	names := func() string {
		var out []string
		for _, f := range specs {
			out = append(out, f.name)
		}
		if len(out) == 0 {
			return "none"
		}
		return list(out)
	}
	for i := 0; i < len(words); i++ {
		w := words[i]
		if !isFlag(w) {
			if len(p.flags) > 0 {
				if hasSecret {
					s.errorf(ln.num, "word %d of the line stands where a flag was expected. (It is not shown, in case it is a key.) A flag starts with two dashes; the flags of %s are: %s.", i+3, form, names())
				} else {
					s.errorf(ln.num, "%s stands where a flag was expected. A flag starts with two dashes; the flags of %s are: %s.", show(w.text), form, names())
				}
				return p, false
			}
			p.args = append(p.args, w)
			continue
		}
		name, inline, hasInline := strings.Cut(w.text, "=")
		var spec *flagSpec
		for j := range specs {
			if specs[j].name == name {
				spec = &specs[j]
			}
		}
		if spec == nil {
			s.errorf(ln.num, "%s is not a flag of %s. Its flags are: %s.", showName(name), form, names())
			return p, false
		}
		if spec.status == NotBuilt {
			s.errorf(ln.num, "%s is planned and not built yet, so this line would not do what it says. Until it is built, leave the flag out.", spec.name)
			return p, false
		}
		if !spec.many && slices.ContainsFunc(p.flags, func(u flagUse) bool { return u.spec == spec }) {
			s.errorf(ln.num, "%s is given twice on this line; it takes one value.", spec.name)
			return p, false
		}
		use := flagUse{spec: spec}
		want := 1
		if spec.kind == kPair {
			want = 2
		}
		if hasInline {
			use.vals = append(use.vals, rcWord{text: inline, quoted: true, raw: inline})
		}
		for len(use.vals) < want && i+1 < len(words) && !isFlag(words[i+1]) {
			i++
			use.vals = append(use.vals, words[i])
		}
		if spec.kind == kTruth && len(use.vals) == 1 {
			if _, ok := parseTruth(use.vals[0].text); !ok {
				s.errorf(ln.num, "%s is switched with %s, not with %s. (The value may also be left out, which means true.)", spec.name, truthWords, show(use.vals[0].text))
				return p, false
			}
		}
		if len(use.vals) < want && spec.kind != kTruth {
			if spec.kind == kPair {
				s.errorf(ln.num, "%s needs two words after it, a name and a value.", spec.name)
			} else {
				s.errorf(ln.num, "%s needs a value after it. (A value that begins with two dashes goes in quotes.)", spec.name)
			}
			return p, false
		}
		p.flags = append(p.flags, use)
	}
	return p, true
}

// get returns the one use of a flag, if the line gave it.
func (p parsedLine) get(name string) (flagUse, bool) {
	for _, u := range p.flags {
		if u.spec.name == name {
			return u, true
		}
	}
	return flagUse{}, false
}

// all returns every use of a repeatable flag, in order.
func (p parsedLine) all(name string) []flagUse {
	var out []flagUse
	for _, u := range p.flags {
		if u.spec.name == name {
			out = append(out, u)
		}
	}
	return out
}

// text returns a flag's value as text.
func (p parsedLine) text(name string) (string, bool) {
	u, ok := p.get(name)
	if !ok {
		return "", false
	}
	return u.vals[0].text, true
}

// truth returns a truth flag's value. Written without a value it is true.
func (p parsedLine) truth(name string) (bool, bool) {
	u, ok := p.get(name)
	if !ok {
		return false, false
	}
	if len(u.vals) == 0 {
		return true, true
	}
	v, _ := parseTruth(u.vals[0].text)
	return v, true
}

// whole returns a flag's value as a whole number of at least least.
func (s *source) whole(p parsedLine, name string, least int) (int, bool, bool) {
	text, ok := p.text(name)
	if !ok {
		return 0, false, true
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < least {
		s.errorf(p.line, "%s takes a whole number of at least %d, not %s.", name, least, show(text))
		return 0, true, false
	}
	return n, true, true
}

// number returns a flag's value as a number between lo and hi.
func (s *source) number(p parsedLine, name string, lo, hi float64) (float64, bool, bool) {
	text, ok := p.text(name)
	if !ok {
		return 0, false, true
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < lo || f > hi {
		if math.IsInf(hi, 1) {
			s.errorf(p.line, "%s takes a number of at least %v, not %s.", name, lo, show(text))
		} else {
			s.errorf(p.line, "%s takes a number from %v to %v, not %s.", name, lo, hi, show(text))
		}
		return 0, true, false
	}
	return f, true, true
}

// object returns a flag's value as a JSON object.
func (s *source) object(p parsedLine, name string) (json.RawMessage, map[string]any, bool, bool) {
	text, ok := p.text(name)
	if !ok {
		return nil, nil, false, true
	}
	var m map[string]any
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil || m == nil || dec.More() {
		s.errorf(p.line, "%s takes a JSON object in one word, such as '{\"id_slot\": 1}'. Put it in single quotes so that its double quotes are kept.", name)
		return nil, nil, true, false
	}
	return json.RawMessage(text), m, true, true
}

// words returns the values of a list flag: each use holds one word, split
// at commas and spaces.
func (p parsedLine) words(name string) ([]string, bool) {
	uses := p.all(name)
	if len(uses) == 0 {
		return nil, false
	}
	out := []string{}
	for _, u := range uses {
		out = append(out, strings.FieldsFunc(u.vals[0].text, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })...)
	}
	return out, true
}

// oneArg returns the single word that follows a directive's name, such as
// an ID.
func (s *source) oneArg(p parsedLine, what string) (string, bool) {
	switch {
	case len(p.args) == 0:
		s.errorf(p.line, "%s needs %s after it.", p.form, what)
		return "", false
	case len(p.args) > 1:
		s.errorf(p.line, "%s takes %s and then flags, but %d words come before the first flag. Put a value that holds spaces in quotes.", p.form, what, len(p.args))
		return "", false
	}
	return p.args[0].text, true
}

// idPattern is what an ID may look like: letters, digits, dots, dashes and
// underscores, starting with a letter or a digit.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// id returns the ID that follows a directive's name, checked.
func (s *source) id(p parsedLine, what string) (string, bool) {
	id, ok := s.oneArg(p, what)
	if !ok {
		return "", false
	}
	if !idPattern.MatchString(id) {
		s.errorf(p.line, "%s is not a usable name for %s: use letters, digits, dots, dashes and underscores, starting with a letter or a digit.", showName(id), p.form)
		return "", false
	}
	return id, true
}

// remove reads "<directive> remove <id>".
func (s *source) remove(ln rcLine, p parsedLine, what, kind string, feature Feature, do func(*builder, string, Origin)) {
	id, ok := s.id(p, "the name to remove")
	if !ok {
		return
	}
	o := s.at(ln.num)
	s.add(op{kind: kind, text: what + " remove " + id, origin: o, feature: feature, what: "removing a " + what,
		apply: func(b *builder) { do(b, id, o) }})
}

// providerChange is what one provider add line sets. A nil field was not
// on the line and is left as it was.
type providerChange struct {
	typ          *providerType
	name         *string
	urls         []string
	hasURLs      bool
	key          *Secret
	keyFrom      string
	discover     *bool
	extraBody    map[string]any
	hasExtraBody bool
	headers      []Header
	hasHeaders   bool
	disable      *bool
	prefix       *string
	options      json.RawMessage
	flatRate     *bool
}

// providerAdd reads "provider add <id> [flags]".
func (s *source) providerAdd(ln rcLine, p parsedLine) {
	id, ok := s.id(p, "an id for the provider")
	if !ok {
		return
	}
	var ch providerChange
	good := true
	if text, ok := p.text("--type"); ok {
		if ch.typ = findProviderType(text); ch.typ == nil {
			s.errorf(ln.num, "%s is not a provider type gnosh knows. The types are: %s.", showName(text), list(providerTypeNames()))
			good = false
		}
	}
	for _, u := range p.all("--base-url") {
		ch.hasURLs = true
		raw := u.vals[0].text
		s.secrets.addURL(raw)
		if why := checkURL(raw); why != "" {
			s.errorf(ln.num, "the address given to --base-url cannot be used: %s. An address looks like http://127.0.0.1:8080.", why)
			good = false
			continue
		}
		ch.urls = append(ch.urls, raw)
	}
	if text, ok := p.text("--name"); ok {
		ch.name = &text
	}
	if u, ok := p.get("--api-key"); ok {
		key := NewSecret(u.vals[0].text)
		s.secrets.add(key.Reveal())
		ch.key = &key
		ch.keyFrom = variableOnly(u.vals[0].raw)
	}
	if v, ok := p.truth("--discover-models"); ok {
		ch.discover = &v
	}
	if _, m, has, ok := s.object(p, "--extra-body"); has {
		ch.extraBody, ch.hasExtraBody = m, ok
		good = good && ok
	}
	for _, u := range p.all("--extra-header") {
		ch.hasHeaders = true
		name, value := u.vals[0].text, u.vals[1].text
		s.secrets.add(value)
		if !isHeaderName(name) {
			s.errorf(ln.num, "%s is not a usable HTTP header name for --extra-header.", showName(name))
			good = false
			continue
		}
		ch.headers = append(ch.headers, Header{Name: name, Value: NewSecret(value)})
	}
	if v, ok := p.truth("--disable"); ok {
		ch.disable = &v
	}
	if text, ok := p.text("--system-prompt-prefix"); ok {
		ch.prefix = &text
	}
	if raw, _, has, ok := s.object(p, "--provider-options"); has {
		ch.options = raw
		good = good && ok
	}
	if v, ok := p.truth("--flat-rate"); ok {
		ch.flatRate = &v
	}
	if !good {
		return
	}
	text := "provider " + id
	if ch.typ != nil {
		text += " (" + ch.typ.name + ")"
	}
	for _, u := range ch.urls {
		text += " at " + displayURL(u)
	}
	o := s.at(ln.num)
	s.add(op{kind: KindProvider, text: text, origin: o, apply: func(b *builder) { b.addProvider(id, ch, o) }})
}

// variableOnly returns "$NAME" when a word as written is nothing but one
// variable, bare or in double quotes, and "" otherwise. It is how a key
// that came from the environment is shown: by the variable's name.
func variableOnly(raw string) string {
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}
	name, ok := strings.CutPrefix(raw, "$")
	if !ok {
		return ""
	}
	if strings.HasPrefix(name, "{") && strings.HasSuffix(name, "}") {
		name = name[1 : len(name)-1]
		name, _, _ = strings.Cut(name, ":?")
	}
	if !isName(name) {
		return ""
	}
	return "$" + name
}

// isHeaderName reports whether s can be the name of an HTTP header.
func isHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '-' || c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

// modelRef splits "<provider>/<id>" at its first slash.
func (s *source) modelRef(line int, text, form string) (ModelRef, bool) {
	prov, id, ok := strings.Cut(text, "/")
	if !ok || prov == "" || id == "" {
		s.errorf(line, "%s names a model as <provider>/<id>, the form gnosh models prints; %s has no provider in front.", form, showName(text))
		return ModelRef{}, false
	}
	if !idPattern.MatchString(prov) || strings.ContainsAny(id, " \t") {
		s.errorf(line, "%s is not a usable model name for %s: it is written <provider>/<id>.", showName(text), form)
		return ModelRef{}, false
	}
	return ModelRef{Provider: prov, ID: id}, true
}

// levelPattern is what a thinking level may look like.
var levelPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// modelChange is what one model add line sets.
type modelChange struct {
	name      *string
	window    *int
	maxTokens *int
	levels    []string
	hasLevels bool
	canReason *bool
	images    *bool
	effort    *string
	prices    [4]*float64
}

// modelAdd reads "model add <provider>/<id> [flags]".
func (s *source) modelAdd(ln rcLine, p parsedLine) {
	arg, ok := s.oneArg(p, "a model, as <provider>/<id>,")
	if !ok {
		return
	}
	ref, ok := s.modelRef(ln.num, arg, p.form)
	if !ok {
		return
	}
	var ch modelChange
	good := true
	if text, ok := p.text("--name"); ok {
		ch.name = &text
	}
	if n, has, ok := s.whole(p, "--context-window", 1); has {
		ch.window = &n
		good = good && ok
	}
	if n, has, ok := s.whole(p, "--default-max-tokens", 1); has {
		ch.maxTokens = &n
		good = good && ok
	}
	if levels, ok := p.words("--reasoning-levels"); ok {
		ch.levels, ch.hasLevels = levels, true
		if len(levels) == 0 {
			s.errorf(ln.num, "--reasoning-levels needs at least one level, such as \"off on\" or \"low medium high\".")
			good = false
		}
		for _, l := range levels {
			if !levelPattern.MatchString(l) {
				s.errorf(ln.num, "%s is not a usable thinking level for --reasoning-levels: a level is a lower-case word such as off, on, low, medium or high.", showName(l))
				good = false
			}
		}
	}
	if v, ok := p.truth("--can-reason"); ok {
		ch.canReason = &v
	}
	if v, ok := p.truth("--supports-images"); ok {
		ch.images = &v
	}
	if text, ok := p.text("--reasoning-effort"); ok {
		ch.effort = &text
		if !levelPattern.MatchString(text) {
			s.errorf(ln.num, "%s is not a usable thinking level for --reasoning-effort: a level is a lower-case word such as off, on, low, medium or high.", showName(text))
			good = false
		}
	}
	for i, name := range []string{"--price-input", "--price-output", "--price-cache-create", "--price-cache-hit"} {
		if f, has, ok := s.number(p, name, 0, math.Inf(1)); has {
			ch.prices[i] = &f
			good = good && ok
		}
	}
	if !good {
		return
	}
	o := s.at(ln.num)
	s.add(op{kind: KindModel, text: "model " + ref.String(), origin: o, apply: func(b *builder) { b.addModel(ref, ch, o) }})
}

// modelRemove reads "model remove <provider>/<id>".
func (s *source) modelRemove(ln rcLine, p parsedLine) {
	arg, ok := s.oneArg(p, "a model, as <provider>/<id>,")
	if !ok {
		return
	}
	ref, ok := s.modelRef(ln.num, arg, p.form)
	if !ok {
		return
	}
	o := s.at(ln.num)
	s.add(op{kind: KindRemoval, text: "model remove " + ref.String(), origin: o, apply: func(b *builder) { b.removeModel(ref, o) }})
}

// slotChange is what one model large or model small line sets.
type slotChange struct {
	ref              *ModelRef
	thinking         *string
	temperature      *float64
	maxTokens        *int
	topP             *float64
	topK             *int
	frequencyPenalty *float64
	presencePenalty  *float64
	options          json.RawMessage
}

// modelSlot reads "model large [<provider>/<id>] [flags]" and the same for
// small.
func (s *source) modelSlot(ln rcLine, p parsedLine, slot string) {
	var ch slotChange
	good := true
	switch len(p.args) {
	case 0:
	case 1:
		ref, ok := s.modelRef(ln.num, p.args[0].text, p.form)
		if !ok {
			return
		}
		ch.ref = &ref
	default:
		s.errorf(ln.num, "%s takes one model, as <provider>/<id>, and then flags, but %d words come before the first flag.", p.form, len(p.args))
		return
	}
	think, hasThink := p.truth("--think")
	effort, hasEffort := p.text("--reasoning-effort")
	switch {
	case hasThink && hasEffort:
		s.errorf(ln.num, "--think and --reasoning-effort both set the thinking level; give one of them.")
		good = false
	case hasThink && think:
		on := "on"
		ch.thinking = &on
	case hasThink:
		off := "off"
		ch.thinking = &off
	case hasEffort:
		if !levelPattern.MatchString(effort) {
			s.errorf(ln.num, "%s is not a usable thinking level for --reasoning-effort: a level is a lower-case word such as off, on, low, medium or high.", showName(effort))
			good = false
		}
		ch.thinking = &effort
	}
	if f, has, ok := s.number(p, "--temperature", 0, 2); has {
		ch.temperature = &f
		good = good && ok
	}
	if n, has, ok := s.whole(p, "--max-tokens", 1); has {
		ch.maxTokens = &n
		good = good && ok
	}
	if f, has, ok := s.number(p, "--top-p", 0, 1); has {
		ch.topP = &f
		good = good && ok
	}
	if n, has, ok := s.whole(p, "--top-k", 0); has {
		ch.topK = &n
		good = good && ok
	}
	if f, has, ok := s.number(p, "--frequency-penalty", -2, 2); has {
		ch.frequencyPenalty = &f
		good = good && ok
	}
	if f, has, ok := s.number(p, "--presence-penalty", -2, 2); has {
		ch.presencePenalty = &f
		good = good && ok
	}
	if raw, _, has, ok := s.object(p, "--provider-options"); has {
		ch.options = raw
		good = good && ok
	}
	if !good {
		return
	}
	if ch.ref == nil && len(p.flags) == 0 {
		s.errorf(ln.num, "%s needs a model, as <provider>/<id>, or a flag after it.", p.form)
		return
	}
	text := "model " + slot
	if ch.ref != nil {
		text += " " + ch.ref.String()
	}
	o := s.at(ln.num)
	s.add(op{kind: KindModel, text: text, origin: o, apply: func(b *builder) { b.setSlot(slot, ch, o) }})
}

// sshDestination is what a host's destination may look like: user@address
// or an alias, with nothing a shell or ssh would read as more than a name.
var sshDestination = regexp.MustCompile(`^[A-Za-z0-9_.\[\]:%@-]+$`)

// hostAdd reads "host add <name> --ssh <destination> [--description ...]".
func (s *source) hostAdd(ln rcLine, p parsedLine) {
	name, ok := s.id(p, "a name for the machine")
	if !ok {
		return
	}
	if name == "local" {
		s.errorf(ln.num, "a host cannot be named local: that word means this machine. Choose another name.")
		return
	}
	dest, ok := p.text("--ssh")
	switch {
	case !ok:
		s.errorf(ln.num, "host add %s needs --ssh and a destination, such as --ssh user@203.0.113.10.", name)
		return
	case dest == "":
		s.errorf(ln.num, "the destination given to --ssh is empty. It is user@address, or a Host alias from your ssh configuration.")
		return
	case strings.HasPrefix(dest, "-"):
		s.errorf(ln.num, "the destination given to --ssh starts with a dash, so ssh would read it as an option. It is user@address, or a Host alias from your ssh configuration.")
		return
	case !sshDestination.MatchString(dest):
		s.errorf(ln.num, "the destination given to --ssh holds characters a shell would act on. It is user@address, or a Host alias from your ssh configuration; keys, ports and jump hosts belong in ~/.ssh/config.")
		return
	}
	h := Host{Name: name, SSH: dest, Origin: s.at(ln.num)}
	h.Description, _ = p.text("--description")
	s.add(op{kind: KindHost, text: "host " + name + " → " + dest, origin: h.Origin, feature: FeatureHosts, what: "a host",
		apply: func(b *builder) { b.addHost(h) }})
}

// hookEventFor checks the event a hook line names.
func (s *source) hookEventFor(line int, name string) *hookEvent {
	e := findHookEvent(name)
	if e == nil {
		hint := ""
		if near := nearHookEvent(name); near != "" {
			hint = fmt.Sprintf(" Did you mean %s? Event names are exact: no other letter case, no underscores.", near)
		}
		s.errorf(line, "%s is not a hook event gnosh fires, so a hook on it would never run.%s The events are: %s.", showName(name), hint, list(HookEventNames()))
	}
	return e
}

// checkMatcher says what is wrong with a hook's matcher for its event, or
// "".
func checkMatcher(e *hookEvent, matcher string) string {
	if matcher == "" {
		return ""
	}
	if e.matcher == "" {
		return fmt.Sprintf("%s has nothing for a matcher to match, so a hook with one would never run. Take the matcher away", e.name)
	}
	if matcher == "*" || isExactMatcher(matcher) {
		return ""
	}
	if _, err := regexp.Compile(matcher); err != nil {
		return fmt.Sprintf("the matcher %s is read as a regular expression (it holds more than names, commas and |) and cannot be compiled: %v", show(matcher), err)
	}
	return ""
}

// isExactMatcher reports whether a matcher is a list of exact names: only
// letters, digits, underscores, dashes, spaces, commas and |.
func isExactMatcher(m string) bool {
	for i := 0; i < len(m); i++ {
		c := m[i]
		if !(c == '_' || c == '-' || c == ' ' || c == ',' || c == '|' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

// hookAdd reads "hook add <event> --command <command> [--name] [--matcher]
// [--timeout]".
func (s *source) hookAdd(ln rcLine, p parsedLine) {
	name, ok := s.oneArg(p, "an event")
	if !ok {
		return
	}
	e := s.hookEventFor(ln.num, name)
	if e == nil {
		return
	}
	h := Hook{Event: e.name, Scope: s.scope, BaseDir: s.baseDir, Origin: s.at(ln.num)}
	if h.Command, ok = p.text("--command"); !ok || strings.TrimSpace(h.Command) == "" {
		s.errorf(ln.num, "hook add %s needs --command and the command to run.", e.name)
		return
	}
	if h.Name, ok = p.text("--name"); ok && !idPattern.MatchString(h.Name) {
		s.errorf(ln.num, "%s is not a usable hook name: use letters, digits, dots, dashes and underscores.", showName(h.Name))
		return
	}
	h.Matcher, _ = p.text("--matcher")
	if why := checkMatcher(e, h.Matcher); why != "" {
		s.errorf(ln.num, "%s.", why)
		return
	}
	if secs, has, ok := s.number(p, "--timeout", 0, 1e7); has {
		if !ok {
			return
		}
		if secs == 0 {
			s.errorf(ln.num, "--timeout takes a number of seconds above zero.")
			return
		}
		h.Timeout = time.Duration(secs * float64(time.Second))
	}
	s.add(op{kind: KindHook, text: "hook " + e.name + ": " + h.Command, origin: h.Origin, feature: FeatureHooks, what: "a hook",
		apply: func(b *builder) { b.addHook(h) }})
}

// hookRemove reads "hook remove <event> [--name <name>]".
func (s *source) hookRemove(ln rcLine, p parsedLine) {
	name, ok := s.oneArg(p, "an event")
	if !ok {
		return
	}
	e := s.hookEventFor(ln.num, name)
	if e == nil {
		return
	}
	hookName, _ := p.text("--name")
	o := s.at(ln.num)
	s.add(op{kind: KindHookOff, text: strings.TrimSpace("hook remove " + e.name + " " + hookName), origin: o, feature: FeatureHooks, what: "removing a hook",
		apply: func(b *builder) { b.removeHooks(e.name, hookName, o) }})
}

// permissions reads "permissions allow <rule>..." and "permissions deny
// <rule>...".
func (s *source) permissions(ln rcLine, d *directiveSpec, rest []rcWord) {
	if len(rest) == 0 || d.findSub(rest[0].text) == nil {
		got := "nothing"
		if len(rest) > 0 {
			got = showName(rest[0].text)
		}
		s.errorf(ln.num, "permissions is followed by allow or deny and then one or more rules, not by %s.", got)
		return
	}
	which := rest[0].text
	if len(rest) == 1 {
		s.errorf(ln.num, "permissions %s needs at least one rule after it: a tool name such as Read, or a tool with a pattern such as 'Bash(git diff *)'.", which)
		return
	}
	for _, w := range rest[1:] {
		if isFlag(w) {
			s.errorf(ln.num, "permissions %s takes rules and no flags; %s looks like a flag. Put a rule that begins with dashes in quotes.", which, showName(w.text))
			return
		}
		s.rule(which, w.text, s.at(ln.num))
	}
}

// rule checks one permission rule and records it.
func (s *source) rule(which, text string, o Origin) {
	if why := checkRule(which, text, s.tools); why != "" {
		s.probs.at(Error, o, "the %s rule %s cannot be used: %s.", which, show(text), why)
		return
	}
	kind := map[string]string{"allow": KindAllowRule, "ask": KindAskRule, "deny": KindDenyRule}[which]
	file, scope, dir := s.file, s.scope, s.ruleDir
	if o.File == "" {
		file = ""
	}
	s.add(op{kind: kind, text: which + " " + text, origin: o, feature: FeaturePermissions, what: "a permission rule",
		apply: func(b *builder) {
			if b.in.Tools == nil && !strings.HasPrefix(text, "mcp__") {
				b.probs.at(Error, o, "the %s rule %s cannot be checked: this gnosh was started without its list of tool names, so a tool's name cannot be told from a misspelling.", which, show(text))
			}
			b.addRule(which, Rule{Text: text, Origin: o}, scope, file, dir)
		}})
}

// checkRule says why a permission rule cannot be used, or "". It checks
// the rule's shape and that the tool it names is one gnosh has. What the
// pattern in the parentheses means is left to the permission package.
func checkRule(which, text string, tools []string) string {
	if text == "" {
		return "it is empty"
	}
	name := text
	if i := strings.IndexByte(text, '('); i >= 0 {
		if !strings.HasSuffix(text, ")") {
			return "the parenthesis it opens is not closed at its end"
		}
		name = text[:i]
	} else if strings.ContainsAny(text, ")") {
		return "it closes a parenthesis it never opened"
	}
	if name == "" {
		return "no tool name comes before the parenthesis"
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c == '_' || c == '-' || c == '*' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return fmt.Sprintf("%s is not a tool name: a rule is a tool name, or a tool name with a pattern in parentheses", showName(name))
		}
	}
	glob := strings.Contains(name, "*")
	if strings.HasPrefix(name, "mcp__") {
		if which != "allow" {
			return "it names a tool of an MCP server, and such a rule would do nothing here: those tools are only known once the server connects. To keep a server's tools away, do not add the server"
		}
		if glob {
			server, _, ok := strings.Cut(strings.TrimPrefix(name, "mcp__"), "__")
			if !ok || server == "" || strings.Contains(server, "*") {
				return "an allow rule may use * only after a whole server name, as in mcp__server__get_*"
			}
		}
		if name != text {
			return "a rule for an MCP tool takes no pattern in parentheses"
		}
		return ""
	}
	if glob && which == "allow" {
		return "an allow rule cannot use * in the tool's name: it would let every matching tool run without asking. Name each tool"
	}
	if tools == nil {
		// The calling program gave no list of tools, so the name cannot be
		// told from a misspelling here. That is said when the rule is put
		// in force, and not for a rule that is only held.
		return ""
	}
	if glob {
		for _, t := range tools {
			if ok, _ := path.Match(name, t); ok {
				return ""
			}
		}
		return fmt.Sprintf("no tool's name matches %s. The tools are: %s", show(name), list(tools))
	}
	if !slices.Contains(tools, name) {
		hint := ""
		for _, t := range tools {
			if strings.EqualFold(t, name) {
				hint = fmt.Sprintf(" Did you mean %s? Tool names are exact.", t)
			}
		}
		return fmt.Sprintf("%s is not a tool gnosh has, so the rule would do nothing.%s The tools are: %s", showName(name), hint, list(tools))
	}
	return ""
}

// SplitRules splits a list of permission rules written in one argument, as
// --allowedTools "Bash(git *) Edit" or "Bash,Read". It splits at commas and
// spaces, but not inside parentheses.
func SplitRules(s string) []string {
	var out []string
	depth, start := 0, -1
	for i, r := range s {
		switch {
		case r == '(':
			depth++
		case r == ')' && depth > 0:
			depth--
		}
		if depth == 0 && (r == ',' || r == ' ' || r == '\t' || r == '\n') {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

// pairs reads the uses of a repeatable key-and-value flag as variables.
func (s *source) pairs(p parsedLine, flag string, o Origin) ([]EnvVar, bool) {
	var out []EnvVar
	for _, u := range p.all(flag) {
		name, value := u.vals[0].text, u.vals[1].text
		s.secrets.add(value)
		if !isName(name) {
			s.errorf(p.line, "%s is not a usable variable name for %s.", showName(name), flag)
			return nil, false
		}
		out = append(out, EnvVar{Name: name, Value: NewSecret(value), Origin: o})
	}
	return out, true
}

// seconds returns a --timeout flag's value as a length of time.
func (s *source) seconds(p parsedLine) (time.Duration, bool) {
	secs, has, ok := s.number(p, "--timeout", 0, 1e7)
	if !has {
		return 0, true
	}
	if ok && secs == 0 {
		s.errorf(p.line, "--timeout takes a number of seconds above zero.")
		ok = false
	}
	return time.Duration(secs * float64(time.Second)), ok
}

// mcpAdd reads "mcp add <name> [flags]".
func (s *source) mcpAdd(ln rcLine, p parsedLine) {
	name, ok := s.id(p, "a name for the server")
	if !ok {
		return
	}
	m := MCPServer{Name: name, Type: "stdio", Origin: s.at(ln.num)}
	if t, ok := p.text("--type"); ok {
		if t != "stdio" && t != "sse" && t != "http" {
			s.errorf(ln.num, "%s is not an MCP server type gnosh knows. The types are stdio (the default), sse and http.", showName(t))
			return
		}
		m.Type = t
	}
	if cmd, ok := p.text("--command"); ok {
		m.Command = expandHome(cmd, s.home)
	}
	for _, u := range p.all("--args") {
		m.Args = append(m.Args, u.vals[0].text)
	}
	if m.Env, ok = s.pairs(p, "--env", m.Origin); !ok {
		return
	}
	m.URL, _ = p.text("--url")
	s.secrets.addURL(m.URL)
	for _, u := range p.all("--header") {
		s.secrets.add(u.vals[1].text)
		if !isHeaderName(u.vals[0].text) {
			s.errorf(ln.num, "%s is not a usable HTTP header name for --header.", showName(u.vals[0].text))
			return
		}
		m.Headers = append(m.Headers, Header{Name: u.vals[0].text, Value: NewSecret(u.vals[1].text)})
	}
	if m.Timeout, ok = s.seconds(p); !ok {
		return
	}
	m.Disabled, _ = p.truth("--disabled")
	m.EnabledTools, _ = p.words("--enabled-tools")
	m.DisabledTools, _ = p.words("--disabled-tools")
	kind, feature := KindMCPLocal, FeatureMCP
	text := "mcp " + name
	// A disabled entry is not checked until it is enabled, and needs no
	// part of gnosh to be built.
	if m.Disabled {
		feature = ""
	} else if m.Type == "stdio" {
		if strings.TrimSpace(m.Command) == "" {
			s.errorf(ln.num, "mcp add %s is a stdio server, which needs --command and the program to start. (For a server at an address, give --type http and --url.)", name)
			return
		}
		if m.URL != "" || len(m.Headers) > 0 {
			s.errorf(ln.num, "mcp add %s has --url or --header, which belong to a server at an address. Give --type http (or sse), or take them away.", name)
			return
		}
	} else {
		if why := checkURL(m.URL); why != "" {
			s.errorf(ln.num, "mcp add %s is a server at an address, and the address given to --url cannot be used: %s.", name, why)
			return
		}
		if m.Command != "" || len(m.Args) > 0 {
			s.errorf(ln.num, "mcp add %s has --command or --args, which belong to a stdio server. Give --type stdio, or take them away.", name)
			return
		}
	}
	if m.Type == "stdio" {
		text += ": " + strings.TrimSpace(m.Command+" "+strings.Join(m.Args, " "))
	} else {
		kind = KindMCPOnline
		text += " at " + displayURL(m.URL)
	}
	s.add(op{kind: kind, text: text, origin: m.Origin, feature: feature, what: "an MCP server",
		apply: func(b *builder) { b.addMCP(m) }})
}

// knownLSP is what gnosh knows about a language server by its name, used
// for whatever an lsp add line leaves out.
var knownLSP = map[string]struct{ fileTypes, rootMarkers []string }{
	"gopls":  {[]string{"go"}, []string{"go.mod", "go.work"}},
	"gnopls": {[]string{"gno"}, []string{"gnomod.toml", "gnowork.toml", "gno.mod"}},
}

// lspAdd reads "lsp add <name> --command <command> [flags]".
func (s *source) lspAdd(ln rcLine, p parsedLine) {
	name, ok := s.id(p, "a name for the language server")
	if !ok {
		return
	}
	l := LSPServer{Name: name, Origin: s.at(ln.num)}
	if cmd, ok := p.text("--command"); ok {
		l.Command = expandHome(cmd, s.home)
	}
	for _, u := range p.all("--args") {
		l.Args = append(l.Args, u.vals[0].text)
	}
	if l.Env, ok = s.pairs(p, "--env", l.Origin); !ok {
		return
	}
	l.FileTypes, _ = p.words("--filetypes")
	l.RootMarkers, _ = p.words("--root-markers")
	if known, ok := knownLSP[name]; ok {
		if l.FileTypes == nil {
			l.FileTypes = slices.Clone(known.fileTypes)
		}
		if l.RootMarkers == nil {
			l.RootMarkers = slices.Clone(known.rootMarkers)
		}
	}
	if l.Timeout, ok = s.seconds(p); !ok {
		return
	}
	l.Disabled, _ = p.truth("--disabled")
	good := true
	if raw, _, has, ok := s.object(p, "--init-options"); has {
		l.InitOptions = raw
		good = good && ok
	}
	if raw, _, has, ok := s.object(p, "--options"); has {
		l.Options = raw
		good = good && ok
	}
	if !good {
		return
	}
	feature := FeatureLSP
	if l.Disabled {
		feature = ""
	} else if strings.TrimSpace(l.Command) == "" {
		s.errorf(ln.num, "lsp add %s needs --command and the language server's program.", name)
		return
	}
	s.add(op{kind: KindLSP, text: "lsp " + name + ": " + strings.TrimSpace(l.Command+" "+strings.Join(l.Args, " ")), origin: l.Origin, feature: feature, what: "a language server",
		apply: func(b *builder) { b.addLSP(l) }})
}

// option reads "option <key> [value]" and "option reset <key>".
func (s *source) option(ln rcLine, rest []rcWord) {
	if len(rest) == 0 {
		s.errorf(ln.num, "option needs a key after it, such as option request-timeout 900.")
		return
	}
	reset := rest[0].text == "reset" && !rest[0].quoted
	if reset {
		rest = rest[1:]
		if len(rest) != 1 {
			s.errorf(ln.num, "option reset is followed by one key, the option to put back as it was.")
			return
		}
	}
	key := rest[0].text
	if why := goneOption(key); why != "" {
		s.errorf(ln.num, "option %s is gone: %s. Take the line out.", showName(key), why)
		return
	}
	spec := findOption(key)
	if spec == nil {
		s.errorf(ln.num, "%s is not an option gnosh knows. The options are: %s.", showName(key), list(optionKeys()))
		return
	}
	o := s.at(ln.num)
	if reset {
		s.add(op{kind: KindOption, text: "option reset " + key, origin: o, apply: func(b *builder) { spec.reset(&b.opts) }})
		return
	}
	vals := rest[1:]
	if len(vals) > 1 {
		if spec.secret != nil {
			s.errorf(ln.num, "option %s takes one value, and %d words follow it. Put a value that holds spaces in quotes.", key, len(vals))
		} else {
			s.errorf(ln.num, "option %s takes one value, and %d words follow it (%s ...). Put a value that holds spaces in quotes.", key, len(vals), show(vals[0].text))
		}
		return
	}
	if len(vals) == 0 && spec.kind != oTruth {
		s.errorf(ln.num, "option %s needs a value after it.", key)
		return
	}
	val := ""
	if len(vals) == 1 {
		val = vals[0].text
	}
	text := "option " + key
	var set func(*Options)
	switch spec.kind {
	case oTruth:
		v := true
		if len(vals) == 1 {
			var ok bool
			if v, ok = parseTruth(val); !ok {
				s.errorf(ln.num, "option %s is switched with %s, not with %s.", key, truthWords, show(val))
				return
			}
		}
		set = func(opts *Options) { spec.truth(opts).put(v, o) }
		text += " " + strconv.FormatBool(v)
	case oInt:
		n, err := strconv.Atoi(val)
		if err != nil || spec.bounded && (n < spec.min || n > spec.max) {
			if spec.bounded && spec.max < 1<<30 {
				s.errorf(ln.num, "option %s takes a whole number from %d to %d, not %s.", key, spec.min, spec.max, show(val))
			} else if spec.bounded {
				s.errorf(ln.num, "option %s takes a whole number of at least %d, not %s.", key, spec.min, show(val))
			} else {
				s.errorf(ln.num, "option %s takes a whole number, not %s.", key, show(val))
			}
			return
		}
		set = func(opts *Options) { spec.num(opts).put(n, o) }
		text += " " + val
	case oEnum:
		if !slices.Contains(spec.enum, val) {
			s.errorf(ln.num, "option %s takes one of %s, not %s.", key, list(spec.enum), show(val))
			return
		}
		set = func(opts *Options) { spec.text(opts).put(val, o) }
		text += " " + val
	case oURL:
		s.secrets.addURL(val)
		if why := checkURL(val); why != "" {
			s.errorf(ln.num, "the address given to option %s cannot be used: %s.", key, why)
			return
		}
		set = func(opts *Options) { spec.text(opts).put(val, o) }
		text += " " + displayURL(val)
	case oText:
		if spec.secret != nil {
			s.secrets.add(val)
			set = func(opts *Options) { spec.secret(opts).put(NewSecret(val), o) }
			break
		}
		if spec.check != nil {
			if why := spec.check(val); why != "" {
				s.errorf(ln.num, "the value given to option %s cannot be used: %s.", key, why)
				return
			}
		}
		set = func(opts *Options) { spec.text(opts).put(val, o) }
		text += " " + val
	case oPath:
		if val == "" {
			s.errorf(ln.num, "option %s needs a path, and the value is empty.", key)
			return
		}
		val = expandHome(val, s.home)
		set = func(opts *Options) { spec.text(opts).put(val, o) }
		text += " " + val
	case oPathList:
		if val == "" {
			s.errorf(ln.num, "option %s needs a path, and the value is empty.", key)
			return
		}
		base := s.baseDir
		if spec.fromCwd {
			base = s.cwd
		}
		val = absPath(val, s.home, base)
		set = func(opts *Options) { addItem(spec.list(opts), val, o) }
		text += " " + val
	case oNameList:
		if val == "" {
			s.errorf(ln.num, "option %s needs a name, and the value is empty.", key)
			return
		}
		set = func(opts *Options) { addItem(spec.list(opts), val, o) }
		text += " " + val
	}
	s.add(op{kind: spec.trustKind(), text: text, origin: o, apply: func(b *builder) { set(&b.opts) }})
}

// addItem adds a value to a list option unless the list already has it.
func addItem(items *[]ListItem, v string, o Origin) {
	for _, it := range *items {
		if it.V == v {
			return
		}
	}
	*items = append(*items, ListItem{V: v, Origin: o})
}

// reset puts an option back as it is when nothing sets it.
func (o *optionSpec) reset(opts *Options) {
	def := defaultOptions()
	switch {
	case o.truth != nil:
		*o.truth(opts) = *o.truth(&def)
	case o.num != nil:
		*o.num(opts) = *o.num(&def)
	case o.text != nil:
		*o.text(opts) = *o.text(&def)
	case o.secret != nil:
		*o.secret(opts) = *o.secret(&def)
	case o.list != nil:
		*o.list(opts) = nil
	}
}
