package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// llamaCpp is a chat-completions provider that talks to llama.cpp's own
// server (llama-server). What it adds to the plain format is in
// llamaFields; besides that it can ask the server how big a model's
// context is and how many tokens a prompt takes.
type llamaCpp struct{ openAI }

var (
	_ core.Provider     = (*llamaCpp)(nil)
	_ core.ModelLister  = (*llamaCpp)(nil)
	_ core.TokenCounter = (*llamaCpp)(nil)
)

// llamaFields writes the request fields that are llama.cpp's own.
func (o *openAI) llamaFields(body obj, req *core.Request) {
	// The server keeps the last prompt's state and only reads what
	// changed since. It is the server's default; saying it costs nothing
	// and holds if a server was started with the default switched off.
	body["cache_prompt"] = true

	// Thinking is set through the chat template, which is where
	// llama.cpp's models take it from: an on/off thinker such as Qwen
	// reads enable_thinking, a model with levels such as gpt-oss reads
	// reasoning_effort, and each ignores the other's.
	kwargs := obj{}
	switch req.Thinking.Mode {
	case "on":
		kwargs["enable_thinking"] = true
	case "off":
		kwargs["enable_thinking"] = false
	}
	if req.Thinking.Effort != "" {
		kwargs["reasoning_effort"] = req.Thinking.Effort
		delete(body, "reasoning_effort")
	}
	if len(kwargs) > 0 {
		body["chat_template_kwargs"] = kwargs
	}
	// A budget for the thinking: the server ends the reasoning there, with
	// the note, and the model goes on to its answer. (A server too old to
	// know the fields thinks as long as it likes, as before.)
	if t := req.Thinking; t.BudgetTokens > 0 && t.Mode != "off" {
		body["reasoning_budget_tokens"] = t.BudgetTokens
		if t.BudgetNote != "" {
			body["reasoning_budget_message"] = t.BudgetNote
		}
	}

	// The server allows one tool call per answer unless asked for more.
	// The other two formats allow several by default, and a harness that
	// reads five files wants one turn for it, not five.
	if len(req.Tools) > 0 {
		body["parallel_tool_calls"] = true
	}
}

// Models lists the models the server has. For a server that runs one
// model, the model's ContextWindow is the context size the server runs it
// with, n_ctx in the server's /props.
//
// A server that holds several models (its "router mode") loads a model
// when a request names it, and puts the one in use out of memory to make
// room. So Models never names a model: such a server is asked for its
// list and nothing else, and the ContextWindow of its models is zero,
// which means unknown. The window of a model on such a server has to come
// from the configuration.
func (l *llamaCpp) Models(ctx context.Context) ([]core.ModelInfo, error) {
	list, err := l.modelList(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]core.ModelInfo, 0, len(list.Data))
	router := false
	for _, m := range list.Data {
		info := core.ModelInfo{ID: m.ID}
		// Only a router says whether a model is loaded, and the context
		// it is run with.
		if m.Status != nil {
			router, info.State = true, m.Status.Value
		}
		if m.ContextLength != nil {
			info.ContextWindow = m.ContextLength.count()
		}
		out = append(out, info)
	}
	// A server that runs one model lists exactly one. Anything else is
	// taken for a router: /props could only speak for one of its models.
	if router || len(out) != 1 {
		return out, nil
	}
	window, err := l.contextSize(ctx)
	if err != nil {
		return nil, err
	}
	out[0].ContextWindow = window
	return out, nil
}

// Load asks the server how busy it is: its /slots, each with whether a
// request is in it, how long that request's prompt is, and how much of it
// has been read (from the cache, or worked through). A server that holds
// several models answers for the one named; one that runs a single model
// takes no notice of the name. A server that keeps its slots to itself
// (--no-slots) answers with an error.
func (l *llamaCpp) Load(ctx context.Context, model string) (core.ServerLoad, error) {
	var query url.Values
	if model != "" {
		query = url.Values{"model": {model}}
	}
	body, err := l.call(ctx, http.MethodGet, l.rootEndpoint("/slots", query), nil)
	if err != nil {
		return core.ServerLoad{}, err
	}
	var slots []struct {
		Busy      bool `json:"is_processing"`
		Prompt    *num `json:"n_prompt_tokens"`
		Processed *num `json:"n_prompt_tokens_processed"`
		Cached    *num `json:"n_prompt_tokens_cache"`
	}
	if err := json.Unmarshal(body, &slots); err != nil {
		return core.ServerLoad{}, l.fail(core.ErrBadResponse, "the server's slots cannot be read: %v", err)
	}
	var load core.ServerLoad
	for _, s := range slots {
		prompt := countOf(s.Prompt)
		load.Slots = append(load.Slots, core.SlotLoad{Busy: s.Busy, Prompt: prompt, Read: min(countOf(s.Processed)+countOf(s.Cached), prompt)})
	}
	return load, nil
}

// contextSize asks the server's /props for the context size it runs its
// model with. The question names no model. A server that has no such
// thing to say gives zero.
func (l *llamaCpp) contextSize(ctx context.Context) (int, error) {
	body, err := l.call(ctx, http.MethodGet, l.rootEndpoint("/props", nil), nil)
	if err != nil {
		var pe *core.ProviderError
		if errors.As(err, &pe) && (pe.Kind == core.ErrInvalidRequest || pe.Kind == core.ErrBadResponse) {
			return 0, nil // The server has no /props.
		}
		return 0, err
	}
	var props struct {
		Settings struct {
			NCtx *num `json:"n_ctx"`
		} `json:"default_generation_settings"`
	}
	if json.Unmarshal(body, &props) != nil {
		return 0, nil
	}
	return countOf(props.Settings.NCtx), nil
}

// CountTokens asks the server how many tokens req's prompt takes, without
// running it.
//
// The server has an endpoint for exactly that, which takes the same body
// as a chat completion: /v1/chat/completions/input_tokens. A server built
// before that endpoint existed answers 404; then the count is made from
// two older endpoints instead, /apply-template, which renders the prompt
// the chat template would build, and /tokenize, which splits it into
// tokens. That count leaves out what the template does not render as
// text, images above all.
func (l *llamaCpp) CountTokens(ctx context.Context, req *core.Request) (int, error) {
	if req == nil {
		return 0, l.fail(core.ErrInvalidRequest, "there is no request to count")
	}
	if l.noImages.Load() {
		req = withoutImages(req)
	}
	n, err := l.countTokens(ctx, req)
	if err != nil && hasImages(req) && imagesRefused(err) {
		// The server takes no images: counted, and from now on sent,
		// without them (images.go).
		l.noImages.Store(true)
		return l.countTokens(ctx, withoutImages(req))
	}
	return n, err
}

// countTokens is CountTokens for the request as it is.
func (l *llamaCpp) countTokens(ctx context.Context, req *core.Request) (int, error) {
	chat, err := l.chatBody(req, false)
	if err != nil {
		return 0, err
	}
	body, err := l.encodeBody(chat, req)
	if err != nil {
		return 0, err
	}

	answer, err := l.call(ctx, http.MethodPost, l.endpoint("/chat/completions/input_tokens", nil), body)
	if err == nil {
		var counted struct {
			InputTokens *num `json:"input_tokens"`
		}
		if json.Unmarshal(answer, &counted) != nil || counted.InputTokens == nil {
			e := l.fail(core.ErrBadResponse, "the server's token count cannot be read")
			e.Body = string(clip(answer, bodyStart))
			return 0, e
		}
		return counted.InputTokens.count(), nil
	}
	var pe *core.ProviderError
	if !errors.As(err, &pe) || pe.Status != http.StatusNotFound {
		return 0, err
	}

	answer, err = l.call(ctx, http.MethodPost, l.rootEndpoint("/apply-template", nil), body)
	if err != nil {
		return 0, err
	}
	var rendered struct {
		Prompt *string `json:"prompt"`
	}
	if json.Unmarshal(answer, &rendered) != nil || rendered.Prompt == nil {
		e := l.fail(core.ErrBadResponse, "the prompt the server rendered cannot be read")
		e.Body = string(clip(answer, bodyStart))
		return 0, e
	}
	ask := obj{"content": *rendered.Prompt}
	if req.Model != "" {
		ask["model"] = req.Model // A server with several models needs to know whose tokens.
	}
	body, err = encodeJSON(ask)
	if err != nil {
		return 0, l.fail(core.ErrInvalidRequest, "the request cannot be written as JSON: %v", err)
	}
	answer, err = l.call(ctx, http.MethodPost, l.rootEndpoint("/tokenize", nil), body)
	if err != nil {
		return 0, err
	}
	var tokens struct {
		Tokens []json.RawMessage `json:"tokens"`
	}
	if json.Unmarshal(answer, &tokens) != nil {
		e := l.fail(core.ErrBadResponse, "the server's token list cannot be read")
		e.Body = string(clip(answer, bodyStart))
		return 0, e
	}
	return len(tokens.Tokens), nil
}
