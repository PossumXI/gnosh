package llm

import "net/http"

// TraceKind says what a TraceRecord holds.
type TraceKind string

const (
	// TraceRequest is a request about to be sent: Method, URL and Body.
	TraceRequest TraceKind = "request"
	// TraceResponse is a response's Status and Header. Body is set as well
	// when the response was read whole: an error, a model list, a token
	// count.
	TraceResponse TraceKind = "response"
	// TraceEvent is one server-sent event of a stream: its Event name and
	// its Data.
	TraceEvent TraceKind = "event"
	// TraceIgnored is a line of a stream that is not part of any event the
	// provider reads: a comment, a keep-alive, junk. Data holds the line.
	TraceIgnored TraceKind = "ignored"
	// TraceTimings is the speed report llama.cpp ends a stream with, in
	// Timings.
	TraceTimings TraceKind = "timings"
)

// TraceRecord is one thing that went over the wire, as Config.Trace is told
// it. Which fields are set depends on Kind.
type TraceRecord struct {
	Kind TraceKind

	// Method and URL say where a request goes. The URL is given without
	// its query and without any user name or password.
	Method string
	URL    string

	// Status and Header are a response's. The provider keeps no reference
	// to Header.
	Status int
	Header http.Header

	// Body is the JSON of a request, or the body of a response that was
	// read whole.
	Body string

	// Event and Data are a server-sent event's name and data.
	Event string
	Data  string

	// Timings is llama.cpp's speed report.
	Timings *Timings
}

// Timings is what llama.cpp's server reports about a finished answer. The
// whole context it used is PromptN + CacheN + PredictedN tokens.
type Timings struct {
	// CacheN is how many prompt tokens were reused from the server's
	// cache.
	CacheN int
	// PromptN is how many prompt tokens the server had to read.
	PromptN int
	// PromptMS is how long reading them took, in milliseconds.
	PromptMS float64
	// PromptPerSecond is the reading speed in tokens per second.
	PromptPerSecond float64
	// PredictedN is how many tokens the model wrote.
	PredictedN int
	// PredictedMS is how long writing them took, in milliseconds.
	PredictedMS float64
	// PredictedPerSecond is the writing speed in tokens per second.
	PredictedPerSecond float64
}
