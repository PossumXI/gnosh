package app

import (
	"net/http/httptest"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// goneURL is the address of a model server that is not there.
func goneURL() string {
	s := httptest.NewServer(nil)
	s.Close()
	return s.URL
}

// A run whose model's server is not there goes on with the fallback model
// the configuration names, and says so.
func TestFallbackModelTakesOverARun(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "from the fallback"})
	w.Settings(`{"providers": {"gone": {"type": "llamacpp", "baseUrl": "` + goneURL() + `"}}, "model": "gone/big", "fallbackModel": ["local/test-model"]}`)
	r := run(t, w, "-p", "hello").wantCode(t, 0)
	if r.out != "from the fallback\n" {
		t.Errorf("stdout = %q", r.out)
	}
	has(t, "stderr", r.err, "gone (big)", "could not be reached", "This turn goes on with the fallback model local (test-model)",
		"the next turn asks gone (big)")
}

// --fallback-model names them for one run, a bare name as -m takes it.
func TestFallbackModelFlag(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "flagged"})
	w.Settings(`{"providers": {"gone": {"type": "llamacpp", "baseUrl": "` + goneURL() + `"}}, "model": "gone/big"}`)
	r := run(t, w, "-p", "--fallback-model", "local/test-model", "hello").wantCode(t, 0)
	if r.out != "flagged\n" {
		t.Errorf("stdout = %q", r.out)
	}
	has(t, "stderr", r.err, "This turn goes on with the fallback model local (test-model)")

	// Without one, the run waits for the model and then fails, as before.
	w = apptest.NewWorld(t)
	w.Settings(`{"providers": {"gone": {"type": "llamacpp", "baseUrl": "` + goneURL() + `"}}, "model": "gone/big"}`)
	r = run(t, w, "-p", "hello")
	if r.code == 0 {
		t.Fatalf("a run with its model gone and no fallback ended well: %q", r.out)
	}
	has(t, "stderr", r.err, "is not running")
}
