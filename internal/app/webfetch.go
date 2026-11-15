package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/tools/webtool"
)

// This file is the WebFetch tool (spec/cc/tools.md, section 10): a page
// from the web, read by a separate request to the small model with the
// model's request about it, so that the model reads an answer and not the
// raw page. The fetching itself, and what is refused, is webtool's.

const webFetchName = "WebFetch"

// webFetchMaxChars is how much of a page one call reads; the rest is read
// with offset.
const webFetchMaxChars = 100_000

// webFetchCacheFor is how long a fetched page is kept for another call.
const webFetchCacheFor = 15 * time.Minute

const webFetchDescription = `Fetches a page from the web and answers a request about it.

The page is turned into Markdown, and a separate, smaller model reads it with your request and answers; you get that answer, not the raw page, so say exactly what you need from it (a command, a function's signature, the steps of a procedure, whether it says something). The answer may leave out what you did not ask for.

- url: http or https, on the open web. http is upgraded to https. A host on this machine or on a private network (localhost, a name without a dot, 192.168.x.x and the like) is refused: use curl through Bash for those.
- A redirect to another host is not followed: the result says where it leads, and you can fetch that address yourself.
- At most 100,000 characters of a page are read in one call; the result says how much is left, and offset reads on from there.
- A page is kept for 15 minutes, so asking again about the same page fetches nothing new.
- Each fetch asks the user first, unless they have allowed the site.`

var webFetchSchema = json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"The page's address: http or https, on the open web."},"prompt":{"type":"string","description":"What you need from the page."},"offset":{"type":"integer","minimum":0,"description":"How many characters of the page to skip, to read on in a long page."}},"required":["url","prompt"],"additionalProperties":false}`)

// webFetchSystem is what the small model is told when it reads a page.
const webFetchSystem = `You read one web page for another model and answer its request from what the page says, and from nothing else. Where exact text matters (code, commands, names, numbers, quotations), give it exactly as the page has it. If the page does not say what was asked, say so plainly, and say what it is about instead. Be brief: the answer goes to a model that will act on it.`

// fetchPage fetches a page. Tests put a page of their own in its place.
var fetchPage = webtool.Fetch

type webFetchInput struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt"`
	Offset int    `json:"offset"`
}

// webFetchTool is the tool of one session.
type webFetchTool struct {
	l *live

	mu    sync.Mutex
	cache map[string]cachedPage
}

type cachedPage struct {
	page webtool.Page
	at   time.Time
}

func (t *webFetchTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: webFetchName, Description: webFetchDescription, InputSchema: webFetchSchema}
}

// read reads a call's arguments and says, in words for the model, what is
// wrong with them when something is.
func (t *webFetchTool) read(input json.RawMessage) (webFetchInput, string) {
	var in webFetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return in, fmt.Sprintf("the arguments cannot be read (%v): url and prompt are strings, offset a number", err)
	}
	in.URL, in.Prompt = strings.TrimSpace(in.URL), strings.TrimSpace(in.Prompt)
	switch {
	case in.URL == "":
		return in, "url is empty"
	case in.Prompt == "":
		return in, "prompt is empty: say what you need from the page"
	case in.Offset < 0:
		return in, "offset is below zero"
	}
	return in, ""
}

// Describe says what a call would do: fetch from a host. A URL that would
// be refused is said to be one now, before anybody is asked about it.
func (t *webFetchTool) Describe(call core.ToolCall, env *core.ToolEnv) (core.Action, error) {
	in, wrong := t.read(call.Input)
	act := core.Action{Kind: core.ActionNetwork, URL: in.URL, Summary: "Fetch a web page"}
	if wrong != "" {
		return act, errors.New(wrong)
	}
	u, err := webtool.Check(in.URL, false)
	if err != nil {
		return act, err
	}
	act.URL, act.Summary = u.String(), "Fetch "+u.Hostname()
	return act, nil
}

// Run fetches the page, or takes it from the cache, and has the small model
// answer the request about it.
func (t *webFetchTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	in, wrong := t.read(call.Input)
	if wrong != "" {
		return core.ErrorResult("Nothing was fetched: " + wrong + "."), nil
	}
	u, err := webtool.Check(in.URL, false)
	if err != nil {
		return core.ErrorResult("Nothing was fetched: " + err.Error() + "."), nil
	}
	key := u.String()
	page, cached := t.cached(key)
	if !cached {
		page, err = fetchPage(ctx, key, webtool.Options{UserAgent: "gnosh/" + firstOf(t.l.p.o.BuildID, "devel")})
		if err != nil {
			return core.ErrorResult(fmt.Sprintf("%s was not fetched: %v.", key, err)), nil
		}
		t.keep(key, page)
	}
	meta := map[string]any{"bytes": page.Bytes, "code": page.Code, "codeText": page.Status, "durationMs": page.Took.Milliseconds(), "url": page.URL}
	if page.Redirect != "" {
		res := core.TextResult(fmt.Sprintf("%s redirects to %s, which is on another host, and WebFetch does not follow that. Fetch %s itself if it is the page you want.", key, page.Redirect, page.Redirect))
		res.Meta = meta
		return res, nil
	}
	if page.Code >= 400 {
		res := core.ErrorResult(fmt.Sprintf("%s answered %d %s. What it sent begins:\n%s", page.URL, page.Code, page.Status, clipChars(page.Text, 0, 2000)))
		res.Meta = meta
		return res, nil
	}
	total := utf8.RuneCountInString(page.Text)
	if in.Offset >= total && total > 0 {
		return core.ErrorResult(fmt.Sprintf("The page has %d characters, so there is nothing from offset %d on.", total, in.Offset)), nil
	}
	window := clipChars(page.Text, in.Offset, webFetchMaxChars)
	read := utf8.RuneCountInString(window)
	from := ""
	if in.Offset > 0 {
		from = fmt.Sprintf(" (from character %d of %d)", in.Offset, total)
	}
	answer, err := t.l.agent.Side(ctx, agent.SideRequest{
		Purpose: "fetch",
		System:  webFetchSystem,
		Prompt:  fmt.Sprintf("The page %s%s:\n<page>\n%s\n</page>\n\nThe request: %s", page.URL, from, window, in.Prompt),
		Timeout: 5 * time.Minute,
	})
	var text string
	if err != nil || strings.TrimSpace(answer.Text) == "" {
		// The page itself, then, as far as it fits: an answer the model can
		// still use.
		why := "it answered nothing"
		if err != nil {
			why = err.Error()
		}
		text = fmt.Sprintf("The page could not be read for you by the smaller model (%s), so here is the beginning of its text%s:\n%s", why, from, clipChars(window, 0, 20_000))
	} else {
		text = strings.TrimSpace(answer.Text)
	}
	if left := total - in.Offset - read; left > 0 {
		text += fmt.Sprintf("\n[%d of the page's %d characters were read; %d are left, and WebFetch with offset %d reads on.]", read, total, left, in.Offset+read)
	}
	if page.Cut {
		text += "\n[The page was longer than what is fetched, and what is past that is not here.]"
	}
	res := core.TextResult(text)
	res.Meta = meta
	return res, nil
}

func (t *webFetchTool) cached(key string) (webtool.Page, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	c, ok := t.cache[key]
	if !ok || t.l.p.now().Sub(c.at) > webFetchCacheFor {
		return webtool.Page{}, false
	}
	return c.page, true
}

func (t *webFetchTool) keep(key string, p webtool.Page) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cache == nil {
		t.cache = map[string]cachedPage{}
	}
	now := t.l.p.now()
	for k, c := range t.cache {
		if now.Sub(c.at) > webFetchCacheFor {
			delete(t.cache, k)
		}
	}
	t.cache[key] = cachedPage{page: p, at: now}
}

// clipChars returns at most n characters of s from character from on.
func clipChars(s string, from, n int) string {
	start, i := -1, 0
	for at := range s {
		if i == from {
			start = at
		}
		if start >= 0 && i == from+n {
			return s[start:at]
		}
		i++
	}
	if start < 0 {
		return ""
	}
	return s[start:]
}
