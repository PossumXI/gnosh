package config

import (
	"slices"
)

// builder holds the configuration while the ops of each file are put in
// force, weakest file first. Its add and remove methods are what the
// directives do; a mistake that only shows against what earlier lines set
// (a model on a provider that does not exist) is reported from here, at
// the line that made it.
type builder struct {
	in    *Inputs
	probs *problems

	providers []*Provider
	models    []*Model
	large     Slot
	small     Slot
	// stateThinking is the thinking level the state file holds for each
	// model, keyed by provider/id.
	stateThinking map[string]Value[string]
	// largeBefore and smallBefore are the model choices as they stood
	// before the state file's were put on top, to fall back on when the
	// state file's choice turns out to be stale.
	largeBefore, smallBefore Value[ModelRef]

	hosts    []Host
	hooks    []Hook
	hooksOff Value[bool]

	layers []*RuleLayer
	mode   Value[string]
	// modeFromUser reports that the mode in force came from the user's own
	// settings file, which is what makes bypassPermissions available.
	modeFromUser   bool
	bypassDisabled Value[bool]
	denyInstead    Value[bool]
	dirs           []ListItem

	env  map[string]EnvVar
	mcp  []MCPServer
	lsp  []LSPServer
	opts Options
}

// newBuilder returns a builder holding the defaults.
func newBuilder(in *Inputs, probs *problems) *builder {
	return &builder{
		in:            in,
		probs:         probs,
		stateThinking: map[string]Value[string]{},
		env:           map[string]EnvVar{},
		opts:          defaultOptions(),
	}
}

// run puts one op in force. An op for a part of gnosh the binary does not
// have is a mistake: the setting would look configured and do nothing.
func (b *builder) run(o op) {
	if o.feature != "" && !b.in.Built.has(o.feature) {
		words := featureWords[o.feature]
		b.probs.at(Error, o.origin, "%s is configured here, and this gnosh has no %s yet, so the setting would look in force and do nothing. gnosh does not start with a setting like that: take it out, or use a gnosh that has %s.", o.what, words, words)
	}
	o.apply(b)
}

// provider finds a provider by ID.
func (b *builder) provider(id string) *Provider {
	for _, p := range b.providers {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// providerIDs lists the providers so far, for a message.
func (b *builder) providerIDs() string {
	if len(b.providers) == 0 {
		return "none"
	}
	ids := make([]string, len(b.providers))
	for i, p := range b.providers {
		ids[i] = p.ID
	}
	return list(ids)
}

// addProvider adds a provider or updates the one with the same ID. What
// the line gave replaces what was there; what it left out stays.
func (b *builder) addProvider(id string, ch providerChange, o Origin) {
	p := b.provider(id)
	if p == nil {
		if ch.typ == nil {
			b.probs.at(Error, o, "provider add %s needs --type the first time a provider is named, so gnosh knows how to talk to it. The types are: %s.", id, list(providerTypeNames()))
			return
		}
		p = &Provider{ID: id, Name: id}
		b.providers = append(b.providers, p)
	}
	if ch.typ != nil {
		p.Type, p.Wire = ch.typ.name, ch.typ.wire
	}
	if ch.name != nil {
		if p.Name = *ch.name; p.Name == "" {
			p.Name = id
		}
	}
	if ch.hasURLs {
		p.Endpoints = nil
		for _, u := range ch.urls {
			p.Endpoints = append(p.Endpoints, Endpoint{Given: u})
		}
	}
	if ch.key != nil {
		p.APIKey, p.KeyFrom = *ch.key, ch.keyFrom
	}
	if ch.discover != nil {
		p.DiscoverModels.put(*ch.discover, o)
	}
	if ch.hasExtraBody {
		p.ExtraBody = ch.extraBody
	}
	if ch.hasHeaders {
		p.Headers = ch.headers
	}
	if ch.disable != nil {
		p.Disabled = *ch.disable
	}
	if ch.prefix != nil {
		p.SystemPromptPrefix = *ch.prefix
	}
	if ch.options != nil {
		p.ProviderOptions = ch.options
	}
	if ch.flatRate != nil {
		p.FlatRate = *ch.flatRate
	}
	p.Origin = o
}

// removeProvider removes a provider, the models registered on it, and any
// model choice that named it.
func (b *builder) removeProvider(id string, o Origin) {
	if b.provider(id) == nil {
		b.probs.at(Error, o, "provider remove %s: there is no provider of that name to remove. The providers so far are: %s.", id, b.providerIDs())
		return
	}
	b.providers = slices.DeleteFunc(b.providers, func(p *Provider) bool { return p.ID == id })
	b.models = slices.DeleteFunc(b.models, func(m *Model) bool { return m.Provider == id })
	for _, sl := range []*Slot{&b.large, &b.small} {
		if sl.Model.Set && sl.Model.V.Provider == id {
			sl.Model = Value[ModelRef]{}
		}
	}
}

// model finds a registered model.
func (b *builder) model(ref ModelRef) *Model {
	for _, m := range b.models {
		if m.Provider == ref.Provider && m.ID == ref.ID {
			return m
		}
	}
	return nil
}

// addModel registers a model on an existing provider, or updates it.
func (b *builder) addModel(ref ModelRef, ch modelChange, o Origin) {
	if b.provider(ref.Provider) == nil {
		b.probs.at(Error, o, "model add %s: there is no provider named %s. Add it first, with provider add, on a line above this one. The providers so far are: %s.", ref, ref.Provider, b.providerIDs())
		return
	}
	m := b.model(ref)
	if m == nil {
		m = &Model{Provider: ref.Provider, ID: ref.ID, Name: ref.ID}
		b.models = append(b.models, m)
	}
	if ch.name != nil {
		if m.Name = *ch.name; m.Name == "" {
			m.Name = ref.ID
		}
	}
	if ch.window != nil {
		m.ContextWindow.put(*ch.window, o)
	}
	if ch.maxTokens != nil {
		m.DefaultMaxTokens.put(*ch.maxTokens, o)
	}
	if ch.hasLevels {
		m.ReasoningLevels = ch.levels
	}
	if ch.canReason != nil {
		m.CanReason.put(*ch.canReason, o)
	}
	if ch.images != nil {
		m.SupportsImages.put(*ch.images, o)
	}
	if ch.effort != nil {
		m.ReasoningEffort.put(*ch.effort, o)
	}
	for i, v := range []*Value[float64]{&m.PriceInput, &m.PriceOutput, &m.PriceCacheCreate, &m.PriceCacheHit} {
		if ch.prices[i] != nil {
			v.put(*ch.prices[i], o)
		}
	}
	m.Origin = o
}

// removeModel takes a registered model away.
func (b *builder) removeModel(ref ModelRef, o Origin) {
	if b.model(ref) == nil {
		b.probs.at(Error, o, "model remove %s: no model of that name was added with model add, so there is nothing to remove.", ref)
		return
	}
	b.models = slices.DeleteFunc(b.models, func(m *Model) bool { return m.Provider == ref.Provider && m.ID == ref.ID })
}

// setSlot applies a model large or model small line.
func (b *builder) setSlot(slot string, ch slotChange, o Origin) {
	sl := &b.large
	if slot == "small" {
		sl = &b.small
	}
	if ch.ref != nil {
		if b.provider(ch.ref.Provider) == nil {
			b.probs.at(Error, o, "model %s %s: there is no provider named %s. Add it first, with provider add, on a line above this one. The providers so far are: %s.", slot, ch.ref, ch.ref.Provider, b.providerIDs())
			return
		}
		sl.Model.put(*ch.ref, o)
	}
	if ch.thinking != nil {
		sl.Thinking.put(*ch.thinking, o)
	}
	if ch.temperature != nil {
		sl.Temperature.put(*ch.temperature, o)
	}
	if ch.maxTokens != nil {
		sl.MaxTokens.put(*ch.maxTokens, o)
	}
	if ch.topP != nil {
		sl.TopP.put(*ch.topP, o)
	}
	if ch.topK != nil {
		sl.TopK.put(*ch.topK, o)
	}
	if ch.frequencyPenalty != nil {
		sl.FrequencyPenalty.put(*ch.frequencyPenalty, o)
	}
	if ch.presencePenalty != nil {
		sl.PresencePenalty.put(*ch.presencePenalty, o)
	}
	if ch.options != nil {
		sl.ProviderOptions = ch.options
	}
}

// addHost declares a machine, or replaces the one of the same name.
func (b *builder) addHost(h Host) {
	for i := range b.hosts {
		if b.hosts[i].Name == h.Name {
			b.hosts[i] = h
			return
		}
	}
	b.hosts = append(b.hosts, h)
}

// removeHost takes a declared machine away.
func (b *builder) removeHost(name string, o Origin) {
	if !slices.ContainsFunc(b.hosts, func(h Host) bool { return h.Name == name }) {
		b.probs.at(Error, o, "host remove %s: there is no host of that name to remove.", name)
		return
	}
	b.hosts = slices.DeleteFunc(b.hosts, func(h Host) bool { return h.Name == name })
}

// addHook adds a hook. A named hook replaces the hook of the same event
// and name.
func (b *builder) addHook(h Hook) {
	if b.in.Built.Hooks && b.in.Built.HookEvents != nil && !slices.Contains(b.in.Built.HookEvents, h.Event) {
		b.probs.at(Error, h.Origin, "a hook is configured on %s, and this gnosh does not fire that event yet, so the hook would never run. gnosh does not start with a hook like that: take it out, or use a gnosh that fires %s. This one fires: %s.", h.Event, h.Event, list(b.in.Built.HookEvents))
	}
	if h.Name != "" {
		for i := range b.hooks {
			if b.hooks[i].Event == h.Event && b.hooks[i].Name == h.Name {
				b.hooks[i] = h
				return
			}
		}
	}
	b.hooks = append(b.hooks, h)
}

// removeHooks removes the hook of an event that has the given name, or,
// with no name, every hook of the event that earlier lines and files set.
func (b *builder) removeHooks(event, name string, o Origin) {
	match := func(h Hook) bool { return h.Event == event && (name == "" || h.Name == name) }
	if name != "" && !slices.ContainsFunc(b.hooks, match) {
		b.probs.at(Error, o, "hook remove %s --name %s: no hook of that name is configured on that event, so there is nothing to remove.", event, name)
		return
	}
	b.hooks = slices.DeleteFunc(b.hooks, match)
}

// addRule adds a permission rule to the layer of the file or flag it came
// from.
func (b *builder) addRule(which string, r Rule, scope, file, dir string) {
	var layer *RuleLayer
	for _, l := range b.layers {
		if l.Scope == scope && l.File == file {
			layer = l
		}
	}
	if layer == nil {
		layer = &RuleLayer{Scope: scope, File: file, BaseDir: dir}
		b.layers = append(b.layers, layer)
	}
	rules := &layer.Allow
	switch which {
	case "ask":
		rules = &layer.Ask
	case "deny":
		rules = &layer.Deny
	}
	if !slices.ContainsFunc(*rules, func(have Rule) bool { return have.Text == r.Text }) {
		*rules = append(*rules, r)
	}
}

// setEnv sets a variable of the env block. A later file wins.
func (b *builder) setEnv(v EnvVar) {
	if have, ok := b.env[v.Name]; ok && v.Origin.Layer < have.Origin.Layer {
		return
	}
	b.env[v.Name] = v
}

// addMCP adds an MCP server, or replaces the one of the same name.
func (b *builder) addMCP(m MCPServer) {
	for i := range b.mcp {
		if b.mcp[i].Name == m.Name {
			b.mcp[i] = m
			return
		}
	}
	b.mcp = append(b.mcp, m)
}

// removeMCP takes an MCP server away.
func (b *builder) removeMCP(name string, o Origin) {
	if !slices.ContainsFunc(b.mcp, func(m MCPServer) bool { return m.Name == name }) {
		b.probs.at(Error, o, "mcp remove %s: there is no MCP server of that name to remove.", name)
		return
	}
	b.mcp = slices.DeleteFunc(b.mcp, func(m MCPServer) bool { return m.Name == name })
}

// addLSP adds a language server, or replaces the one of the same name.
func (b *builder) addLSP(l LSPServer) {
	for i := range b.lsp {
		if b.lsp[i].Name == l.Name {
			b.lsp[i] = l
			return
		}
	}
	b.lsp = append(b.lsp, l)
}

// removeLSP takes a language server away.
func (b *builder) removeLSP(name string, o Origin) {
	if !slices.ContainsFunc(b.lsp, func(l LSPServer) bool { return l.Name == name }) {
		b.probs.at(Error, o, "lsp remove %s: there is no language server of that name to remove.", name)
		return
	}
	b.lsp = slices.DeleteFunc(b.lsp, func(l LSPServer) bool { return l.Name == name })
}

// sweep takes out of the builder everything a project's file put there
// that an untrusted folder may not contribute. Ops of a held file are never
// applied in the first place; this runs after every merge all the same, so
// that no later step can put back what is held. It reports what it found,
// which should be nothing.
func (b *builder) sweep(keep func(kind string) bool, projectFiles []string) (removed []string) {
	ok := func(kind string) bool { return keep != nil && keep(kind) }
	from := func(o Origin) bool { return o.File != "" && slices.Contains(projectFiles, o.File) }
	note := func(what string) { removed = append(removed, what) }

	if !ok(KindProvider) {
		b.providers = slices.DeleteFunc(b.providers, func(p *Provider) bool {
			if from(p.Origin) {
				note("provider " + p.ID)
				return true
			}
			return false
		})
	}
	if !ok(KindModel) {
		b.models = slices.DeleteFunc(b.models, func(m *Model) bool {
			if from(m.Origin) {
				note("model " + m.Ref())
				return true
			}
			return false
		})
		for name, sl := range map[string]*Slot{"large": &b.large, "small": &b.small} {
			if sl.Model.Set && from(sl.Model.Origin) {
				note("model " + name)
				sl.Model = Value[ModelRef]{}
			}
			if sl.Thinking.Set && from(sl.Thinking.Origin) {
				note("thinking level of model " + name)
				sl.Thinking = Value[string]{}
			}
		}
	}
	if !ok(KindHost) {
		b.hosts = slices.DeleteFunc(b.hosts, func(h Host) bool {
			if from(h.Origin) {
				note("host " + h.Name)
				return true
			}
			return false
		})
	}
	if !ok(KindHook) {
		b.hooks = slices.DeleteFunc(b.hooks, func(h Hook) bool {
			if from(h.Origin) {
				note("hook " + h.Event)
				return true
			}
			return false
		})
	}
	if !ok(KindHookOff) && b.hooksOff.Set && from(b.hooksOff.Origin) {
		note("disableAllHooks")
		b.hooksOff = Value[bool]{}
	}
	for _, l := range b.layers {
		if !slices.Contains(projectFiles, l.File) {
			continue
		}
		if !ok(KindAllowRule) && len(l.Allow) > 0 {
			note("allow rules")
			l.Allow = nil
		}
		if !ok(KindAskRule) && len(l.Ask) > 0 {
			note("ask rules")
			l.Ask = nil
		}
		if !ok(KindDenyRule) && len(l.Deny) > 0 {
			note("deny rules")
			l.Deny = nil
		}
	}
	if !ok(KindMode) {
		if b.mode.Set && from(b.mode.Origin) {
			note("defaultMode")
			b.mode = Value[string]{}
		}
		if b.bypassDisabled.Set && from(b.bypassDisabled.Origin) {
			note("disableBypassPermissionsMode")
			b.bypassDisabled = Value[bool]{}
		}
		if b.denyInstead.Set && from(b.denyInstead.Origin) {
			note("denyInsteadOfPrompting")
			b.denyInstead = Value[bool]{}
		}
	}
	if !ok(KindAddDir) {
		b.dirs = slices.DeleteFunc(b.dirs, func(d ListItem) bool {
			if from(d.Origin) {
				note("additional directory")
				return true
			}
			return false
		})
	}
	if !ok(KindEnv) {
		for name, v := range b.env {
			if from(v.Origin) {
				note("env " + name)
				delete(b.env, name)
			}
		}
	}
	b.mcp = slices.DeleteFunc(b.mcp, func(m MCPServer) bool {
		kind := KindMCPOnline
		if m.Type == "stdio" {
			kind = KindMCPLocal
		}
		if !ok(kind) && from(m.Origin) {
			note("mcp " + m.Name)
			return true
		}
		return false
	})
	if !ok(KindLSP) {
		b.lsp = slices.DeleteFunc(b.lsp, func(l LSPServer) bool {
			if from(l.Origin) {
				note("lsp " + l.Name)
				return true
			}
			return false
		})
	}
	def := defaultOptions()
	for i := range optionSpecs {
		spec := &optionSpecs[i]
		if ok(spec.trustKind()) {
			continue
		}
		var origin Origin
		switch {
		case spec.truth != nil:
			origin = spec.truth(&b.opts).Origin
		case spec.num != nil:
			origin = spec.num(&b.opts).Origin
		case spec.text != nil:
			origin = spec.text(&b.opts).Origin
		case spec.secret != nil:
			origin = spec.secret(&b.opts).Origin
		case spec.list != nil:
			items := spec.list(&b.opts)
			before := len(*items)
			*items = slices.DeleteFunc(*items, func(it ListItem) bool { return from(it.Origin) })
			if len(*items) != before {
				note("option " + spec.key)
			}
			continue
		}
		if from(origin) {
			note("option " + spec.key)
			switch {
			case spec.truth != nil:
				*spec.truth(&b.opts) = *spec.truth(&def)
			case spec.num != nil:
				*spec.num(&b.opts) = *spec.num(&def)
			case spec.text != nil:
				*spec.text(&b.opts) = *spec.text(&def)
			case spec.secret != nil:
				*spec.secret(&b.opts) = *spec.secret(&def)
			}
		}
	}
	slices.Sort(removed)
	return removed
}
