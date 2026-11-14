package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/config"
	"github.com/gnosh-ai/gnosh/internal/llm"
)

// doctor checks that a turn could run with this configuration, and returns
// one line for each thing it looked at. It is config.Doctor with the
// probes that look at the real machine.
//
// A look must never disturb a running model server (CFG-61): a server that
// holds one model at a time loads a model as soon as a request names it,
// and throws out the one in use. So the two probes that reach a server do
// exactly this and no more: one opens a connection and closes it, sending
// nothing; the other sends one GET of the address's models list. No
// request of doctor's names a model.
func (p *program) doctor(ctx context.Context, cfg *config.Config, problems []config.Problem) []config.Finding {
	return config.Doctor(ctx, cfg, problems, config.Probes{
		Connect:    dialOnly,
		ListModels: listModelNames,
		LookPath:   exec.LookPath,
		Exists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
	})
}

// listModelNames asks the server at e for its list of models, with one
// GET of the list and nothing else, and returns their names.
func listModelNames(ctx context.Context, pc config.Provider, e config.Endpoint) ([]string, error) {
	u, err := url.Parse(e.URL)
	if err != nil {
		return nil, errors.New("its address cannot be read as a URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/models"
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("its address cannot be read as a URL")
	}
	req.Header.Set("Accept", "application/json")
	switch key := pc.APIKey.Reveal(); {
	case pc.Wire == llm.TypeAnthropic:
		req.Header.Set("anthropic-version", "2023-06-01")
		if key != "" {
			req.Header.Set("x-api-key", key)
		}
	case key != "":
		req.Header.Set("Authorization", "Bearer "+key)
	}
	for _, h := range pc.Headers {
		if !h.Value.Empty() {
			req.Header.Set(h.Name, h.Value.Reveal())
		}
	}
	client := &http.Client{
		// A redirect is not followed: the key must not travel to an
		// address the person did not configure.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		// The error of the HTTP client names the full address, which can
		// hold a token; only what went wrong is passed on.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the server answered %s", resp.Status)
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, errors.New("the answer is not a list of models")
	}
	var names []string
	for _, m := range list.Data {
		if m.ID != "" {
			names = append(names, m.ID)
		}
	}
	if len(names) == 0 {
		for _, m := range list.Models {
			if name := firstOf(m.Name, m.Model); name != "" {
				names = append(names, name)
			}
		}
	}
	return names, nil
}

// doctorCommand is `gnosh doctor`: it checks that a turn could run here
// and says what it found, one line each. It starts no session, sends no
// prompt and asks nobody anything: the folder's own settings, and the
// sub-agent kinds and skills it defines, count when the folder is
// recorded as trusted (gnosh trust) and are reported as held otherwise.
// It says where each kind of sub-agent and each skill came from. It exits
// 1 when a turn cannot run.
func (p *program) doctorCommand(ctx context.Context, inv cli.Invocation) int {
	st, code := p.prepare(ctx, inv, false)
	if st == nil {
		return code
	}
	findings := p.doctor(ctx, st.cfg, st.problems)
	if st.cfg != nil {
		findings = append(findings, defFindings(st.cfg, st.trusted)...)
	}
	for _, f := range findings {
		fmt.Fprintln(p.stdout, f.String())
	}
	if st.cfg != nil {
		for _, h := range st.gitPrograms {
			fmt.Fprintln(p.stdout, config.Finding{Area: "trust", Level: config.Note,
				Text: "the repository's own git settings name a program git would run: " + h.Text + ". gnosh never runs git on a folder's behalf before the folder is trusted."}.String())
		}
		fmt.Fprintln(p.stdout, config.Finding{Area: "sessions", Level: config.Fine,
			Text: "sessions of this folder are kept under " + st.sessions().ProjectDir(st.cfg.Paths.Cwd)}.String())
	}
	fmt.Fprintln(p.stdout)
	if !config.CanRun(findings) {
		fmt.Fprintln(p.stdout, "A turn cannot run until what is marked FAIL is put right.")
		return cli.ExitFailure
	}
	fmt.Fprintln(p.stdout, "A turn can run.")
	return cli.ExitOK
}
