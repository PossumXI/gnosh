package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
)

// This file is a run in the background whose permission questions a
// person answers from the agent view, as Claude Code's background sessions
// wait under Needs input (code.claude.com/docs/en/agent-view) where gnosh's
// used to refuse every call that asks. gnosh --bg, /background and /fork
// give the run the base of its log's name in AsksEnv; the run then puts
// each permission question in <base>.ask and waits, with no clock, for
// <base>.answer, which the view writes (runSource.Answer). Only permission
// questions go so: a key's password, the model's own questions and an MCP
// server's still have nobody to ask. Each question carries a random token
// that the answer must give back, and the token is in no file the model
// was shown, so a model that wrote the answer file itself would not be
// heard.

// AsksEnv names the base of the files a run's questions go through: the
// path of its log without .log. gnosh takes it out of its environment as
// it starts, as it does RestartedEnv: nothing it starts may see it.
const AsksEnv = "GNOSH_RUN_ASKS"

// runEnv is the environment of a run in the background whose log is
// log: this one's, and where its questions go.
func runEnv(environ []string, log string) []string {
	env := slices.DeleteFunc(slices.Clone(environ), func(kv string) bool { return strings.HasPrefix(kv, AsksEnv+"=") })
	return append(env, AsksEnv+"="+strings.TrimSuffix(log, ".log"))
}

// askLook is how often a run looks for the answer to its question.
const askLook = 300 * time.Millisecond

// runAsk is a question a run waits on, as its .ask file holds it.
type runAsk struct {
	ID      string      `json:"id"`
	Token   string      `json:"token"`
	Title   string      `json:"title"`
	Text    string      `json:"text"`
	Choices []askChoice `json:"choices"`
	Since   time.Time   `json:"since"`
}

// askChoice is an answer a question offers.
type askChoice struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// runAnswer is the answer to it, as the .answer file holds it.
type runAnswer struct {
	ID     string `json:"id"`
	Token  string `json:"token"`
	Choice string `json:"choice"`
	Text   string `json:"text,omitempty"`
}

// askFile and answerFile are where the questions of the run whose log is
// log go.
func askFile(log string) string    { return strings.TrimSuffix(log, ".log") + ".ask" }
func answerFile(log string) string { return strings.TrimSuffix(log, ".log") + ".answer" }

// writeFileAtomic writes data to path in one step: a reader sees the old
// file or the whole new one.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// relayAsk puts the permission question q of the run whose files begin at
// base in its .ask file, waits for the answer the agent view writes, and
// hands it to answer. It returns when the question is answered or ctx
// ends (the question was taken back, the run is ending); the .ask file
// goes with it either way. A question that cannot be written down is
// answered no, with why, so that the run does not wait on nobody.
func relayAsk(ctx context.Context, base string, q agent.Question, now func() time.Time, answer func(agent.Answer) error) {
	var b [16]byte
	_, _ = rand.Read(b[:])
	ask := runAsk{ID: q.ID, Token: hex.EncodeToString(b[:]), Title: q.Title, Text: q.Text, Since: now().UTC()}
	for _, c := range q.Choices {
		ask.Choices = append(ask.Choices, askChoice{Key: c.Key, Label: c.Label})
	}
	data, _ := json.Marshal(ask)
	asks, answers := base+".ask", base+".answer"
	_ = os.Remove(answers) // (an answer to an earlier question)
	if err := writeFileAtomic(asks, data); err != nil {
		_ = answer(agent.Answer{Choice: "no", Text: fmt.Sprintf("(gnosh could not put the question to the agent view: %v. Nobody saw it.)", err)})
		return
	}
	defer os.Remove(asks)
	tick := time.NewTicker(askLook)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		data, err := os.ReadFile(answers)
		if err != nil {
			continue
		}
		var a runAnswer
		if json.Unmarshal(data, &a) != nil || a.ID != ask.ID || a.Token != ask.Token {
			// Not the answer to this question: it is not heard.
			_ = os.Remove(answers)
			continue
		}
		_ = os.Remove(answers)
		_ = answer(agent.Answer{Choice: a.Choice, Text: a.Text})
		return
	}
}

// readAsk is the question the run whose log is log waits on, if any.
func readAsk(log string) *runAsk {
	data, err := os.ReadFile(askFile(log))
	if err != nil {
		return nil
	}
	var a runAsk
	if json.Unmarshal(data, &a) != nil || a.ID == "" || a.Token == "" {
		return nil
	}
	return &a
}

// answerAsk writes the answer the person chose to the question the run
// whose log is log waits on: choice is the key of one of its choices, and
// text words that go with it.
func answerAsk(log, id, choice, text string) error {
	a := readAsk(log)
	if a == nil || a.ID != id {
		return fmt.Errorf("the question was answered or taken back already")
	}
	known := false
	for _, c := range a.Choices {
		known = known || c.Key == choice
	}
	if !known {
		return fmt.Errorf("%q is not one of the question's answers", choice)
	}
	data, _ := json.Marshal(runAnswer{ID: a.ID, Token: a.Token, Choice: choice, Text: text})
	return writeFileAtomic(answerFile(log), data)
}
