package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// chunk writes one chat-completions chunk as a server-sent event. The
// streams in these tests are written by hand from the format's public
// description.
func chunk(body string) string {
	return `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"m",` + body + "}"
}

// delta is a chunk whose only choice carries the given delta.
func delta(d string) string {
	return chunk(`"choices":[{"index":0,"delta":` + d + `,"finish_reason":null}]`)
}

// finish is a chunk that gives the finish reason.
func finish(reason string) string {
	return chunk(`"choices":[{"index":0,"delta":{},"finish_reason":"` + reason + `"}]`)
}

const (
	roleChunk = `{"role":"assistant","content":""}`
	doneLine  = "data: [DONE]"
)

// usageChunk is the extra chunk a server sends when asked to include
// usage.
func usageChunk(usage string) string {
	return chunk(`"choices":[],"usage":` + usage)
}

type streamCase struct {
	name    string
	stream  string
	events  []string
	message []string
	change  func(*Config)
}

func runStreamCases(t *testing.T, typ string, cases []streamCase) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t, playing(tc.stream))
			res := ask(f.provider(typ, tc.change))
			wantOK(t, res)
			wantLines(t, "events", res.events, tc.events)
			wantLines(t, "message", res.message, tc.message)
			f.waitClosed()
		})
	}
}

func TestOpenAIStreams(t *testing.T) {
	runStreamCases(t, TypeOpenAI, []streamCase{
		{
			name: "plain text",
			stream: events(
				delta(roleChunk),
				delta(`{"content":"Hel"}`),
				delta(`{"content":"lo"}`),
				delta(`{"content":"!"}`),
				finish("stop"),
				usageChunk(`{"prompt_tokens":9,"completion_tokens":3,"total_tokens":12}`),
				doneLine,
			),
			events: []string{
				`text "Hel"`, `text "lo"`, `text "!"`,
				`usage in=9 out=3 read=0 write=0`,
				`stop end_turn`,
			},
			message: []string{`text "Hello!"`},
		},
		{
			name: "reasoning_content then text",
			stream: events(
				delta(`{"role":"assistant","content":null,"reasoning_content":"Let me "}`),
				delta(`{"reasoning_content":"think."}`),
				delta(`{"content":"Four."}`),
				finish("stop"),
				doneLine,
			),
			events:  []string{`thinking "Let me "`, `thinking "think."`, `text "Four."`, `stop end_turn`},
			message: []string{`thinking "Let me think."`, `text "Four."`},
		},
		{
			name: "reasoning then text",
			stream: events(
				delta(`{"role":"assistant","content":"","reasoning":"Hm"}`),
				delta(`{"reasoning":"m."}`),
				delta(`{"content":"Four.","reasoning":null}`),
				finish("stop"),
				doneLine,
			),
			events:  []string{`thinking "Hm"`, `thinking "m."`, `text "Four."`, `stop end_turn`},
			message: []string{`thinking "Hmm."`, `text "Four."`},
		},
		{
			name: "reasoning under both names is read once",
			stream: events(
				delta(`{"reasoning_content":"once","reasoning":"once"}`),
				delta(`{"content":"ok"}`),
				finish("stop"),
				doneLine,
			),
			events:  []string{`thinking "once"`, `text "ok"`, `stop end_turn`},
			message: []string{`thinking "once"`, `text "ok"`},
		},
		{
			name: "one tool call, arguments in pieces",
			stream: events(
				delta(roleChunk),
				delta(`{"content":"I'll look."}`),
				delta(`{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"Read","arguments":""}}]}`),
				delta(`{"tool_calls":[{"index":0,"function":{"arguments":"{\"file_"}}]}`),
				delta(`{"tool_calls":[{"index":0,"function":{"arguments":"path\":\"/x"}}]}`),
				delta(`{"tool_calls":[{"index":0,"function":{"arguments":"\"}"}}]}`),
				finish("tool_calls"),
				doneLine,
			),
			events: []string{
				`text "I'll look."`,
				`tool call_abc Read`,
				`args call_abc "{\"file_"`,
				`args call_abc "path\":\"/x"`,
				`args call_abc "\"}"`,
				`done call_abc`,
				`stop tool_use`,
			},
			message: []string{`text "I'll look."`, `tool_use call_abc Read {"file_path":"/x"}`},
		},
		{
			name: "parallel tool calls, their arguments interleaved",
			stream: events(
				delta(`{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"Read","arguments":""}}]}`),
				delta(`{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"Glob","arguments":""}}]}`),
				delta(`{"tool_calls":[{"index":0,"function":{"arguments":"{\"file_path\":"}}]}`),
				delta(`{"tool_calls":[{"index":1,"function":{"arguments":"{\"pattern\":"}}]}`),
				delta(`{"tool_calls":[{"index":2,"id":"call_c","type":"function","function":{"name":"Bash","arguments":"{\"command\":\"ls\"}"}},{"index":0,"function":{"arguments":"\"/a\"}"}}]}`),
				delta(`{"tool_calls":[{"index":1,"function":{"arguments":"\"*.go\"}"}}]}`),
				finish("tool_calls"),
				doneLine,
			),
			events: []string{
				`tool call_a Read`,
				`tool call_b Glob`,
				`args call_a "{\"file_path\":"`,
				`args call_b "{\"pattern\":"`,
				`tool call_c Bash`,
				`args call_c "{\"command\":\"ls\"}"`,
				`args call_a "\"/a\"}"`,
				`args call_b "\"*.go\"}"`,
				`done call_a`, `done call_b`, `done call_c`,
				`stop tool_use`,
			},
			message: []string{
				`tool_use call_a Read {"file_path":"/a"}`,
				`tool_use call_b Glob {"pattern":"*.go"}`,
				`tool_use call_c Bash {"command":"ls"}`,
			},
		},
		{
			name: "a call sent whole, and arguments sent as an object",
			stream: events(
				delta(`{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"/a\"}"}}]}`),
				delta(`{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"Glob","arguments":{"pattern":"*.go"}}}]}`),
				finish("tool_calls"),
				doneLine,
			),
			events: []string{
				`tool call_1 Read`, `args call_1 "{\"file_path\":\"/a\"}"`,
				`tool call_2 Glob`, `args call_2 "{\"pattern\":\"*.go\"}"`,
				`done call_1`, `done call_2`,
				`stop tool_use`,
			},
			message: []string{`tool_use call_1 Read {"file_path":"/a"}`, `tool_use call_2 Glob {"pattern":"*.go"}`},
		},
		{
			name: "empty and missing ids get made-up ones",
			stream: events(
				delta(`{"tool_calls":[{"index":0,"id":"","type":"function","function":{"name":"Read","arguments":"{}"}}]}`),
				delta(`{"tool_calls":[{"index":1,"type":"function","function":{"name":"Glob","arguments":"{}"}}]}`),
				delta(`{"tool_calls":[{"index":2,"id":null,"function":{"name":"Bash","arguments":"{}"}}]}`),
				finish("tool_calls"),
				doneLine,
			),
			events: []string{
				`tool made_1 Read`, `args made_1 "{}"`,
				`tool made_2 Glob`, `args made_2 "{}"`,
				`tool made_3 Bash`, `args made_3 "{}"`,
				`done made_1`, `done made_2`, `done made_3`,
				`stop tool_use`,
			},
			message: []string{`tool_use made_1 Read {}`, `tool_use made_2 Glob {}`, `tool_use made_3 Bash {}`},
		},
		{
			name: "one id used twice is made unique",
			stream: events(
				delta(`{"tool_calls":[{"index":0,"id":"same","function":{"name":"Read","arguments":"{\"a\":1}"}}]}`),
				delta(`{"tool_calls":[{"index":1,"id":"same","function":{"name":"Read","arguments":"{\"a\":2}"}}]}`),
				finish("tool_calls"),
				doneLine,
			),
			events: []string{
				`tool same Read`, `args same "{\"a\":1}"`,
				`tool made_1 Read`, `args made_1 "{\"a\":2}"`,
				`done same`, `done made_1`,
				`stop tool_use`,
			},
			message: []string{`tool_use same Read {"a":1}`, `tool_use made_1 Read {"a":2}`},
		},
		{
			name: "every call sent whole under index 0",
			stream: events(
				delta(`{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"Read","arguments":"{\"a\":1}"}}]}`),
				delta(`{"tool_calls":[{"index":0,"id":"call_2","function":{"name":"Read","arguments":"{\"a\":2}"}}]}`),
				delta(`{"tool_calls":[{"index":0,"id":"","function":{"name":"Glob","arguments":"{\"a\":3}"}}]}`),
				finish("stop"),
				doneLine,
			),
			events: []string{
				`tool call_1 Read`, `args call_1 "{\"a\":1}"`,
				`tool call_2 Read`, `args call_2 "{\"a\":2}"`,
				`tool made_1 Glob`, `args made_1 "{\"a\":3}"`,
				`done call_1`, `done call_2`, `done made_1`,
				`stop tool_use`,
			},
			message: []string{`tool_use call_1 Read {"a":1}`, `tool_use call_2 Read {"a":2}`, `tool_use made_1 Glob {"a":3}`},
		},
		{
			name: "id and name repeated on every piece",
			stream: events(
				delta(`{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"Read","arguments":"{\"a\":"}}]}`),
				delta(`{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"Read","arguments":"1}"}}]}`),
				delta(`{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"Read","arguments":""}}]}`),
				finish("tool_calls"),
				doneLine,
			),
			events: []string{
				`tool call_1 Read`, `args call_1 "{\"a\":"`, `args call_1 "1}"`,
				`done call_1`,
				`stop tool_use`,
			},
			message: []string{`tool_use call_1 Read {"a":1}`},
		},
		{
			name: "no index: calls told apart by id, then by name",
			stream: events(
				delta(`{"tool_calls":[{"id":"call_1","function":{"name":"Read","arguments":"{\"a\":"}}]}`),
				delta(`{"tool_calls":[{"id":"call_1","function":{"arguments":"1}"}}]}`),
				delta(`{"tool_calls":[{"function":{"name":"Glob","arguments":"{\"b\":"}}]}`),
				delta(`{"tool_calls":[{"function":{"arguments":"2}"}}]}`),
				finish("tool_calls"),
				doneLine,
			),
			events: []string{
				`tool call_1 Read`, `args call_1 "{\"a\":"`, `args call_1 "1}"`,
				`tool made_1 Glob`, `args made_1 "{\"b\":"`, `args made_1 "2}"`,
				`done call_1`, `done made_1`,
				`stop tool_use`,
			},
			message: []string{`tool_use call_1 Read {"a":1}`, `tool_use made_1 Glob {"b":2}`},
		},
		{
			name: "the name comes after the first arguments",
			stream: events(
				delta(`{"tool_calls":[{"index":0,"id":"call_1","function":{"arguments":"{\"a\":"}}]}`),
				delta(`{"tool_calls":[{"index":0,"function":{"name":"Read","arguments":"1}"}}]}`),
				finish("tool_calls"),
				doneLine,
			),
			events: []string{
				`tool call_1 Read`, `args call_1 "{\"a\":"`, `args call_1 "1}"`,
				`done call_1`,
				`stop tool_use`,
			},
			message: []string{`tool_use call_1 Read {"a":1}`},
		},
		{
			name: "a call that never gets a name is still handed on",
			stream: events(
				delta(`{"tool_calls":[{"index":0,"id":"call_1","function":{"arguments":"{\"a\":1}"}}]}`),
				finish("tool_calls"),
				doneLine,
			),
			events: []string{
				`tool call_1 `, `args call_1 "{\"a\":1}"`,
				`done call_1`,
				`stop tool_use`,
			},
			message: []string{`tool_use call_1  {"a":1}`},
		},
		{
			name: "the old function_call form, and a call without its wrapper",
			stream: events(
				delta(`{"function_call":{"name":"Read","arguments":"{\"a\":"}}`),
				delta(`{"function_call":{"arguments":"1}"}}`),
				delta(`{"tool_calls":[{"index":1,"id":"call_2","name":"Glob","arguments":"{\"b\":2}"}]}`),
				finish("function_call"),
				doneLine,
			),
			events: []string{
				`tool made_1 Read`, `args made_1 "{\"a\":"`, `args made_1 "1}"`,
				`tool call_2 Glob`, `args call_2 "{\"b\":2}"`,
				`done made_1`, `done call_2`,
				`stop tool_use`,
			},
			message: []string{`tool_use made_1 Read {"a":1}`, `tool_use call_2 Glob {"b":2}`},
		},
		{
			name: "usage: the cached part is taken out of the input",
			stream: events(
				delta(`{"content":"ok"}`),
				finish("stop"),
				usageChunk(`{"prompt_tokens":1000,"completion_tokens":20,"total_tokens":1020,"prompt_tokens_details":{"cached_tokens":800},"completion_tokens_details":{"reasoning_tokens":12}}`),
				doneLine,
			),
			events:  []string{`text "ok"`, `usage in=200 out=20 read=800 write=0`, `stop end_turn`},
			message: []string{`text "ok"`},
		},
		{
			name: "usage: tokens written to the cache",
			stream: events(
				delta(`{"content":"ok"}`),
				finish("stop"),
				usageChunk(`{"prompt_tokens":1000,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":300,"cache_write_tokens":500}}`),
				doneLine,
			),
			events:  []string{`text "ok"`, `usage in=200 out=20 read=300 write=500`, `stop end_turn`},
			message: []string{`text "ok"`},
		},
		{
			name: "usage: in the chunk that gives the finish reason, with odd numbers",
			stream: events(
				delta(`{"content":"ok"}`),
				chunk(`"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10.0,"completion_tokens":"2","prompt_cache_hit_tokens":4}`),
				doneLine,
			),
			events:  []string{`text "ok"`, `usage in=6 out=2 read=4 write=0`, `stop end_turn`},
			message: []string{`text "ok"`},
		},
		{
			name: "usage: null on every chunk but the last, as the format says",
			stream: events(
				chunk(`"choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}],"usage":null`),
				chunk(`"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":null`),
				usageChunk(`{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}`),
				doneLine,
			),
			events:  []string{`text "ok"`, `usage in=5 out=1 read=0 write=0`, `stop end_turn`},
			message: []string{`text "ok"`},
		},
		{
			name: "the usage chunk repeats the finish reason (OpenRouter)",
			stream: events(
				delta(`{"content":"ok"}`),
				chunk(`"choices":[{"index":0,"delta":{"content":"","role":"assistant"},"finish_reason":"stop","native_finish_reason":"stop"}]`),
				chunk(`"choices":[{"index":0,"delta":{"content":"","role":"assistant"},"finish_reason":"stop","native_finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1}`),
				doneLine,
			),
			events:  []string{`text "ok"`, `usage in=5 out=1 read=0 write=0`, `stop end_turn`},
			message: []string{`text "ok"`},
		},
		{
			name: "an empty usage object on every chunk replaces nothing",
			stream: events(
				chunk(`"choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}],"usage":{"prompt_tokens":5,"completion_tokens":1}`),
				chunk(`"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{}`),
				doneLine,
			),
			events:  []string{`text "ok"`, `usage in=5 out=1 read=0 write=0`, `stop end_turn`},
			message: []string{`text "ok"`},
		},
		{
			name: "ids, names and indexes of the wrong type are still read",
			stream: events(
				delta(`{"tool_calls":[{"index":"0","id":7,"type":"function","function":{"name":"Read","arguments":"{}"}}]}`),
				delta(`{"tool_calls":[{"index":1.0,"id":true,"function":{"name":null,"arguments":"{\"a\":1}"}}]}`),
				delta(`{"tool_calls":[{"index":1,"id":{"x":1},"function":{"name":"Glob"}}]}`),
				finish("tool_calls"),
				doneLine,
			),
			events: []string{
				`tool 7 Read`, `args 7 "{}"`,
				`tool true Glob`, `args true "{\"a\":1}"`,
				`done 7`, `done true`,
				`stop tool_use`,
			},
			message: []string{`tool_use 7 Read {}`, `tool_use true Glob {"a":1}`},
		},
		{
			name: "an empty answer is an answer",
			stream: events(
				delta(roleChunk),
				finish("stop"),
				usageChunk(`{"prompt_tokens":5,"completion_tokens":0}`),
				doneLine,
			),
			events: []string{`usage in=5 out=0 read=0 write=0`, `stop end_turn`},
		},
		{
			name: "the stream closes after the finish reason, with no [DONE]",
			stream: events(
				delta(`{"content":"ok"}`),
				finish("stop"),
			),
			events:  []string{`text "ok"`, `stop end_turn`},
			message: []string{`text "ok"`},
		},
		{
			name: "[DONE] with no finish reason: the reason is read off the answer",
			stream: events(
				delta(`{"content":"ok"}`),
				doneLine,
			),
			events:  []string{`text "ok"`, `stop end_turn`},
			message: []string{`text "ok"`},
		},
		{
			name: "[DONE] with no finish reason, after a tool call",
			stream: events(
				delta(`{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"Read","arguments":"{}"}}]}`),
				doneLine,
			),
			events:  []string{`tool call_1 Read`, `args call_1 "{}"`, `done call_1`, `stop tool_use`},
			message: []string{`tool_use call_1 Read {}`},
		},
		{
			name: "comments, junk, CRLF, odd events and other choices are passed over",
			stream: ": OPENROUTER PROCESSING\r\n\r\n" +
				"event: keep-alive\r\n\r\n" +
				"data: ping\r\n\r\n" +
				"retry: 3000\r\n" +
				delta(`{"content":"o"}`) + "\r\n\r\n" +
				"<html>junk</html>\n\n" +
				chunk(`"choices":[{"index":1,"delta":{"content":"NOT THIS"},"finish_reason":"length"}]`) + "\n\n" +
				chunk(`"choices":[{"index":0,"delta":{"content":"k","unknown_field":{"x":1}},"finish_reason":null,"logprobs":null}],"obfuscation":"aB3","system_fingerprint":"fp"`) + "\r\r" +
				chunk(`"choices":[{"index":0,"delta":{"content":null,"refusal":null,"tool_calls":null},"finish_reason":null}]`) + "\n\n" +
				finish("stop") + "\n\n" +
				"data: [DONE]",
			events:  []string{`text "o"`, `text "k"`, `stop end_turn`},
			message: []string{`text "ok"`},
		},
		{
			name: "content sent as a list of parts",
			stream: events(
				delta(`{"content":[{"type":"text","text":"a"},{"type":"image_url","image_url":{}},{"type":"text","text":"b"}]}`),
				finish("stop"),
				doneLine,
			),
			events:  []string{`text "ab"`, `stop end_turn`},
			message: []string{`text "ab"`},
		},
		{
			name: "a refusal is shown as what the model said",
			stream: events(
				delta(`{"refusal":"I can't help with that."}`),
				finish("stop"),
				doneLine,
			),
			events:  []string{`text "I can't help with that."`, `stop end_turn`},
			message: []string{`text "I can't help with that."`},
		},
		{
			name: "the whole answer in one message, from a server that does not stream",
			stream: events(
				chunk(`"choices":[{"index":0,"message":{"role":"assistant","content":"ok","tool_calls":[{"id":"call_1","type":"function","function":{"name":"Read","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2}`),
				doneLine,
			),
			events:  []string{`text "ok"`, `tool call_1 Read`, `args call_1 "{}"`, `done call_1`, `usage in=3 out=2 read=0 write=0`, `stop tool_use`},
			message: []string{`text "ok"`, `tool_use call_1 Read {}`},
		},
		{
			name: "think tags are left alone unless asked for",
			stream: events(
				delta(`{"content":"<think>hm</think>ok"}`),
				finish("stop"),
				doneLine,
			),
			events:  []string{`text "<think>hm</think>ok"`, `stop end_turn`},
			message: []string{`text "<think>hm</think>ok"`},
		},
		{
			name:   "think tags are split when asked for",
			change: func(c *Config) { c.ThinkTags = "on" },
			stream: events(
				delta(`{"content":"<think>hm</think>ok"}`),
				finish("stop"),
				doneLine,
			),
			events:  []string{`thinking "hm"`, `text "ok"`, `stop end_turn`},
			message: []string{`thinking "hm"`, `text "ok"`},
		},
	})
}

func TestOpenAIStopReasons(t *testing.T) {
	call := delta(`{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"Read","arguments":"{}"}}]}`)
	text := delta(`{"content":"x"}`)
	tests := []struct {
		reason string
		body   string
		want   core.StopReason
	}{
		{"stop", text, core.StopEndTurn},
		{"length", text, core.StopMaxTokens},
		{"content_filter", text, core.StopRefusal},
		{"tool_calls", call, core.StopToolUse},
		{"function_call", call, core.StopToolUse},
		{"tool", call, core.StopToolUse},       // llama.cpp's older builds
		{"stop", call, core.StopToolUse},       // servers that say "stop" after tool calls
		{"tool_calls", text, core.StopEndTurn}, // and "tool_calls" after none
		{"length", call, core.StopMaxTokens},   // cut off in the middle of a call
		{"content_filter", call, core.StopRefusal},
		{"stop_sequence", text, core.StopSequence},
		{"eos", text, core.StopEndTurn},
		{"STOP", text, core.StopEndTurn},
		{"LENGTH", text, core.StopMaxTokens},
		{"Tool_Calls", call, core.StopToolUse},
		{"something_new", text, core.StopEndTurn},
		{"something_new", call, core.StopToolUse},
	}
	for _, tt := range tests {
		f := newFake(t, playing(events(tt.body, finish(tt.reason), doneLine)))
		res := ask(f.provider(TypeOpenAI, nil))
		wantOK(t, res)
		if res.stop != tt.want {
			t.Errorf("finish_reason %q: stop = %s, want %s", tt.reason, res.stop, tt.want)
		}
	}
}

func TestOpenAIErrorsBeforeTheStream(t *testing.T) {
	tests := []struct {
		name   string
		status int
		header map[string]string
		body   string
		kind   core.ErrorKind
		retry  time.Duration
	}{
		{"bad key", 401, nil, `{"error":{"message":"Incorrect API key provided.","type":"invalid_request_error","param":null,"code":"invalid_api_key"}}`, core.ErrAuth, 0},
		{"forbidden", 403, nil, `{"error":{"message":"Country not supported","type":"request_forbidden"}}`, core.ErrAuth, 0},
		{"rate limit", 429, map[string]string{"Retry-After": "7"}, `{"error":{"message":"Rate limit reached","type":"requests","code":"rate_limit_exceeded"}}`, core.ErrRateLimit, 7 * time.Second},
		{"rate limit in milliseconds", 429, map[string]string{"Retry-After-Ms": "250"}, `{"error":{"message":"Rate limit reached","type":"requests","code":"rate_limit_exceeded"}}`, core.ErrRateLimit, 250 * time.Millisecond},
		{"context too long", 400, nil, `{"error":{"message":"This model's maximum context length is 128000 tokens.","type":"invalid_request_error","code":"context_length_exceeded"}}`, core.ErrContextTooLong, 0},
		{"body too large", 413, nil, `too big`, core.ErrContextTooLong, 0},
		{"bad request", 400, nil, `{"error":{"message":"bad","type":"invalid_request_error"}}`, core.ErrInvalidRequest, 0},
		{"not found", 404, nil, `{"error":{"message":"no such model","type":"invalid_request_error","code":"model_not_found"}}`, core.ErrInvalidRequest, 0},
		{"server error", 500, nil, `{"error":{"message":"oops","type":"server_error"}}`, core.ErrServer, 0},
		{"bad gateway page", 502, map[string]string{"Content-Type": "text/html"}, `<html><head><title>502 Bad Gateway</title></head></html>`, core.ErrServer, 0},
		{"overloaded", 503, nil, `{"error":{"message":"The engine is currently overloaded","type":"server_error"}}`, core.ErrOverloaded, 0},
		{"overloaded, 529", 529, nil, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`, core.ErrOverloaded, 0},
		{"redirect", 307, map[string]string{"Location": "/elsewhere"}, ``, core.ErrInvalidRequest, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(t, answering(tt.status, tt.header, tt.body))
			res := ask(f.provider(TypeOpenAI, nil))
			e := wantFailure(t, res.err, tt.kind)
			if e.Status != tt.status || e.RetryAfter != tt.retry || e.Body != tt.body {
				t.Errorf("status %d, retry after %s, body %q", e.Status, e.RetryAfter, e.Body)
			}
			if len(res.events) != 0 {
				t.Errorf("events before the error: %q", res.events)
			}
			if got := len(f.requests()); got != 1 {
				t.Errorf("%d requests were sent; a provider does not retry", got)
			}
			f.waitClosed()
		})
	}
}

func TestOpenAIErrorsInTheStream(t *testing.T) {
	tests := []struct {
		name   string
		stream string
		kind   core.ErrorKind
		status int
		events []string
	}{
		{
			"an error chunk after some of the answer",
			events(delta(`{"content":"half an ans"}`), `data: {"error":{"message":"The server had an error","type":"server_error","code":null}}`),
			core.ErrServer, 0, []string{`text "half an ans"`},
		},
		{
			"an error as the only event",
			events(`data: {"error":{"code":429,"message":"Rate limit exceeded","metadata":{"error_type":"rate_limit_exceeded"}}}`),
			core.ErrRateLimit, 429, nil,
		},
		{
			"OpenRouter's error chunk",
			events(delta(`{"content":"x"}`), chunk(`"provider":"p","error":{"code":502,"message":"Provider disconnected unexpectedly"},"choices":[{"index":0,"delta":{"content":""},"finish_reason":"error"}]`)),
			core.ErrServer, 502, []string{`text "x"`},
		},
		{
			"a finish reason of error with nothing else",
			events(delta(`{"content":"x"}`), finish("error")),
			core.ErrServer, 0, []string{`text "x"`},
		},
		{
			"llama.cpp runs out of context",
			events(`data: {"error":{"code":400,"message":"the request exceeds the available context size, try increasing it","type":"exceed_context_size_error","n_prompt_tokens":9000,"n_ctx":8192}}`),
			core.ErrContextTooLong, 400, nil,
		},
		{
			"an error event by name",
			events(delta(`{"content":"x"}`), "event: error\ndata: {\"message\":\"overloaded, try later\"}"),
			core.ErrOverloaded, 0, []string{`text "x"`},
		},
		{
			"an error sent bare",
			events(delta(`{"content":"x"}`), `data: {"object":"error","message":"This model's maximum context length is 4096 tokens.","type":"BadRequestError","code":400}`),
			core.ErrContextTooLong, 400, []string{`text "x"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(t, playing(tt.stream, hang{}))
			res := ask(f.provider(TypeOpenAI, nil))
			e := wantFailure(t, res.err, tt.kind)
			if e.Status != tt.status {
				t.Errorf("status = %d, want %d", e.Status, tt.status)
			}
			if e.Body == "" {
				t.Errorf("the error keeps none of what the server sent")
			}
			wantLines(t, "events", res.events, tt.events)
			if res.stops != 0 {
				t.Errorf("a stream that failed sent a stop event")
			}
			f.waitClosed()
		})
	}
}

func TestOpenAIStreamCutShort(t *testing.T) {
	tests := []struct {
		name   string
		steps  []any
		kind   core.ErrorKind
		events []string
	}{
		{"the stream ends with no reason and no [DONE]", []any{events(delta(`{"content":"half"}`))}, core.ErrBadResponse, []string{`text "half"`}},
		{"[DONE] and nothing else", []any{events(doneLine)}, core.ErrBadResponse, nil},
		{"[DONE] after a chunk that says nothing", []any{events(delta(roleChunk), doneLine)}, core.ErrBadResponse, nil},
		{"no events at all", []any{""}, core.ErrBadResponse, nil},
		{"only comments", []any{": hi\n\n: there\n\n"}, core.ErrBadResponse, nil},
		{"a chunk that is not JSON", []any{events(delta(`{"content":"a"}`), `data: {"choices":[{"delta":{"content":"b`)}, core.ErrBadResponse, []string{`text "a"`}},
		{"a chunk of the wrong shape", []any{events(`data: {"choices":"none"}`)}, core.ErrBadResponse, nil},
		{"the connection breaks", []any{events(delta(`{"content":"half"}`)), cut{}}, core.ErrNetwork, []string{`text "half"`}},
		{"the connection breaks in the middle of an event", []any{events(delta(`{"content":"one"}`)) + `data: {"choices":[{"index":0,"delta":{"content":"tw`, cut{}}, core.ErrNetwork, []string{`text "one"`}},
		{"the connection breaks before anything", []any{cut{}}, core.ErrNetwork, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(t, playing(tt.steps...))
			res := ask(f.provider(TypeOpenAI, nil))
			e := wantFailure(t, res.err, tt.kind)
			if !e.Retryable() {
				t.Errorf("a stream cut short can be asked for again")
			}
			wantLines(t, "events", res.events, tt.events)
			if res.stops != 0 {
				t.Errorf("a stream that failed sent a stop event")
			}
			f.waitClosed()
		})
	}
}

func TestOpenAINotAStream(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		kind   core.ErrorKind
		holds  string
		status int
	}{
		{"an error with status 200", `{"error":{"message":"The model is overloaded","type":"overloaded_error"}}`, core.ErrOverloaded, "overloaded", 0},
		{"an error with status 200 that names a status", `{"error":{"code":429,"message":"slow down"}}`, core.ErrRateLimit, "slow down", 429},
		{"a whole answer, not streamed", `{"id":"c","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`, core.ErrBadResponse, "one JSON document", 0},
		{"a web page", `<html><body>Welcome to nginx</body></html>`, core.ErrBadResponse, "not a stream of events", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(t, answering(200, nil, tt.body))
			res := ask(f.provider(TypeOpenAI, nil))
			e := wantFailure(t, res.err, tt.kind)
			if !strings.Contains(e.Message, tt.holds) || e.Status != tt.status || e.Body != tt.body {
				t.Errorf("message %q, status %d, body %q", e.Message, e.Status, e.Body)
			}
			f.waitClosed()
		})
	}
}

// fullRequest is a conversation with every kind of block in it.
func fullRequest() *core.Request {
	temp, topP := 0.5, 0.9
	return &core.Request{
		Model:  "gpt-test",
		System: []string{"You are gnosh.", "", "The project is a compiler."},
		Messages: []core.Message{
			{Role: core.RoleUser, Blocks: []core.Block{core.Text("Read main.go"), core.Text("and <b>fix</b> it.")}},
			{Role: core.RoleAssistant, Blocks: []core.Block{
				{Type: core.BlockThinking, Text: "I should read it.", Signature: "sig-1"},
				core.Text("I'll read it."),
				core.ToolUse("call_1", "Read", json.RawMessage(`{ "file_path": "main.go" }`)),
				core.ToolUse("call_2", "Glob", nil),
			}},
			{Role: core.RoleUser, Blocks: []core.Block{
				core.ToolResultBlock("call_1", core.TextResult("package main")),
				core.ToolResultBlock("call_2", core.ErrorResult("no matches")),
				core.Text("Go on."),
			}},
			{Role: core.RoleAssistant, Blocks: []core.Block{core.Text("Done.")}},
			{Role: core.RoleUser, Blocks: []core.Block{core.Text("Thanks.")}},
		},
		Tools: []core.ToolSpec{
			{Name: "Read", Description: "Reads a file.", InputSchema: json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}`)},
			{Name: "Glob"},
		},
		MaxTokens:   1000,
		Temperature: &temp,
		TopP:        &topP,
		Stop:        []string{"END"},
		Thinking:    core.Thinking{Mode: "on", BudgetTokens: 2000, Effort: "high"},
	}
}

const okStream = `data: {"choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}

data: [DONE]

`

func TestOpenAIRequest(t *testing.T) {
	f := newFake(t, playing(okStream))
	p := f.provider(TypeOpenAI, func(c *Config) { c.APIKey = "sk-secret" })
	wantOK(t, drain(p.Stream(context.Background(), fullRequest())))

	got := f.last()
	if got.Method != "POST" || got.Path != "/v1/chat/completions" {
		t.Errorf("sent %s %s", got.Method, got.Path)
	}
	wantJSON(t, "request body", got.Body, `{
		"model": "gpt-test",
		"stream": true,
		"stream_options": {"include_usage": true},
		"max_completion_tokens": 1000,
		"temperature": 0.5,
		"top_p": 0.9,
		"stop": ["END"],
		"reasoning_effort": "high",
		"messages": [
			{"role": "system", "content": "You are gnosh.\n\nThe project is a compiler."},
			{"role": "user", "content": "Read main.go\n\nand <b>fix</b> it."},
			{"role": "assistant", "content": "I'll read it.", "tool_calls": [
				{"id": "call_1", "type": "function", "function": {"name": "Read", "arguments": "{\"file_path\":\"main.go\"}"}},
				{"id": "call_2", "type": "function", "function": {"name": "Glob", "arguments": "{}"}}
			]},
			{"role": "tool", "tool_call_id": "call_1", "content": "package main"},
			{"role": "tool", "tool_call_id": "call_2", "content": "no matches"},
			{"role": "user", "content": "Go on."},
			{"role": "assistant", "content": "Done."},
			{"role": "user", "content": "Thanks."}
		],
		"tools": [
			{"type": "function", "function": {"name": "Read", "description": "Reads a file.", "parameters": {"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}},
			{"type": "function", "function": {"name": "Glob", "parameters": {"type":"object","properties":{}}}}
		]
	}`)
	if escaped := "\\" + "u003c"; strings.Contains(string(got.Body), escaped) {
		t.Errorf("angle brackets are escaped in the body: %s", got.Body)
	}

	wantHeader := map[string]string{
		"Authorization": "Bearer sk-secret",
		"User-Agent":    "gnosh",
		"Content-Type":  "application/json",
		"Accept":        "text/event-stream",
	}
	for k, v := range wantHeader {
		if got.Header.Get(k) != v {
			t.Errorf("header %s = %q, want %q", k, got.Header.Get(k), v)
		}
	}
	for _, k := range []string{"X-Api-Key", "Anthropic-Version", "Accept-Encoding", "Cookie"} {
		if got.Header.Get(k) != "" {
			t.Errorf("header %s = %q, want none", k, got.Header.Get(k))
		}
	}
	f.waitClosed()
}

func TestOpenAIRequestThinkingKept(t *testing.T) {
	f := newFake(t, playing(okStream))
	p := f.provider(TypeOpenAI, func(c *Config) { c.HistoryThinking = "keep"; c.MaxTokensField = "max_tokens" })
	req := &core.Request{
		Model: "m",
		Messages: []core.Message{
			core.UserText("hi"),
			{Role: core.RoleAssistant, Blocks: []core.Block{
				{Type: core.BlockThinking, Text: "first thought"},
				{Type: core.BlockThinking, Opaque: "hidden"},
				{Type: core.BlockThinking, Text: "second thought", Signature: "s"},
				core.Text("Hello."),
			}},
			core.UserText("more"),
		},
		MaxTokens: 50,
	}
	wantOK(t, drain(p.Stream(context.Background(), req)))
	wantJSON(t, "request body", f.last().Body, `{
		"model": "m", "stream": true, "stream_options": {"include_usage": true}, "max_tokens": 50,
		"messages": [
			{"role": "user", "content": "hi"},
			{"role": "assistant", "content": "Hello.", "reasoning_content": "first thought\n\nsecond thought"},
			{"role": "user", "content": "more"}
		]
	}`)
	if h := f.last().Header.Get("Authorization"); h != "" {
		t.Errorf("Authorization = %q with no key configured", h)
	}
}

func TestOpenAIRequestImages(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nfake")
	f := newFake(t, playing(okStream))
	p := f.provider(TypeOpenAI, nil)
	req := &core.Request{
		Model: "m",
		Messages: []core.Message{
			{Role: core.RoleUser, Blocks: []core.Block{
				core.Text("What is this?"),
				{Type: core.BlockImage, MediaType: "image/jpeg", Data: []byte("jpegdata")},
			}},
			{Role: core.RoleAssistant, Blocks: []core.Block{
				core.ToolUse("call_1", "Screenshot", json.RawMessage(`{}`)),
				core.ToolUse("call_2", "Read", json.RawMessage(`{"file_path":"a.png"}`)),
				core.ToolUse("call_3", "Read", json.RawMessage(`{"file_path":"b.txt"}`)),
			}},
			{Role: core.RoleUser, Blocks: []core.Block{
				{Type: core.BlockToolResult, ToolUseID: "call_1", Content: []core.Block{
					core.Text("Took a screenshot."),
					{Type: core.BlockImage, Data: png},
					{Type: core.BlockImage, MediaType: "image/gif", Data: []byte("gif")},
				}},
				{Type: core.BlockToolResult, ToolUseID: "call_2", Content: []core.Block{
					{Type: core.BlockImage, MediaType: "image/png", Data: png},
				}},
				{Type: core.BlockToolResult, ToolUseID: "call_3", Content: []core.Block{core.Text("plain")}},
				core.Text("Which one is bigger?"),
			}},
		},
	}
	wantOK(t, drain(p.Stream(context.Background(), req)))
	wantJSON(t, "request body", f.last().Body, `{
		"model": "m", "stream": true, "stream_options": {"include_usage": true},
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "What is this?"},
				{"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,anBlZ2RhdGE="}}
			]},
			{"role": "assistant", "content": "", "tool_calls": [
				{"id": "call_1", "type": "function", "function": {"name": "Screenshot", "arguments": "{}"}},
				{"id": "call_2", "type": "function", "function": {"name": "Read", "arguments": "{\"file_path\":\"a.png\"}"}},
				{"id": "call_3", "type": "function", "function": {"name": "Read", "arguments": "{\"file_path\":\"b.txt\"}"}}
			]},
			{"role": "tool", "tool_call_id": "call_1", "content": "Took a screenshot.\n\n[This result came with 2 images. They are in the next message.]"},
			{"role": "tool", "tool_call_id": "call_2", "content": "[This result came with 1 image. It is in the next message.]"},
			{"role": "tool", "tool_call_id": "call_3", "content": "plain"},
			{"role": "user", "content": [
				{"type": "text", "text": "Images from the result of tool call call_1 (Screenshot):"},
				{"type": "image_url", "image_url": {"url": "data:image/png;base64,iVBORw0KGgpmYWtl"}},
				{"type": "image_url", "image_url": {"url": "data:image/gif;base64,Z2lm"}},
				{"type": "text", "text": "Image from the result of tool call call_2 (Read):"},
				{"type": "image_url", "image_url": {"url": "data:image/png;base64,iVBORw0KGgpmYWtl"}},
				{"type": "text", "text": "Which one is bigger?"}
			]}
		]
	}`)
}

func TestOpenAIRequestOddConversations(t *testing.T) {
	f := newFake(t, playing(okStream))
	p := f.provider(TypeOpenAI, nil)
	req := &core.Request{
		// No model, no system prompt, and turns that say nothing.
		Messages: []core.Message{
			{Role: core.RoleUser},
			{Role: core.RoleAssistant, Blocks: []core.Block{{Type: core.BlockThinking, Text: "only thought"}}},
			{Role: core.RoleUser, Blocks: []core.Block{core.Text("")}},
			{Role: core.RoleAssistant, Blocks: []core.Block{
				core.Text("a"), core.Text("b"),
				{Type: core.BlockToolUse, ID: "call_1", Name: "Read", Input: json.RawMessage(`not json`)},
				{Type: core.BlockToolUse, ID: "call_2", Name: "Read", Input: json.RawMessage(`[1]`)},
			}},
			{Role: core.RoleUser, Blocks: []core.Block{
				core.Text("before"),
				{Type: core.BlockToolResult, ToolUseID: "call_1"},
				{Type: core.BlockToolResult, ToolUseID: "call_2", IsError: true, Content: []core.Block{core.Text("a"), core.Text("b")}},
				{Type: core.BlockToolResult, ToolUseID: "call_3", IsError: true},
			}},
		},
	}
	wantOK(t, drain(p.Stream(context.Background(), req)))
	wantJSON(t, "request body", f.last().Body, `{
		"stream": true, "stream_options": {"include_usage": true},
		"messages": [
			{"role": "user", "content": ""},
			{"role": "assistant", "content": ""},
			{"role": "user", "content": ""},
			{"role": "assistant", "content": "ab", "tool_calls": [
				{"id": "call_1", "type": "function", "function": {"name": "Read", "arguments": "{}"}},
				{"id": "call_2", "type": "function", "function": {"name": "Read", "arguments": "{}"}}
			]},
			{"role": "tool", "tool_call_id": "call_1", "content": ""},
			{"role": "tool", "tool_call_id": "call_2", "content": "ab"},
			{"role": "tool", "tool_call_id": "call_3", "content": "[The tool call failed and gave no details.]"},
			{"role": "user", "content": "before"}
		]
	}`)
}

func TestOpenAIRequestExtra(t *testing.T) {
	f := newFake(t, playing(okStream))
	p := f.provider(TypeOpenAI, func(c *Config) {
		c.Extra = map[string]any{
			"seed":           7,
			"stream_options": map[string]any{"include_usage": nil, "include_obfuscation": false},
			"provider":       map[string]any{"order": []string{"a", "b"}, "sort": "price"},
			"temperature":    0.1,
			"metadata":       map[string]any{"from": "config"},
		}
	})
	temp := 0.9
	req := &core.Request{
		Model:       "m",
		Messages:    []core.Message{core.UserText("hi")},
		Temperature: &temp,
		MaxTokens:   10,
		Extra: map[string]any{
			"provider":              map[string]any{"sort": "speed", "only": []string{"c"}},
			"max_completion_tokens": nil,
			"max_tokens":            99,
			"metadata":              "replaced",
			"model":                 "other",
		},
	}
	wantOK(t, drain(p.Stream(context.Background(), req)))
	wantJSON(t, "request body", f.last().Body, `{
		"model": "other",
		"stream": true,
		"stream_options": {"include_obfuscation": false},
		"seed": 7,
		"temperature": 0.1,
		"max_tokens": 99,
		"metadata": "replaced",
		"provider": {"order": ["a", "b"], "sort": "speed", "only": ["c"]},
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	// Neither map was changed by being merged.
	if len(req.Extra["provider"].(map[string]any)) != 2 {
		t.Errorf("the request's Extra was changed: %v", req.Extra)
	}
	wantOK(t, drain(p.Stream(context.Background(), &core.Request{Model: "m", Messages: []core.Message{core.UserText("hi")}})))
	wantJSON(t, "second request body", f.last().Body, `{
		"model": "m",
		"stream": true,
		"stream_options": {"include_obfuscation": false},
		"seed": 7,
		"temperature": 0.1,
		"metadata": {"from": "config"},
		"provider": {"order": ["a", "b"], "sort": "price"},
		"messages": [{"role": "user", "content": "hi"}]
	}`)
}

func TestHeaders(t *testing.T) {
	f := newFake(t, playing(okStream))
	p := f.provider(TypeOpenAI, func(c *Config) {
		c.APIKey = "sk-secret"
		c.Headers = map[string]string{
			"authorization":       "Basic abc", // wins over the provider's
			"X-Title":             "gnosh",
			"User-Agent":          "", // removed
			"Accept":              "", // removed
			"HTTP-Referer":        "https://example.com",
			"OpenAI-Organization": "org-1",
		}
	})
	wantOK(t, ask(p))
	h := f.last().Header
	want := map[string]string{
		"Authorization":       "Basic abc",
		"X-Title":             "gnosh",
		"User-Agent":          "",
		"Accept":              "",
		"Http-Referer":        "https://example.com",
		"Openai-Organization": "org-1",
		"Content-Type":        "application/json",
	}
	for k, v := range want {
		if h.Get(k) != v {
			t.Errorf("header %s = %q, want %q", k, h.Get(k), v)
		}
	}
}

func TestOpenAIModels(t *testing.T) {
	f := newFake(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/models" {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[
			{"id":"gpt-plain","object":"model","created":1,"owned_by":"openai"},
			{"id":"or/model","context_length":200000,"top_provider":{"max_completion_tokens":16000}},
			{"id":"groq-model","context_window":131072,"max_completion_tokens":8192},
			{"id":"vllm-model","max_model_len":32768}
		]}`))
	})
	p := f.provider(TypeOpenAI, func(c *Config) { c.APIKey = "sk-1" })
	lister, ok := p.(core.ModelLister)
	if !ok {
		t.Fatal("an openai provider does not list models")
	}
	if _, ok := p.(core.TokenCounter); ok {
		t.Error("an openai provider claims to count tokens; the format has no way to")
	}
	got, err := lister.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []core.ModelInfo{
		{ID: "gpt-plain"},
		{ID: "or/model", ContextWindow: 200000, MaxOutput: 16000},
		{ID: "groq-model", ContextWindow: 131072, MaxOutput: 8192},
		{ID: "vllm-model", ContextWindow: 32768},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("model %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	req := f.last()
	if req.Header.Get("Authorization") != "Bearer sk-1" || req.Header.Get("Accept") != "application/json" || req.Header.Get("Content-Type") != "" || len(req.Body) != 0 {
		t.Errorf("request headers %v, body %q", req.Header, req.Body)
	}
	f.waitClosed()

	// Failures are sorted like any other.
	f2 := newFake(t, answering(401, nil, `{"error":{"message":"bad key","code":"invalid_api_key"}}`))
	_, err = f2.provider(TypeOpenAI, nil).(core.ModelLister).Models(context.Background())
	wantFailure(t, err, core.ErrAuth)
	f3 := newFake(t, answering(200, nil, `not json`))
	_, err = f3.provider(TypeOpenAI, nil).(core.ModelLister).Models(context.Background())
	wantFailure(t, err, core.ErrBadResponse)
}
