package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app/agentdef"
	"github.com/gnosh-ai/gnosh/internal/app/skill"
	"github.com/gnosh-ai/gnosh/internal/config"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/hooks"
	"github.com/gnosh-ai/gnosh/internal/instructions"
	"github.com/gnosh-ai/gnosh/internal/permission"
	"github.com/gnosh-ai/gnosh/internal/prompt"
	"github.com/gnosh-ai/gnosh/internal/tools/fstools"
)

// This file is skills in a session, and the folders that agent
// definitions and skills are read from.
//
// A skill is the person's own: instructions on disk for one kind of task
// (the package skill reads them; the format is Claude Code's). The person
// runs one by typing /name, and a model runs one with the Skill tool.
// Either way the skill's text, with its arguments put in, goes into the
// conversation once and stays there. The model is told which skills there
// are with the session's first prompt, and again after a summary, together
// with the text of each skill it had loaded.
//
// The folders (spec/DECISIONS.md, 15): gnosh's own come first, Claude
// Code's after them, so that what a person already has keeps working. For
// the person that is <config>/skills and <config>/commands, then
// ~/.claude/skills and ~/.claude/commands; for a project, .gnosh/ and then
// .claude/ in the working directory and every folder above it up to where
// the project begins. A project's are read only in a folder the person
// has trusted. Agent definitions follow the same rule (agents in place of
// skills).
//
// What is built of a skill's settings: its name, description, when_to_use,
// argument-hint, arguments, disable-model-invocation, user-invocable,
// allowed-tools and disallowed-tools (rules that let calls run, or refuse
// them, until the person's next prompt), effort (a thinking level until
// then), context: fork with agent, background and model (the skill runs
// in a sub-agent of that kind, on that model: forkSkill), and paths (the
// model is told of the skill once the session works on a matching file:
// skillsForFile). A skill that asks for anything else Claude Code's format
// has (a model of its own outside a fork, hooks, commands run before the
// model sees it) is refused, with the reason, when it is run: it is never
// run as something else. A sub-agent has no Skill tool.

// skillToolName is the name of the tool, which is Claude Code's.
const skillToolName = "Skill"

const skillDescription = `Loads a skill: instructions the user keeps for one kind of task. The skills there are were named in the first message of this session, each with what it is for. When the task in front of you matches one, call this tool with its name before you start on the task, then do what its text says: it is the user's own way of doing that kind of work, and it comes before your habits.

- skill: the name, as it was listed.
- args: optional. What the skill is to be told for this run, such as an issue number or a file, as one string.

The result is the skill's text, which stays in the conversation: load a skill once, not before every step. Do not call this tool with a name that was not listed, and do not guess what a skill would say.`

var skillSchema = json.RawMessage(`{"type":"object","properties":{"skill":{"type":"string","description":"The skill's name, as listed in the first message of this session."},"args":{"type":"string","description":"Optional: what the skill is to be told for this run, as one string."}},"required":["skill"],"additionalProperties":false}`)

// ownDir and theirDir are the folders of a project, or of a person's
// home, that definitions are read from: gnosh's own first.
const (
	ownDir   = ".gnosh"
	theirDir = ".claude"
)

// userDefDirs lists where a person's own definitions of one kind
// ("agents", "skills", "commands") are read from: gnosh's config folder
// first, then Claude Code's.
func userDefDirs(cfg *config.Config, kind string) []string {
	return []string{filepath.Join(cfg.Paths.ConfigDir, kind), filepath.Join(cfg.Paths.Home, theirDir, kind)}
}

// projectDefDirs lists where a project's definitions of one kind are read
// from: in the working directory and every folder above it up to where
// the project begins, the closest first, and then in more, the folders
// that were added to the session. In each folder gnosh's own comes first.
func projectDefDirs(cfg *config.Config, kind string, more []string) []string {
	var dirs []string
	add := func(dir string) {
		for _, d := range []string{filepath.Join(dir, ownDir, kind), filepath.Join(dir, theirDir, kind)} {
			if !slices.Contains(dirs, d) {
				dirs = append(dirs, d)
			}
		}
	}
	root := cfg.Paths.ProjectRoot
	for dir := cfg.Paths.Cwd; ; dir = filepath.Dir(dir) {
		add(dir)
		if dir == root || dir == filepath.Dir(dir) || !strings.HasPrefix(dir, root) {
			break
		}
	}
	for _, r := range more {
		add(r)
	}
	return dirs
}

// loadAgentDefs reads the kinds of sub-agent there are in a folder: the
// person's, the project's when the folder is trusted, and gnosh's own.
// Safe mode reads none but gnosh's own.
func loadAgentDefs(cfg *config.Config, trusted bool, more []string) *agentdef.Set {
	if cfg.Switches.SafeMode.V {
		return agentdef.LoadFrom(nil, nil)
	}
	var project []string
	if trusted {
		project = projectDefDirs(cfg, "agents", more)
	}
	return agentdef.LoadFrom(userDefDirs(cfg, "agents"), project)
}

// loadSkills reads the skills there are in a folder: the person's, and
// the project's when the folder is trusted. Safe mode reads none.
func loadSkills(cfg *config.Config, trusted bool, more []string) *skill.Set {
	if cfg.Switches.SafeMode.V {
		return skill.Load(skill.Dirs{}, skill.Dirs{})
	}
	user := skill.Dirs{Skills: userDefDirs(cfg, "skills"), Commands: userDefDirs(cfg, "commands")}
	var project skill.Dirs
	if trusted {
		project = skill.Dirs{Skills: projectDefDirs(cfg, "skills", more), Commands: projectDefDirs(cfg, "commands", more)}
	}
	return skill.Load(user, project)
}

// heldDefs counts what a folder that is not trusted defines for itself
// and so is not read: its kinds of sub-agent and its skills.
func heldDefs(cfg *config.Config, more []string) (agents, skills int) {
	if cfg.Switches.SafeMode.V {
		return 0, 0
	}
	for _, d := range agentdef.LoadFrom(nil, projectDefDirs(cfg, "agents", more)).All() {
		if d.Scope == "project" {
			agents++
		}
	}
	held := skill.Load(skill.Dirs{}, skill.Dirs{Skills: projectDefDirs(cfg, "skills", more), Commands: projectDefDirs(cfg, "commands", more)})
	return agents, len(held.All())
}

// defFindings is what gnosh doctor says about the definitions on disk:
// each kind of sub-agent and each skill with the file it came from, the
// files that hold nothing usable, and what a folder that is not trusted
// keeps to itself.
func defFindings(cfg *config.Config, trusted bool) []config.Finding {
	var out []config.Finding
	add := func(area string, level config.Level, format string, args ...any) {
		out = append(out, config.Finding{Area: area, Level: level, Text: fmt.Sprintf(format, args...)})
	}
	if cfg.Switches.SafeMode.V {
		return out
	}
	defs := loadAgentDefs(cfg, trusted, nil)
	for _, d := range defs.All() {
		if d.Scope == "built-in" {
			add("agents", config.Fine, "the sub-agent kind %s is gnosh's own", d.Name)
		} else {
			add("agents", config.Fine, "the sub-agent kind %s is defined in %s", d.Name, d.Source)
		}
	}
	for _, sk := range defs.Skipped() {
		add("agents", config.Note, "%s defines no kind of sub-agent: %s", sk.Path, sk.Reason)
	}
	skills := loadSkills(cfg, trusted, nil)
	for _, sk := range skills.All() {
		if len(sk.Unbuilt) > 0 {
			add("skills", config.Note, "the skill %s (%s) cannot be run by this gnosh: it asks for %s", sk.Name, sk.Source, strings.Join(sk.Unbuilt, ", "))
		} else {
			add("skills", config.Fine, "the skill %s is %s", sk.Name, sk.Source)
		}
	}
	for _, sk := range skills.Skipped() {
		add("skills", config.Note, "%s is no skill: %s", sk.Path, sk.Reason)
	}
	if !trusted {
		if agents, held := heldDefs(cfg, nil); agents+held > 0 {
			add("trust", config.Note, "this folder defines %d kind(s) of sub-agent and %d skill(s) of its own, which are not read because the folder is not trusted", agents, held)
		}
	}
	return out
}

// skillsOff says this session has no skills at all.
func (l *live) skillsOff() bool {
	return l.cfg.Switches.SafeMode.V || l.inv.DisableSlashCommands
}

// skills reads the skills there are for this session. The folders are
// read again when they were last read more than a moment ago, so a file
// that was just written is used without a restart.
func (l *live) skills() *skill.Set {
	if l.skillsOff() {
		return skill.Load(skill.Dirs{}, skill.Dirs{})
	}
	l.skillMu.Lock()
	defer l.skillMu.Unlock()
	if l.skillSet == nil || time.Since(l.skillRead) > skillsFresh {
		l.skillSet, l.skillRead = loadSkills(l.cfg, l.st.trusted, l.moreRoots()), time.Now()
	}
	return l.skillSet
}

// skillsFresh is how long a reading of the skill folders is used.
const skillsFresh = time.Second

// slashReloadSkills is /reload-skills: the skill and command folders are
// read again now, and the skills found are named. (gnosh reads them again
// on its own a moment after a change, so the command is for the person
// who wants to see what it found.)
func (l *live) slashReloadSkills(ctx context.Context, c *SlashContext) error {
	if l.skillsOff() {
		c.Say("Skills are off in this session (safe mode, or --disable-slash-commands), so there is nothing to read.")
		return nil
	}
	l.skillMu.Lock()
	l.skillSet, l.skillRead = loadSkills(l.cfg, l.st.trusted, l.moreRoots()), time.Now()
	all := l.skillSet.All()
	l.skillMu.Unlock()
	var names []string
	for _, sk := range all {
		names = append(names, sk.Name)
	}
	slices.Sort(names)
	switch {
	case len(names) == 0:
		c.Say("The skill and command folders were read again, and hold no skill.")
	case len(names) > 30:
		c.Say("The skill and command folders were read again: %d skills, among them %s. /skills lists them all.", len(names), strings.Join(names[:30], ", "))
	default:
		c.Say("The skill and command folders were read again: %d skills, %s. /skills says more of each.", len(names), strings.Join(names, ", "))
	}
	return nil
}

// typedSkills describes the skills the person can run by typing them, for
// a front end.
func (l *live) typedSkills() []CommandInfo {
	var out []CommandInfo
	for _, sk := range l.skills().All() {
		if !sk.NoUser {
			out = append(out, CommandInfo{Name: sk.Name, Args: sk.ArgumentHint, Summary: oneLine(sk.Description, 200), InRun: true})
		}
	}
	return out
}

// skillHidden says the person's settings keep a skill from the model (the
// hiddenSkills option).
func (l *live) skillHidden(name string) bool {
	for _, d := range l.cfg.Options.HiddenSkills {
		if d.V == name {
			return true
		}
	}
	return false
}

// forModel lists the skills a model may load: not the ones that are the
// person's alone, hidden from it, or beyond this build, nor one for some
// files (paths) before the session has worked on such a file.
func (l *live) forModel() []skill.Skill {
	var out []skill.Skill
	for _, sk := range l.skills().All() {
		if !sk.NoModel && len(sk.Unbuilt) == 0 && !l.skillHidden(sk.Name) && (len(sk.Paths) == 0 || l.skillWoken(sk.Name)) {
			out = append(out, sk)
		}
	}
	return out
}

// skillWoken reports whether a skill for some files (paths) has been told
// to the model, the session having worked on one of them.
func (l *live) skillWoken(name string) bool {
	l.skillMu.Lock()
	defer l.skillMu.Unlock()
	return l.skillsWoken[name]
}

// skillsForFile tells the model of the skills for some files (paths) that
// a call of a file tool in this session has just worked on one of: they
// apply from now on. The patterns are read against the project's root, as
// a rule's paths are.
func (l *live) skillsForFile(done agent.ToolDone) []agent.Part {
	if done.Result.IsError || done.AgentID != "" || !slices.Contains(l.toolNames(), skillToolName) {
		return nil
	}
	names := fstools.Defaults()
	switch done.Call.Name {
	case names.ReadName, names.WriteName, names.EditName:
	default:
		return nil
	}
	path := touchedPath(done)
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(l.cwd(), path)
	}
	rel, err := filepath.Rel(l.cfg.Paths.ProjectRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	rel = filepath.ToSlash(rel)
	var lines []string
	for _, sk := range l.skills().All() {
		if len(sk.Paths) == 0 || sk.NoModel || len(sk.Unbuilt) > 0 || l.skillHidden(sk.Name) || l.skillWoken(sk.Name) {
			continue
		}
		if pats, _ := instructions.CompilePaths(sk.Paths); !pats.Match(rel) {
			continue
		}
		l.skillMu.Lock()
		if l.skillsWoken == nil {
			l.skillsWoken = map[string]bool{}
		}
		l.skillsWoken[sk.Name] = true
		l.skillMu.Unlock()
		line := "- `" + sk.Name + "`"
		if d := oneLine(sk.Description, skill.MaxDescription); d != "" {
			line += ": " + d
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return nil
	}
	return []agent.Part{{Source: "skills", Text: prompt.Wrap("The user keeps these skills for files like " + rel + ", which you are working on now. When the task calls for one, load it with the Skill tool and follow what it says.\n" + strings.Join(lines, "\n"))}}
}

// skillsLine tells the model which skills there are. It goes out with a
// session's first prompt, and again after a summary. Every name is in it;
// the descriptions are left off from the end when together they would
// take more than a hundredth of the model's window.
func (l *live) skillsLine() string {
	if !slices.Contains(l.toolNames(), skillToolName) {
		return ""
	}
	list := l.forModel()
	if len(list) == 0 {
		return ""
	}
	budget := 8000
	if l.agent != nil {
		if w := l.agent.Context().Window; w > 0 {
			budget = max(w*4/100, 2000)
		}
	}
	lines := make([]string, len(list))
	used := 0
	for i, sk := range list {
		lines[i] = "- `" + sk.Name + "`"
		used += len(lines[i]) + 1
	}
	for i, sk := range list {
		d := oneLine(sk.Description, skill.MaxDescription)
		if d == "" || used+len(d)+2 > budget {
			continue
		}
		lines[i] += ": " + d
		used += len(d) + 2
	}
	return prompt.Wrap("The user keeps these skills: instructions for one kind of task each. When a task matches one, load it with the Skill tool before you start on the task, and follow what it says.\n" + strings.Join(lines, "\n"))
}

// skillRun is a skill as it went into the conversation.
type skillRun struct {
	name, text string
}

// unbuiltSkill says why a skill cannot be run by this build.
func unbuiltSkill(sk skill.Skill) string {
	return fmt.Sprintf("the skill %s (%s) asks for %s, which this gnosh does not have yet. It is not run without: that would be another skill than the one its author wrote", sk.Name, sk.Source, strings.Join(sk.Unbuilt, ", "))
}

// loadSkill makes a skill ready for the conversation: its text with the
// arguments put in, and the permission rules it carries put in force
// until the person's next prompt. again says the same text is already in
// the conversation. refusal says why it was not loaded.
func (l *live) loadSkill(sk skill.Skill, args string) (text string, again bool, refusal string) {
	if len(sk.Unbuilt) > 0 {
		return "", false, unbuiltSkill(sk)
	}
	effort := ""
	if l.agent != nil {
		effort = l.agent.State().Level
	}
	vars := skill.Vars{SessionID: l.ts.ID(), SkillDir: sk.Dir, ProjectDir: l.cfg.Paths.ProjectRoot, Effort: effort}
	text = sk.Render(args, vars)
	fill := func(list []string) []string {
		rules := make([]string, len(list))
		for i, r := range list {
			r = strings.ReplaceAll(r, "${CLAUDE_SKILL_DIR}", vars.SkillDir)
			rules[i] = strings.ReplaceAll(r, "${CLAUDE_PROJECT_DIR}", vars.ProjectDir)
		}
		return rules
	}
	if len(sk.AllowedTools)+len(sk.DisallowedTools) > 0 {
		// What it lets run, and what it takes away (disallowed-tools, as
		// rules that refuse: the model's list of tools stays as it is, so
		// that a local server keeps the prompt it has read).
		err := l.engine.AddLayer(permission.Layer{
			Scope: permission.ScopeSkill, Name: "the skill " + sk.Name + " (" + sk.Source + ")",
			FromProject: sk.Scope == "project", Allow: fill(sk.AllowedTools), Deny: fill(sk.DisallowedTools),
		})
		if err != nil {
			return "", false, fmt.Sprintf("the permission rules the skill %s carries (allowed-tools or disallowed-tools, in %s) cannot be used: %v. It is not run without them", sk.Name, sk.Source, err)
		}
	}
	if sk.Effort != "" {
		l.skillEffort(sk)
	}
	if refusal := l.skillHooks(sk); refusal != "" {
		return "", false, refusal
	}
	if sk.Fork {
		// (A forked skill's text is a sub-agent's task: it never enters
		// this conversation, and is not given back after a summary.)
		return text, false, ""
	}
	l.skillMu.Lock()
	defer l.skillMu.Unlock()
	for i, r := range l.skillRuns {
		if r.name == sk.Name {
			again = r.text == text
			l.skillRuns = append(l.skillRuns[:i], l.skillRuns[i+1:]...)
			break
		}
	}
	l.skillRuns = append(l.skillRuns, skillRun{sk.Name, text})
	return text, again, ""
}

// dropSkillGrants ends the permission rules that skills brought with
// them, and the thinking level one asked for: they hold until the
// person's next prompt.
func (l *live) dropSkillGrants() {
	l.engine.RemoveLayers(permission.ScopeSkill)
	l.skillMu.Lock()
	restore, before := l.effortHeld, l.effortBefore
	l.effortHeld = false
	l.skillMu.Unlock()
	if restore && l.agent != nil {
		_, _ = l.agent.SetThinking(before)
	}
}

// skillHooks puts a skill's own hooks (hooks) in force when the skill is
// first run, for the rest of the session, as Claude Code does; a hook with
// once: true goes after its first run that did not fail. They are checked
// as the settings' hooks are, and a skill whose hooks would not run is not
// run either. It says why not, or "".
func (l *live) skillHooks(sk skill.Skill) string {
	if len(sk.Hooks) == 0 || l.hooks == nil || l.hooks.r == nil || l.cfg.HooksOff != "" {
		return ""
	}
	l.skillMu.Lock()
	defer l.skillMu.Unlock()
	if l.skillHooksIn[sk.Name] {
		return ""
	}
	cfg, err := hooks.ParseConfig(sk.Hooks, sk.Source+" (hooks)")
	if err == nil {
		err = l.hooks.r.Add(cfg)
	}
	if err != nil {
		return fmt.Sprintf("the hooks the skill %s carries (in %s) cannot be put in force: %v. It is not run without them", sk.Name, sk.Source, err)
	}
	if l.skillHooksIn == nil {
		l.skillHooksIn = map[string]bool{}
	}
	l.skillHooksIn[sk.Name] = true
	return ""
}

// skillEffort puts the thinking level a skill asks for (effort) in force
// until the person's next prompt: one of the model's levels, max for its
// most, or auto for its own default. A level the model does not have
// leaves thinking as it is, and the person is told.
func (l *live) skillEffort(sk skill.Skill) {
	if l.agent == nil {
		return
	}
	m := l.agent.Model()
	word := sk.Effort
	switch {
	case word == "auto":
		word = ""
	case word == "max" && len(m.Levels) > 0:
		word = m.Levels[len(m.Levels)-1]
	case !slices.Contains(m.Levels, word):
		l.notice(agent.LevelInfo, "skill", fmt.Sprintf("The skill %s asks for the effort %s, which is not a thinking level of %s; thinking stays as it is.", sk.Name, sk.Effort, firstOf(m.Info.ID, "the model")))
		return
	}
	l.skillMu.Lock()
	if !l.effortHeld {
		l.effortHeld, l.effortBefore = true, m.Level
	}
	l.skillMu.Unlock()
	_, _ = l.agent.SetThinking(word)
}

// typedSkill runs a line that names a skill: /name and what follows. It
// reports false when the line names none. The skill goes to the model as
// a prompt of its own, which the person's line stands for on the screen.
func (l *live) typedSkill(word, args, line string) (CommandResult, bool, error) {
	if l.skillsOff() {
		return CommandResult{}, false, nil
	}
	sk, ok := l.skills().Get(word)
	if !ok {
		return CommandResult{}, false, nil
	}
	res := CommandResult{Handled: true, Name: "skill"}
	if sk.NoUser {
		return res, true, fmt.Errorf("the skill %s is for the model alone (its file says user-invocable: false), so typing it does nothing", sk.Name)
	}
	l.dropSkillGrants()
	l.nameAfter(line)
	text, again, refusal := l.loadSkill(sk, args)
	if refusal != "" {
		return res, true, fmt.Errorf("the skill was not run: %s", refusal)
	}
	// The UserPromptExpansion hooks see what the line expands into, and
	// may stop it or add to it.
	added := ""
	if l.hooks.has(hooks.UserPromptExpansion) {
		out, err := l.hooks.run(l.ctx, hooks.Input{Event: hooks.UserPromptExpansion, ExpansionType: "slash_command",
			CommandName: sk.Name, CommandArgs: args, CommandSource: sk.Scope, Prompt: text})
		switch {
		case err != nil:
			return res, true, fmt.Errorf("the skill was not run: the UserPromptExpansion hooks could not be run (%v)", err)
		case out.EndTurn:
			return res, true, fmt.Errorf("the skill was not run: a UserPromptExpansion hook the user set up stopped it. %s", firstOf(out.StopReason, out.Reason))
		case out.Blocked:
			return res, true, fmt.Errorf("the skill was not run: a UserPromptExpansion hook the user set up blocked it. %s", firstOf(out.Reason, "It gave no reason."))
		}
		if c := strings.TrimSpace(out.AdditionalContext); c != "" {
			added = "\n\n" + hookWords(hooks.UserPromptExpansion, "", c)
		}
	}
	if sk.Fork {
		return l.typedFork(sk, text, line, res)
	}
	lead := fmt.Sprintf("The user ran the skill %q by typing: %s\nWhat follows is the skill: the user's own instructions for this kind of task. Carry them out now.", sk.Name, oneLine(line, 400))
	body := prompt.Wrap(lead) + "\n\n" + text + added
	if again {
		body = prompt.Wrap(fmt.Sprintf("The user ran the skill %q again by typing: %s\nIts text is the same as when it was last loaded, earlier in this conversation: carry it out again from there.", sk.Name, oneLine(line, 400)))
	}
	id, err := l.agent.Submit(agent.Prompt{Hidden: true, Source: "skill", Text: body,
		Notice: fmt.Sprintf("The skill %s is in front of the model (%s).", sk.Name, sk.Source)})
	if err != nil {
		return res, true, fmt.Errorf("the skill could not be sent to the model: %v", err)
	}
	res.Prompt = id
	return res, true, nil
}

// skillTool is the Skill tool of one session.
type skillTool struct {
	l *live
}

type skillInput struct {
	Skill string `json:"skill"`
	Args  string `json:"args"`
}

// Spec is what a model is told about the tool.
func (t *skillTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: skillToolName, Description: skillDescription, InputSchema: skillSchema}
}

// find reads a call and finds the skill it names, or says in words for
// the model why the skill is not loaded.
func (t *skillTool) find(input json.RawMessage) (sk skill.Skill, in skillInput, refusal string) {
	if err := json.Unmarshal(input, &in); err != nil {
		return sk, in, fmt.Sprintf("the arguments cannot be read (%v)", err)
	}
	name := strings.TrimPrefix(strings.TrimSpace(in.Skill), "/")
	if name == "" {
		return sk, in, "the call names no skill"
	}
	sk, ok := t.l.skills().Get(name)
	if ok && sk.NoModel {
		return sk, in, fmt.Sprintf("the skill %s is one only the user starts (its file says disable-model-invocation). Do not carry out its steps another way: tell the user they can run it by typing /%s", sk.Name, sk.Name)
	}
	if !ok || t.l.skillHidden(name) {
		var names []string
		for _, s := range t.l.forModel() {
			names = append(names, s.Name)
		}
		return sk, in, fmt.Sprintf("there is no skill named %q. The skills are: %s", name, firstOf(strings.Join(names, ", "), "none"))
	}
	if len(sk.Unbuilt) > 0 {
		return sk, in, unbuiltSkill(sk)
	}
	return sk, in, ""
}

// Describe says what a call would do.
func (t *skillTool) Describe(call core.ToolCall, env *core.ToolEnv) (core.Action, error) {
	sk, in, refusal := t.find(call.Input)
	act := core.Action{Kind: core.ActionOther, Summary: "Load the skill " + oneLine(firstOf(sk.Name, in.Skill), 80)}
	if refusal != "" {
		return act, fmt.Errorf("%s", refusal)
	}
	act.Summary += " (" + sk.Source + ")"
	if len(sk.AllowedTools) > 0 {
		act.Summary += ", which lets these run without a question until your next prompt: " + oneLine(strings.Join(sk.AllowedTools, ", "), 300)
	}
	return act, nil
}

// Run loads the skill into the conversation.
func (t *skillTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	sk, in, refusal := t.find(call.Input)
	if refusal != "" {
		return core.ErrorResult("The skill was not loaded: " + refusal + "."), nil
	}
	text, again, refusal := t.l.loadSkill(sk, in.Args)
	if refusal != "" {
		return core.ErrorResult("The skill was not loaded: " + refusal + "."), nil
	}
	meta := map[string]any{"skill": sk.Name, "source": sk.Source}
	if sk.Fork {
		// The skill's text is a sub-agent's task, and its report is the
		// result.
		out, _ := t.l.forkSkill(ctx, call.ID, sk, text)
		out.Meta = meta
		return out, nil
	}
	if again {
		return core.ToolResult{Content: []core.Block{core.Text(fmt.Sprintf("The skill %s is already in this conversation with the same text, where it was last loaded. Follow it from there.", sk.Name))}, Meta: meta}, nil
	}
	lead := fmt.Sprintf("The skill %s is loaded. What follows is its text: the user's own instructions for this kind of task. Carry them out.", sk.Name)
	return core.ToolResult{Content: []core.Block{core.Text(prompt.Wrap(lead) + "\n\n" + text)}, Meta: meta}, nil
}

// How much of the skills that were loaded is given back after a summary:
// the start of each, the most recent first, up to a total. In characters,
// at four to a token: 5,000 and 25,000 tokens.
const (
	skillBackEach = 20000
	skillBackAll  = 100000
)

// skillsAfterSummary gives back the skills the conversation had loaded
// before a summary took its place.
func (l *live) skillsAfterSummary() string {
	l.skillMu.Lock()
	runs := slices.Clone(l.skillRuns)
	l.skillMu.Unlock()
	var parts []string
	left := skillBackAll
	for i := len(runs) - 1; i >= 0 && left > 0; i-- {
		text := runs[i].text
		if n := min(skillBackEach, left); len(text) > n {
			text = strings.ToValidUTF8(text[:n], "") + "\n[The rest of this skill was cut. Load it again with the Skill tool if you need all of it.]"
		}
		left -= len(text)
		parts = append(parts, prompt.Wrap(fmt.Sprintf("The skill %s was loaded before the summary. Its text, which still holds:", runs[i].name))+"\n\n"+text)
	}
	return strings.Join(parts, "\n\n")
}

// slashSkills is /skills: every skill there is, where it came from, who
// may run it, and what stops one that cannot be run.
func (l *live) slashSkills(ctx context.Context, c *SlashContext) error {
	if l.skillsOff() {
		c.Say("Skills are off in this session (safe mode, or --disable-slash-commands).")
		return nil
	}
	set := l.skills()
	if len(set.All()) == 0 {
		c.Say("There are no skills. A skill is a folder with a SKILL.md in it: in %s for all your projects, or in %s of a project you trust.",
			userDefDirs(l.cfg, "skills")[0], filepath.Join(ownDir, "skills"))
	}
	for _, sk := range set.All() {
		who := "you type /" + sk.Name + ", and the model may load it"
		switch {
		case len(sk.Unbuilt) > 0:
			who = "cannot be run by this gnosh: it asks for " + strings.Join(sk.Unbuilt, ", ")
		case sk.NoUser:
			who = "for the model alone"
		case sk.NoModel:
			who = "you type /" + sk.Name + "; the model may not load it"
		case l.skillHidden(sk.Name):
			who = "you type /" + sk.Name + "; your settings hide it from the model"
		}
		hint := ""
		if sk.ArgumentHint != "" {
			hint = " " + sk.ArgumentHint
		}
		c.Say("/%s%s  %s", sk.Name, hint, oneLine(sk.Description, 200))
		c.Say("    %s (%s)", sk.Source, who)
		if len(sk.AllowedTools) > 0 {
			c.Say("    lets these run without a question until your next prompt: %s", oneLine(strings.Join(sk.AllowedTools, ", "), 300))
		}
	}
	for _, sk := range set.Skipped() {
		c.Say("not a skill: %s (%s)", sk.Path, sk.Reason)
	}
	if !l.st.trusted {
		if _, held := heldDefs(l.cfg, l.moreRoots()); held > 0 {
			c.Say("This folder has %d skill(s) of its own, which are not read because the folder is not trusted (gnosh trust).", held)
		}
	}
	return nil
}

// forkSkill runs a skill with context: fork (spec/cc/slash-commands-and-
// skills.md 5.6): its text is the task of a sub-agent of the kind the
// skill names (general-purpose when it names none), which sees nothing of
// the conversation, and the sub-agent's report is the result. It waits
// for the report, as gnosh's Agent calls do, unless the skill asks to run
// in the background (background: true) and the session can: the report
// then comes later as a notice.
func (l *live) forkSkill(ctx context.Context, callID string, sk skill.Skill, text string) (core.ToolResult, bool) {
	in := agentInput{Description: "the skill " + sk.Name, Prompt: text, Type: sk.Agent, Model: sk.Model}
	if sk.Background {
		in.Background = &sk.Background
	}
	input, err := json.Marshal(in)
	if err != nil {
		return core.ErrorResult("The skill was not run: " + err.Error() + "."), false
	}
	call := core.ToolCall{ID: callID, Name: agentToolName, Input: input}
	out, err := (&agentTool{l: l}).Run(ctx, call, l.env())
	if err != nil {
		return core.ErrorResult("The skill was not run: " + err.Error() + "."), false
	}
	return out, !out.IsError
}

// typedFork runs a forked skill the person typed: the sub-agent does the
// skill's task, and its report goes to the model as a prompt of gnosh's
// own, which the model answers. A skill run in the background says so,
// and its report comes to the model when it ends.
func (l *live) typedFork(sk skill.Skill, text, line string, res CommandResult) (CommandResult, bool, error) {
	if l.interactive {
		l.notice(agent.LevelInfo, "skill", fmt.Sprintf("The skill %s runs in a sub-agent (%s).", sk.Name, sk.Source))
	}
	out, ok := l.forkSkill(l.ctx, newCallID(), sk, text)
	report := strings.TrimSpace(core.TextOf(out.Content))
	if !ok {
		return res, true, fmt.Errorf("the skill was not run: %s", firstOf(report, "the sub-agent did not start"))
	}
	if status, _ := out.Meta["status"].(string); status == "async_launched" {
		res.Text = fmt.Sprintf("The skill %s runs in a sub-agent in the background; its report comes to the model when it ends.", sk.Name)
		return res, true, nil
	}
	body := prompt.Wrap(fmt.Sprintf("The user ran the skill %q by typing: %s\nIt ran in a sub-agent, which saw nothing of this conversation, and this is its report. Tell the user what came of it, and carry on from there if the skill asks for more.", sk.Name, oneLine(line, 400))) + "\n\n" + prompt.Untrusted(report)
	id, err := l.agent.Submit(agent.Prompt{Hidden: true, Source: "skill", Text: body,
		Notice: fmt.Sprintf("The skill %s ran in a sub-agent; its report is in front of the model.", sk.Name)})
	if err != nil {
		return res, true, fmt.Errorf("the skill's report could not be sent to the model: %v", err)
	}
	res.Prompt = id
	return res, true, nil
}
