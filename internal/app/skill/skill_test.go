package skill

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	sk, reason := Parse("---\nname: ship\ndescription: Ships the build.\nwhen_to_use: When a release is asked for.\nargument-hint: \"[tag]\"\narguments: [tag, branch]\ndisable-model-invocation: yes\nallowed-tools: Bash(git add *) Bash(git commit *), Read\nlicense: MIT\nmade-up: ignored\n---\n\n# Ship\n\nTag $tag.\n", "folder")
	if reason != "" {
		t.Fatalf("skipped: %s", reason)
	}
	if sk.Name != "ship" || sk.Description != "Ships the build. When a release is asked for." || sk.ArgumentHint != "[tag]" || !sk.NoModel || sk.NoUser {
		t.Errorf("the skill: %+v", sk)
	}
	if !reflect.DeepEqual(sk.Arguments, []string{"tag", "branch"}) || !reflect.DeepEqual(sk.AllowedTools, []string{"Bash(git add *)", "Bash(git commit *)", "Read"}) {
		t.Errorf("arguments %q, allowed tools %q", sk.Arguments, sk.AllowedTools)
	}
	if sk.Body != "# Ship\n\nTag $tag." || len(sk.Unbuilt) != 0 {
		t.Errorf("body %q, unbuilt %q", sk.Body, sk.Unbuilt)
	}

	// No settings at all: the folder names it and the first line of the
	// text describes it.
	sk, reason = Parse("# Review a change\n\nRead the diff.\n", "review")
	if reason != "" || sk.Name != "review" || sk.Description != "Review a change" || sk.NoModel {
		t.Errorf("a file without settings: %+v, %q", sk, reason)
	}
	// Settings that cannot be read are no settings.
	sk, reason = Parse("---\n  : : bad\n---\nDo it.\n", "broken")
	if reason != "" || sk.Name != "broken" || sk.Body != "Do it." {
		t.Errorf("a file whose settings cannot be read: %+v, %q", sk, reason)
	}
	sk, _ = Parse("---\nuser-invocable: false\n---\nOnly for the model.\n", "quiet")
	if !sk.NoUser {
		t.Errorf("user-invocable: false was not read")
	}

	// What this gnosh does not have is named, each thing.
	sk, reason = Parse("---\ncontext: fork\nmodel: opus\neffort: high\nhooks:\n  Stop:\n    - x\npaths: [\"*.go\"]\ndisallowed-tools: Bash\nshell: powershell\n---\nRun !`git status` first.\n", "big")
	if reason != "" || len(sk.Unbuilt) != 1 || !sk.Injects || len(sk.Paths) != 1 || sk.Paths[0] != "*.go" || string(sk.Hooks) != `{"Stop":["x"]}` {
		t.Errorf("what is not built: %q (%s)", sk.Unbuilt, reason)
	}
	// context: fork is built, and a forked skill's model is its
	// sub-agent's.
	if !sk.Fork || sk.Model != "opus" {
		t.Errorf("fork %v, model %q", sk.Fork, sk.Model)
	}
	if sk, _ := Parse("---\nmodel: opus\n---\nx\n", "x"); len(sk.Unbuilt) != 1 || sk.Model != "" {
		t.Errorf("a model for a skill that is not forked: %q, %q", sk.Unbuilt, sk.Model)
	}
	if sk, _ := Parse("---\ncontext: fork\nagent: Explore\nbackground: true\n---\nx\n", "x"); !sk.Fork || sk.Agent != "Explore" || !sk.Background || len(sk.Unbuilt) != 0 {
		t.Errorf("a forked skill: %+v", sk)
	}
	if sk, _ := Parse("---\ncontext: thread\n---\nx\n", "x"); len(sk.Unbuilt) != 1 || sk.Fork {
		t.Errorf("a context gnosh does not know: %q", sk.Unbuilt)
	}
	// effort and disallowed-tools are built: read, and not listed.
	if sk.Effort != "high" || len(sk.DisallowedTools) != 1 || sk.DisallowedTools[0] != "Bash" {
		t.Errorf("effort %q, disallowed-tools %q", sk.Effort, sk.DisallowedTools)
	}
	for _, text := range []string{"Now:\n```!\ngit log\n```\n", "See !`date`.", "!`date`"} {
		if sk, _ := Parse(text, "x"); !sk.Injects || len(sk.Unbuilt) != 0 {
			t.Errorf("%q: not seen as running a command first", text)
		}
	}
	for _, text := range []string{"Wow!`not one`", "Use `!` to negate.", "```\n!ls\n```"} {
		if sk, _ := Parse(text, "x"); sk.Injects {
			t.Errorf("%q: taken for a command to run", text)
		}
	}
	// model: inherit asks for nothing.
	if sk, _ := Parse("---\nmodel: inherit\n---\nx\n", "x"); len(sk.Unbuilt) != 0 {
		t.Errorf("model: inherit is listed as not built")
	}

	for name, want := range map[string]string{
		"":                                      "no name",
		"has space":                             "cannot be typed or seen",
		"a<b":                                   "cannot be typed or seen",
		"zero" + string(rune(0x200B)) + "width": "cannot be typed or seen",
		"-dash":                                 "not one that can be typed",
		strings.Repeat("n", 65):                 "longer than 64",
	} {
		if _, reason := Parse("text\n", name); !strings.Contains(reason, want) {
			t.Errorf("the name %q: %q, want it to say %q", name, reason, want)
		}
	}
	if _, reason := Parse("---\nname: empty\n---\n\n", "x"); reason != "it holds no instructions" {
		t.Errorf("an empty skill: %q", reason)
	}
}

func TestRender(t *testing.T) {
	v := Vars{SessionID: "s-1", SkillDir: "/skills/ship", ProjectDir: "/proj", Effort: "high"}
	cases := []struct {
		name, body, args, want string
		arguments              []string
	}{
		{"all of them", "Fix $ARGUMENTS now.", "issue 12", "Fix issue 12 now.", nil},
		{"by number", "A=$0 B=$1 C=$ARGUMENTS[1]", `"hello world" two`, "A=hello world B=two C=two", nil},
		{"a number with no argument stays", "A=$0 B=$1", "one", "A=one B=$1", nil},
		{"by name", "Issue $issue on $branch.", "12", "Issue 12 on .", []string{"issue", "branch"}},
		{"the session's values", "${CLAUDE_SESSION_ID} ${CLAUDE_SKILL_DIR} ${CLAUDE_PROJECT_DIR} ${CLAUDE_EFFORT} ${OTHER}", "", "s-1 /skills/ship /proj high ${OTHER}", nil},
		{"no place for them", "Do the thing.\n", "with care", "Do the thing.\n\nARGUMENTS: with care", nil},
		{"none given", "Do the thing.", "", "Do the thing.", nil},
		{"escaped", `It costs \$1.00, not $0; \$ARGUMENTS and \$issue stay.`, "five", `It costs $1.00, not five; $ARGUMENTS and $issue stay.`, []string{"issue"}},
		{"never read again", "Say $0.", `'$1 ${CLAUDE_SKILL_DIR}' x`, "Say $1 ${CLAUDE_SKILL_DIR}.", nil},
		{"a dollar that is not a place", "Cost: $ and $x and $ARGUMENTSX", "a", "Cost: $ and $x and $ARGUMENTSX\n\nARGUMENTS: a", nil},
	}
	for _, c := range cases {
		sk := Skill{Body: c.body, Arguments: c.arguments}
		if got := sk.Render(c.args, v); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWordsAndRules(t *testing.T) {
	if got := Words(`one "two three" 'four  five' six\ seven ""`); !reflect.DeepEqual(got, []string{"one", "two three", "four  five", "six seven", ""}) {
		t.Errorf("words: %q", got)
	}
	if got := SplitRules("Bash(git add *) Bash(git commit *),Read,  Edit(src/**)"); !reflect.DeepEqual(got, []string{"Bash(git add *)", "Bash(git commit *)", "Read", "Edit(src/**)"}) {
		t.Errorf("rules: %q", got)
	}
}

func TestLoad(t *testing.T) {
	root := t.TempDir()
	write := func(path, text string) {
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("own/skills/ship/SKILL.md", "---\ndescription: gnosh's own ship\n---\nShip.\n")
	write("cc/skills/ship/SKILL.md", "---\ndescription: Claude Code's ship\n---\nShip.\n")
	write("cc/skills/review/SKILL.md", "Review the change.\n")
	write("cc/skills/not-a-skill.txt", "nothing\n")
	write("cc/skills/empty/SKILL.md", "---\nname: empty\n---\n")
	write("cc/commands/review.md", "The older review.\n")
	write("cc/commands/frontend/component.md", "Make a component named $0.\n")
	write("proj/.gnosh/skills/deploy/SKILL.md", "Deploy it.\n")
	write("proj/.claude/skills/ship/SKILL.md", "The project's ship.\n")
	p := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }

	s := Load(
		Dirs{Skills: []string{p("own", "skills"), p("cc", "skills"), p("is-not-there")}, Commands: []string{p("own", "commands"), p("cc", "commands")}},
		Dirs{Skills: []string{p("proj", ".gnosh", "skills"), p("proj", ".claude", "skills")}},
	)
	var got []string
	for _, sk := range s.All() {
		got = append(got, sk.Name+"="+sk.Scope+":"+sk.Description)
	}
	want := []string{"deploy=project:Deploy it.", "frontend:component=user:Make a component named $0.", "review=user:Review the change.", "ship=user:gnosh's own ship"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the skills:\n%q\nwant\n%q", got, want)
	}
	if sk, ok := s.Get("frontend:component"); !ok || !sk.Command || sk.Dir != p("cc", "commands", "frontend") {
		t.Errorf("the command file: %+v", sk)
	}
	if sk, _ := s.Get("ship"); sk.Source != p("own", "skills", "ship", "SKILL.md") || sk.Dir != p("own", "skills", "ship") {
		t.Errorf("ship came from %s", sk.Source)
	}
	reasons := map[string]string{}
	for _, sk := range s.Skipped() {
		rel, _ := filepath.Rel(root, sk.Path)
		reasons[rel] = sk.Reason
	}
	for path, want := range map[string]string{
		"cc/skills/ship/SKILL.md":           `another skill named "ship" comes before it`,
		"proj/.claude/skills/ship/SKILL.md": `another skill named "ship" comes before it`,
		"cc/commands/review.md":             `another skill named "review" comes before it`,
		"cc/skills/empty/SKILL.md":          "it holds no instructions",
	} {
		if !strings.Contains(reasons[path], want) {
			t.Errorf("%s: skipped because %q, want %q", path, reasons[path], want)
		}
	}
	if len(reasons) != 4 {
		t.Errorf("skipped: %v", reasons)
	}
}
