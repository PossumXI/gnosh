package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/shellquote"
)

// This file is the gnokey tool (spec/gnosh/harness-tools.md, TOOL-40 and
// TOOL-41; Jae asked for "a real Gno integration (gno toolchain, realm
// deploy, gnokey signing)"). The model runs gnokey, gno.land's tool for
// keys and transactions, with no shell. When the command signs, gnosh asks
// the person for the key's password at a masked prompt that names the key
// and shows the exact command, hands the password to gnokey on its
// standard input (by gnokey's own flag for that, which gnosh puts in
// itself), and takes it out of what gnokey printed. The model never sees
// the password, and neither do the session's file, its log or a hook.
// Commands that make, change or hand out keys are refused: they stay in
// the person's own terminal. Only a session somebody attends has the tool,
// and only where gnokey is installed; the shell refuses the password flag
// (shelltool), so that nothing but this tool's prompt can sign.

const gnokeyToolName = "gnokey"

const (
	// gnokeyPasswordFlag is gnokey's flag for a password on its standard
	// input: the one way it takes a password without a terminal.
	gnokeyPasswordFlag = "-insecure-password-stdin"
	// gnokeyOutput is the most of gnokey's output the model is given, and
	// gnokeyWait how long a gnokey command may run.
	gnokeyOutput = 30000
	gnokeyWait   = 5 * time.Minute
)

// gnokeyCommands are gnokey's commands, by which a command line's own is
// told from the flags before it; gnokeyKeys are those that make, change
// or hand out keys.
var (
	gnokeyCommands = []string{"add", "delete", "rotate", "import", "export", "generate", "recover", "list", "sign", "verify", "query", "broadcast", "maketx", "multisign", "version"}
	gnokeyKeys     = []string{"add", "delete", "rotate", "import", "export", "generate", "recover"}
)

// gnokeyMainnet names gno.land's main network, by its chain ID and the
// host of its node: what is signed there is real.
var (
	gnokeyMainnetIDs  = []string{"gnoland1", "gnoland-1"}
	gnokeyMainnetHost = "rpc.gno.land"
)

const gnokeyDescription = `Runs gnokey, gno.land's tool for keys and transactions, with the command line you give, and returns what it printed. There is no shell: give gnokey's words alone, quoted as you would type them, with no pipe, redirection or variable.

- Queries (gnokey query), listing keys (gnokey list), verifying, broadcasting a transaction that is already signed, and making one with -broadcast=false (maketx then prints it unsigned) run as they are.
- A command that signs (maketx, which signs and broadcasts unless given -broadcast=false; gnokey sign) asks the user for the key's password at a masked prompt that shows them the exact command. You never see the password: never ask the user for it, never put it in a command, and never try to sign in the shell. Do not pass -insecure-password-stdin: gnosh adds what is needed. If the user does not give the password, nothing is signed; say so and stop.
- Making, changing or handing out keys (add, delete, rotate, import, export, generate, recover) is refused: tell the user the command to run in their own terminal.

The key's name is the last word, as gnokey takes it. On gno.land's main network (chain ID gnoland1) a transaction moves real value: say so before you sign there.`

var gnokeySchema = json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"The gnokey command line, beginning with gnokey, as it would be typed."},"description":{"type":"string","description":"One line saying what the command does, for the user."},"working_dir":{"type":"string","description":"Optional: the directory to run it in, such as one that holds a package to add. The working directory when left out."}},"required":["command","description"],"additionalProperties":false}`)

// gnokeyTool is the gnokey tool of one session; bin is gnokey's path.
type gnokeyTool struct {
	l   *live
	bin string
}

// gnokeyCall is a call of the tool, read: gnokey's arguments, the command
// they are, and, for one that signs, the key, and whether the chain is
// gno.land's main one.
type gnokeyCall struct {
	args        []string
	command     string
	signs       bool
	key         string
	mainnet     bool
	dir         string
	description string
}

func (t *gnokeyTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: gnokeyToolName, Description: gnokeyDescription, InputSchema: gnokeySchema}
}

// read reads a call, and says in words for the model what is wrong with it
// when something is.
func (t *gnokeyTool) read(input json.RawMessage, cwd string) (gnokeyCall, string) {
	var in struct {
		Command     string `json:"command"`
		Description string `json:"description"`
		WorkingDir  string `json:"working_dir"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return gnokeyCall{}, fmt.Sprintf("the arguments cannot be read (%v): command is the gnokey command line, description one line saying what it does", err)
	}
	words, err := shellquote.Split(strings.TrimSpace(in.Command))
	if err != nil {
		return gnokeyCall{}, "the command cannot be read as gnokey's words: " + err.Error()
	}
	if len(words) == 0 || words[0] != gnokeyToolName {
		return gnokeyCall{}, `the command must begin with gnokey: give the gnokey command line as it would be typed ("gnokey query auth/accounts/g1...")`
	}
	c := gnokeyCall{args: words[1:], description: strings.TrimSpace(in.Description), dir: cwd}
	for _, w := range c.args {
		if name, _, _ := strings.Cut(strings.TrimLeft(w, "-"), "="); strings.HasPrefix(w, "-") && name == strings.TrimLeft(gnokeyPasswordFlag, "-") {
			return c, "the command passes gnokey's " + gnokeyPasswordFlag + " itself, which only gnosh may do: leave it out. When the command signs, gnosh asks the user for the key's password and hands it to gnokey itself"
		}
	}
	for _, w := range c.args {
		if slices.Contains(gnokeyCommands, w) {
			c.command = w
			break
		}
	}
	if slices.Contains(gnokeyKeys, c.command) {
		return c, fmt.Sprintf("gnokey %s makes, changes or hands out a key, which stays in the user's own terminal: tell the user the command to run there", c.command)
	}
	switch c.command {
	case "sign":
		c.signs = true
	case "maketx":
		c.signs = !gnokeyUnsigned(c.args)
	}
	if c.signs {
		if n := len(c.args); n > 0 && !strings.HasPrefix(c.args[n-1], "-") {
			c.key = c.args[n-1]
		}
		if c.key == "" {
			return c, "the command signs, and its last word, the key's name, is missing: gnokey takes the key that signs as its last word"
		}
	}
	c.mainnet = gnokeyOnMainnet(c.args)
	if d := strings.TrimSpace(in.WorkingDir); d != "" {
		if !filepath.IsAbs(d) {
			d = filepath.Join(cwd, d)
		}
		if info, err := os.Stat(d); err != nil || !info.IsDir() {
			return c, fmt.Sprintf("working_dir %s is not a directory that is there", d)
		}
		c.dir = filepath.Clean(d)
	}
	return c, ""
}

// gnokeyUnsigned reports whether gnokey maketx's arguments ask for the
// transaction unsigned, which -broadcast=false does: maketx signs, then
// simulates and broadcasts, unless told not to broadcast (gnokey of
// November 2026; an older one signs either way, and without a password
// it then fails, having signed nothing).
func gnokeyUnsigned(args []string) bool {
	off := false
	for _, w := range args {
		name, value, set := strings.Cut(strings.TrimLeft(w, "-"), "=")
		if !strings.HasPrefix(w, "-") || name != "broadcast" {
			continue
		}
		off = set && slices.Contains([]string{"false", "0", "f", "F", "FALSE", "False"}, value)
	}
	return off
}

// gnokeyOnMainnet reports whether the arguments name gno.land's main
// network, by its chain ID or its node.
func gnokeyOnMainnet(args []string) bool {
	value := func(i int, name string) (string, bool) {
		w := args[i]
		if !strings.HasPrefix(w, "-") {
			return "", false
		}
		n, v, set := strings.Cut(strings.TrimLeft(w, "-"), "=")
		switch {
		case n != name:
			return "", false
		case set:
			return v, true
		case i+1 < len(args):
			return args[i+1], true
		}
		return "", false
	}
	for i := range args {
		if id, ok := value(i, "chainid"); ok && slices.Contains(gnokeyMainnetIDs, id) {
			return true
		}
		if remote, ok := value(i, "remote"); ok {
			host := remote
			if u, err := url.Parse(remote); err == nil && u.Host != "" {
				host = u.Host
			}
			if h, _, _ := strings.Cut(host, ":"); strings.EqualFold(h, gnokeyMainnetHost) {
				return true
			}
		}
	}
	return false
}

// line is the command as gnokey is run with it, without the password
// flag gnosh adds: what the person is shown.
func (c gnokeyCall) line() string {
	return gnokeyToolName + " " + shellquote.Words(c.args)
}

// Describe says what a call would run, so that the person is asked about
// the command itself, as for a shell command.
func (t *gnokeyTool) Describe(call core.ToolCall, env *core.ToolEnv) (core.Action, error) {
	c, wrong := t.read(call.Input, env.Cwd)
	act := core.Action{Kind: core.ActionExecute, Summary: firstOf(c.description, "Run gnokey")}
	if len(c.args) > 0 {
		act.Command = c.line()
	}
	if wrong != "" {
		return act, errors.New(wrong)
	}
	return act, nil
}

// Run runs gnokey, asking for the key's password first when the command
// signs.
func (t *gnokeyTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	c, wrong := t.read(call.Input, env.Cwd)
	if wrong != "" {
		return core.ErrorResult("gnokey was not run: " + wrong + "."), nil
	}
	args := c.args
	var password string
	if c.signs {
		a := agent.FromContext(ctx)
		if a == nil {
			return core.ToolResult{}, errors.New("the gnokey tool was run outside a session")
		}
		body := []string{"gnokey is to sign with the key " + c.key + ":", "  " + c.line()}
		if c.mainnet {
			body = append(body, "This is gno.land's main network: what it signs moves real value.")
		}
		body = append(body, "The password goes to gnokey alone: the model, the session's file and its log never see it. Esc signs nothing and stops the turn.")
		ans, err := a.Ask(ctx, agent.Question{Kind: secretKind, Title: "Password for the key " + c.key, Text: strings.Join(body, "\n"), CallID: call.ID, Tool: gnokeyToolName})
		switch {
		case errors.Is(err, agent.ErrNobodyToAsk):
			return core.ErrorResult("gnokey was not run: the command signs, and nobody is at the terminal to type the key's password. Give the user the exact command to run in their own terminal, and stop."), nil
		case err != nil:
			return core.ToolResult{}, err
		}
		if password = ans.Text; password == "" {
			return core.ErrorResult("gnokey was not run: no password was given, so nothing was signed. Say so, and stop."), nil
		}
		// The flag goes before the key, among the flags of the command
		// that signs, where gnokey reads it.
		args = slices.Insert(slices.Clone(args), len(args)-1, gnokeyPasswordFlag)
	}
	runCtx, cancel := context.WithTimeout(ctx, gnokeyWait)
	defer cancel()
	cmd := exec.CommandContext(runCtx, t.bin, args...)
	cmd.Dir = c.dir
	cmd.Env = slices.Clone(t.l.p.o.Environ)
	for _, kv := range t.l.cfg.CommandEnv() {
		if strings.Contains(kv, "=") { // (a bare name takes one out, for the shell)
			cmd.Env = append(cmd.Env, kv)
		}
	}
	if c.signs {
		cmd.Stdin = strings.NewReader(password + "\n")
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	runErr := cmd.Run()
	text := out.String()
	if len(password) >= 3 {
		text = strings.ReplaceAll(text, password, "[the password]")
	}
	password = ""
	if len(text) > gnokeyOutput {
		text = text[:gnokeyOutput] + fmt.Sprintf("\n[%d more bytes of gnokey's output were left out]", len(text)-gnokeyOutput)
	}
	text = strings.TrimRight(text, "\n")
	var exit *exec.ExitError
	switch {
	case runErr == nil:
		return core.TextResult(firstOf(text, "(gnokey printed nothing)")), nil
	case errors.As(runErr, &exit):
		said := fmt.Sprintf("gnokey exited with status %d.", exit.ExitCode())
		if runCtx.Err() != nil && ctx.Err() == nil {
			said = fmt.Sprintf("gnokey did not end within %s and was stopped.", roundTime(gnokeyWait))
		}
		if text != "" {
			said += "\n" + text
		}
		return core.ErrorResult(said), nil
	case ctx.Err() != nil:
		return core.ToolResult{}, ctx.Err()
	}
	return core.ErrorResult("gnokey could not be run: " + runErr.Error() + "."), nil
}
