package app

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

const shipSkill = "---\ndescription: Ships a release. Use when a release or a tag is asked for.\nargument-hint: \"[tag]\"\narguments: [tag]\n---\n\n# Ship\n\nTag the release $tag and say which commit it is.\n"

// lastUser is the last thing the user's side said in a request.
func lastUser(t *testing.T, req apptest.Request) string {
	t.Helper()
	users := userTexts(req)
	if len(users) == 0 {
		t.Fatal("the request holds nothing from the user's side")
	}
	return users[len(users)-1]
}

// A skill the person keeps is run by typing its name: its text, with the
// arguments put in, goes to the model as the prompt. Typed again with the
// same arguments it is not sent a second time. /skills and /status say
// where it came from.
func TestSkillTypedByThePerson(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "Tagged v1.2 at abc123."}, apptest.Reply{Text: "It is still abc123."},
		apptest.Reply{Text: "Tagged v1.3."}, apptest.Reply{Text: "A summary of the conversation."}, apptest.Reply{Text: "Going on."})
	path := w.Write(w.ConfigDir()+"/skills/ship/SKILL.md", shipSkill)
	w.Write(w.Home+"/.claude/skills/ship/SKILL.md", "Claude Code's own ship, which gnosh's folder comes before.\n")
	w.Write(w.Home+"/.claude/commands/git/sync.md", "Fetch and rebase.\n")
	s := sit(t, w)
	s.see("Type a prompt and Enter.")
	s.typeLine("/ship v1.2")
	s.see("The skill ship is in front of the model")
	s.see("Tagged v1.2 at abc123.")
	first := w.Model.Requests()[0]
	has(t, "what the model read", lastUser(t, first), `The user ran the skill "ship" by typing: /ship v1.2`, "# Ship", "Tag the release v1.2 and say which commit it is.")
	hasNot(t, "what the model read", string(first.Body), "Claude Code's own ship")
	// The skill is the person's alone here: with no Skill tool, the model
	// is not told of a list. (It is, when a skill may be loaded by it:
	// see the next test.)
	if names := toolNamesOf(first); !slices.Contains(names, "Skill") {
		t.Errorf("the session's tools are %v, want Skill among them", names)
	}

	s.typeLine("/ship v1.2")
	s.see("It is still abc123.")
	second := lastUser(t, w.Model.Requests()[1])
	has(t, "the second time", second, `The user ran the skill "ship" again`, "Its text is the same")
	hasNot(t, "the second time", second, "# Ship")
	s.typeLine("/ship v1.3")
	s.see("Tagged v1.3.")
	has(t, "with another argument", lastUser(t, w.Model.Requests()[2]), "Tag the release v1.3")

	s.typeLine("/skills")
	s.see("/git:sync  Fetch and rebase.")
	s.see("/ship [tag]  Ships a release.")
	s.see(path + " (you type /ship, and the model may load it)")
	s.see("not a skill: " + w.Home + "/.claude/skills/ship/SKILL.md (another skill named \"ship\" comes before it, and that one is used)")
	s.typeLine("/status")
	s.see("skills       git:sync (" + w.Home + "/.claude/commands/git/sync.md), ship (" + path + ")")

	// After a summary the model is given the skill's text back.
	s.typeLine("/compact")
	s.see("The conversation was summarized")
	s.typeLine("go on")
	s.see("Going on.")
	reqs := w.Model.Requests()
	after := strings.Join(userTexts(reqs[len(reqs)-1]), "\n")
	has(t, "what the model was given back after the summary", after, "The skill ship was loaded before the summary", "Tag the release v1.3", "The user keeps these skills")
	s.typeLine("/exit")
	if code := s.exit(); code != 0 {
		t.Errorf("exit code %d", code)
	}
	checkSession(t, w, w.Sessions()[0])
}

// As the prompt of a run, a skill's name runs the skill, and the run is
// the model's answer to it. A name that is no skill and no command is a
// prompt like any other.
func TestSkillInARun(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "Tagged v2."})
	w.Write(w.ConfigDir()+"/skills/ship/SKILL.md", shipSkill)
	r := run(t, w, "-p", "/ship v2").wantCode(t, 0)
	if r.out != "Tagged v2.\n" {
		t.Errorf("stdout = %q", r.out)
	}
	has(t, "what the model read", lastUser(t, w.Model.Requests()[0]), "Tag the release v2 and say")

	// Skills switched off: the line is not a skill any more.
	w = apptest.NewWorld(t, apptest.Reply{Text: "I do not know that command."})
	w.Write(w.ConfigDir()+"/skills/ship/SKILL.md", shipSkill)
	run(t, w, "-p", "--disable-slash-commands", "/ship v2").wantCode(t, 0)
	hasNot(t, "what the model read with skills off", string(w.Model.Requests()[0].Body), "Tag the release")
}

// A model is told which skills there are and loads one with the Skill
// tool, under the person's rules. A skill that is the person's alone is
// neither listed nor loaded for it.
func TestSkillLoadedByTheModel(t *testing.T) {
	prep := func(replies ...apptest.Reply) *apptest.World {
		w := apptest.NewWorld(t, replies...)
		w.Write(w.ConfigDir()+"/skills/ship/SKILL.md", shipSkill)
		w.Write(w.ConfigDir()+"/skills/wipe/SKILL.md", "---\ndescription: Wipes the staging database.\ndisable-model-invocation: true\n---\nDrop every table.\n")
		return w
	}
	load := func(name, args string) apptest.Reply {
		return apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Skill", Args: `{"skill":"` + name + `","args":"` + args + `"}`}}}
	}
	w := prep(load("ship", "v3"), apptest.Reply{Text: "Tagged v3."})
	run(t, w, "-p", "--allowedTools", "Skill(ship *)", "--", "cut a release").wantCode(t, 0)
	first := w.Model.Requests()[0]
	listing := strings.Join(userTexts(first), "\n")
	has(t, "the first prompt", listing, "The user keeps these skills", "- `ship`: Ships a release. Use when a release or a tag is asked for.")
	hasNot(t, "the first prompt", listing, "wipe")
	got := toolResult(t, w, 1, "c1")
	has(t, "what the model read", got, "The skill ship is loaded.", "Tag the release v3 and say which commit it is.")

	// Without leave, a run refuses the call as it refuses any that would
	// ask.
	w = prep(load("ship", "v3"), apptest.Reply{Text: "I could not load it."})
	run(t, w, "-p", "cut a release").wantCode(t, 0)
	hasNot(t, "what the model read without leave", toolResult(t, w, 1, "c1"), "Tag the release")

	// The person's alone.
	w = prep(load("wipe", ""), apptest.Reply{Text: "That one is yours to run."})
	run(t, w, "-y", "-p", "clean up").wantCode(t, 0)
	got = toolResult(t, w, 1, "c1")
	has(t, "what the model read", got, "the skill wipe is one only the user starts", "Do not carry out its steps another way")
	hasNot(t, "what the model read", got, "Drop every table")

	// A name that is no skill.
	w = prep(load("deploy", ""), apptest.Reply{Text: "There is no such skill."})
	run(t, w, "-y", "-p", "deploy it").wantCode(t, 0)
	has(t, "what the model read", toolResult(t, w, 1, "c1"), `there is no skill named "deploy". The skills are: ship`)
}

// A skill that asks for something this gnosh does not have is not run,
// by the person or by a model, and says why. Nor is one that is for the
// model alone run by typing it.
func TestSkillThatCannotBeRun(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "Nothing to do."})
	w.Write(w.ConfigDir()+"/skills/audit/SKILL.md", "---\ndescription: Audits the repo.\ncontext: fork\nmodel: big\n---\nRead everything, after !`git status`.\n")
	w.Write(w.ConfigDir()+"/skills/quiet/SKILL.md", "---\ndescription: For the model.\nuser-invocable: false\n---\nBe quiet.\n")
	s := sit(t, w)
	s.see("Type a prompt and Enter.")
	s.typeLine("/audit")
	s.see("error: the skill was not run: the skill audit (" + w.ConfigDir() + "/skills/audit/SKILL.md) asks for a model of its own (model), running in a sub-agent (context: fork), commands run before the model sees it")
	s.see("It is not run without: that would be another skill than the one its author wrote")
	s.typeLine("/quiet")
	s.see("error: the skill quiet is for the model alone")
	s.typeLine("/nosuchskill")
	s.see("There is no command /nosuchskill.")
	s.typeLine("/skills")
	s.see("cannot be run by this gnosh: it asks for a model of its own (model)")
	s.typeLine("/exit")
	if code := s.exit(); code != 0 {
		t.Errorf("exit code %d", code)
	}
	if n := len(w.Model.Requests()); n != 0 {
		t.Errorf("the model was sent %d requests", n)
	}
}

// The permission rules a skill carries hold from when it is run until the
// person's next prompt, and no longer.
func TestSkillAllowedToolsHoldUntilTheNextPrompt(t *testing.T) {
	touch := apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"touch made-by-the-skill.txt"}`}}}
	w := apptest.NewWorld(t, touch, apptest.Reply{Text: "Made."}, touch, apptest.Reply{Text: "Not made again."})
	w.Write(w.ConfigDir()+"/skills/make/SKILL.md", "---\ndescription: Makes the file.\nallowed-tools: Bash(touch made-by-the-skill.txt)\n---\nRun: touch made-by-the-skill.txt\n")
	asked := 0
	_, r := attach(t, w, nil, func(ctx context.Context, c SessionControls, f *scripted) int {
		questions := func() int {
			f.mu.Lock()
			defer f.mu.Unlock()
			n := 0
			for _, ev := range f.events {
				if _, ok := ev.(*agent.QuestionEvent); ok {
					n++
				}
			}
			return n
		}
		res, err := c.Command(ctx, "/make")
		if err != nil || !res.Handled || res.Prompt == "" {
			t.Errorf("/make: %+v, %v", res, err)
			return 1
		}
		_ = c.Wait(ctx)
		if n := questions(); n != 0 || !w.Exists("made-by-the-skill.txt") {
			t.Errorf("under the skill's rule: %d question(s), file made: %v", n, w.Exists("made-by-the-skill.txt"))
		}
		// The person's next prompt: the rule is gone, and the same
		// command asks.
		if _, err := c.Submit(agent.Prompt{Text: "do it again"}); err != nil {
			return 1
		}
		q := find(f, "the question", func(e *agent.QuestionEvent) bool { return true })
		asked = questions()
		_ = c.Answer(q.Question.ID, agent.Answer{Choice: "no", Text: "not again"})
		_ = c.Wait(ctx)
		c.Quit("prompt_input_exit")
		return 0
	})
	r.wantCode(t, 0)
	if asked != 1 {
		t.Errorf("after the person's next prompt the command asked %d time(s), want once", asked)
	}
}

// Definitions come from gnosh's own folders first and Claude Code's after
// them, and a project's are read only when its folder is trusted. gnosh
// doctor says where each came from.
func TestDefinitionFolders(t *testing.T) {
	agentFile := func(name, about string) string {
		return "---\nname: " + name + "\ndescription: " + about + "\n---\nDo as asked.\n"
	}
	w := apptest.NewWorld(t)
	own := w.Write(w.ConfigDir()+"/agents/reviewer.md", agentFile("reviewer", "gnosh's own reviewer."))
	w.Write(w.Home+"/.claude/agents/reviewer.md", agentFile("reviewer", "Claude Code's reviewer."))
	theirs := w.Write(w.Home+"/.claude/agents/tester.md", agentFile("tester", "Runs the tests."))
	project := w.Write(".gnosh/agents/fixer.md", agentFile("fixer", "The project's fixer."))
	w.Write(".claude/agents/fixer.md", agentFile("fixer", "The project's other fixer."))
	skillOwn := w.Write(w.ConfigDir()+"/skills/ship/SKILL.md", shipSkill)
	skillProject := w.Write(".gnosh/skills/deploy/SKILL.md", "Deploy the project.\n")
	w.Write(".claude/commands/lint.md", "Lint it.\n")

	r := run(t, w, "doctor")
	has(t, "doctor in a folder that is not trusted", r.out,
		"the sub-agent kind general-purpose is gnosh's own",
		"the sub-agent kind reviewer is defined in "+own,
		"the sub-agent kind tester is defined in "+theirs,
		"the skill ship is "+skillOwn,
		"this folder defines 1 kind(s) of sub-agent and 2 skill(s) of its own, which are not read because the folder is not trusted")
	hasNot(t, "doctor in a folder that is not trusted", r.out, "fixer", "deploy", "lint", "Claude Code's reviewer")

	// Trusted for the one run, a session reads the project's own.
	w.Model.Add(apptest.Reply{Text: "Hello."})
	run(t, w, "--trust-folder", "-p", "hello").wantCode(t, 0)
	said := strings.Join(userTexts(w.Model.Requests()[0]), "\n")
	has(t, "the first prompt in a folder trusted for the run", said, "`fixer`: The project's fixer.", "- `deploy`:", "- `lint`:", "`reviewer`: gnosh's own reviewer.")

	// Recorded as trusted, the doctor reads them too.
	run(t, w, "trust").wantCode(t, 0)
	r = run(t, w, "doctor")
	has(t, "doctor in a trusted folder", r.out,
		"the sub-agent kind fixer is defined in "+project,
		"the skill deploy is "+skillProject,
		"the skill lint is "+w.Path(".claude/commands/lint.md"),
		"the sub-agent kind reviewer is defined in "+own)
	hasNot(t, "doctor in a trusted folder", r.out, "which are not read")
	if t.Failed() {
		t.Logf("doctor in a trusted folder said (exit code %d):\n%s\nand on standard error:\n%s", r.code, r.out, r.err)
	}

}

// A session in a folder that is not trusted says what it leaves out, and
// the model is told only of the person's own.
func TestDefinitionsOfAFolderThatIsNotTrusted(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "Hello."})
	w.Write(w.ConfigDir()+"/agents/reviewer.md", "---\nname: reviewer\ndescription: The person's reviewer.\n---\nReview.\n")
	w.Write(w.ConfigDir()+"/skills/ship/SKILL.md", shipSkill)
	w.Write(".gnosh/agents/fixer.md", "---\nname: fixer\ndescription: The project's fixer.\n---\nFix.\n")
	w.Write(".claude/skills/deploy/SKILL.md", "Deploy the project.\n")
	r := run(t, w, "-p", "hello").wantCode(t, 0)
	has(t, "stderr", r.err, "This folder defines 1 kind(s) of sub-agent and 1 skill(s) of its own")
	said := strings.Join(userTexts(w.Model.Requests()[0]), "\n")
	has(t, "the first prompt", said, "`reviewer`: The person's reviewer.", "- `ship`:")
	hasNot(t, "the first prompt", said, "fixer", "deploy")
}

// A skill's disallowed-tools refuse the calls they name, even with
// permissions skipped, and its effort is the thinking level, both until
// the person's next prompt.
func TestSkillDisallowedToolsAndEffort(t *testing.T) {
	touch := apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"touch not-made.txt"}`}}}
	w := apptest.NewWorld(t, touch, apptest.Reply{Text: "It was refused."})
	w.Settings(`{"models": {"local/test-model": {"thinkingLevels": ["low", "high"]}}}`)
	w.Write(w.ConfigDir()+"/skills/careful/SKILL.md", "---\ndescription: Reads only.\ndisallowed-tools: Bash\neffort: max\n---\nLook, and change nothing.\n")
	r := run(t, w, "-y", "-p", "/careful").wantCode(t, 0)
	if w.Exists("not-made.txt") {
		t.Errorf("a tool the skill takes away ran")
	}
	has(t, "the run's answer", r.out, "It was refused.")
	first, second := requestText(t, w, 0), requestText(t, w, 1)
	has(t, "what the model read of the refusal", second, "This call did not run. The deny rule Bash switches Bash off.")
	has(t, "the first request's thinking", first, `"reasoning_effort":"high"`)
	w.Model.Done()
}
