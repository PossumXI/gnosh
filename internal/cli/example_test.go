package cli_test

import (
	"errors"
	"fmt"
	"time"

	"github.com/gnosh-ai/gnosh/internal/cli"
)

// A person at a terminal.
var terminal = cli.Env{StdinIsTerminal: true, StdoutIsTerminal: true}

func ExampleParse() {
	inv, err := cli.Parse([]string{"-y", "-C", "now run the tests"}, terminal)
	fmt.Println(err)
	fmt.Println(inv.Command, inv.Conversation, inv.PermissionMode)
	fmt.Println(inv.Prompt)
	// Output:
	// <nil>
	// interactive continue bypassPermissions
	// now run the tests
}

// Print mode is -p, and flags go on either side of the prompt.
func ExampleParse_print() {
	inv, _ := cli.Parse([]string{"-p", "--cwd", "/work/app", "what does main.go do?", "-m", "local/qwen"}, terminal)
	fmt.Println(inv.Command, inv.Cwd, inv.Model, inv.OutputFormat)
	fmt.Println(inv.Prompt)
	// Output:
	// print /work/app local/qwen text
	// what does main.go do?
}

// A mistyped flag stops everything, and the message is ready to show.
func ExampleParse_error() {
	_, err := cli.Parse([]string{"-p", "hello", "--outptu-format", "json"}, terminal)
	fmt.Println(err)

	var e *cli.Error
	if errors.As(err, &e) {
		fmt.Println(e.Kind, e.Suggestion)
	}
	// Output:
	// --outptu-format is not a flag of gnosh, so nothing was started. Did you mean --output-format?
	// unknown_flag --output-format
}

// A flag whose feature is not built yet is refused, until the code that
// builds the feature says it is there.
func ExampleTable_SetStatus() {
	table := cli.Default()
	_, err := table.Parse([]string{"-C", "--fork-session"}, terminal)
	fmt.Println(err)

	if err := table.SetStatus("--fork-session", cli.Built); err != nil {
		fmt.Println(err)
	}
	inv, err := table.Parse([]string{"-C", "--fork-session"}, terminal)
	fmt.Println(inv.ForkSession, err)
	// Output:
	// --fork-session is not built yet in this build of gnosh, so nothing was started. Leave it out for now.
	// true <nil>
}

func ExampleVersionLine() {
	fmt.Println(cli.VersionLine(cli.Build{
		Time:   time.Date(2026, 11, 13, 9, 5, 22, 0, time.UTC),
		Commit: "bfdb0d9",
	}))
	// Output:
	// 2026-11-13T09:05:22Z bfdb0d9 (gnosh)
}
