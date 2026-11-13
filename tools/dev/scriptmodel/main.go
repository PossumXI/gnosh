// Command scriptmodel is a model server that answers from a script, for
// trying gnosh by hand without a real model: it speaks the chat-completions
// format a llama.cpp server speaks, and its replies are read from a file.
//
//	go run ./tools/dev/scriptmodel script.json
//
// It prints its address and serves until it is stopped. The script is a
// JSON list of replies, given to the chat requests in order:
//
//	[
//	  {"calls": [{"name": "Read", "args": {"file_path": "a.txt"}}]},
//	  {"thinking": "It is a sum.", "text": "The answer is 4."}
//	]
//
// Point gnosh at it with two lines of gnoshrc:
//
//	provider add local --type llamacpp --base-url http://127.0.0.1:PORT
//	model large local/test-model
//
// It is the same server the tests of internal/app run against
// (internal/app/apptest).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// reply is one reply of the script file.
type reply struct {
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
	Calls    []struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	} `json:"calls"`
	Status int    `json:"status"`
	Body   string `json:"body"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: scriptmodel SCRIPT.json")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "scriptmodel:", err)
		os.Exit(1)
	}
	var script []reply
	if err := json.Unmarshal(data, &script); err != nil {
		fmt.Fprintf(os.Stderr, "scriptmodel: %s is not a JSON list of replies: %v\n", os.Args[1], err)
		os.Exit(1)
	}
	var replies []apptest.Reply
	for _, r := range script {
		rep := apptest.Reply{Text: r.Text, Thinking: r.Thinking, Status: r.Status, Body: r.Body}
		for i, c := range r.Calls {
			args := string(c.Args)
			if args == "" {
				args = "{}"
			}
			rep.Calls = append(rep.Calls, apptest.Call{ID: fmt.Sprintf("call_%d", i), Name: c.Name, Args: args})
		}
		replies = append(replies, rep)
	}
	m := apptest.StartModel(replies...)
	defer m.Close()
	fmt.Println(m.URL)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}
