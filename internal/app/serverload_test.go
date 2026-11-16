package app

import (
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// What the person is told of a model server's slots while a request of
// about 12k tokens waits.
func TestLoadText(t *testing.T) {
	slot := func(busy bool, prompt, read int) core.SlotLoad {
		return core.SlotLoad{Busy: busy, Prompt: prompt, Read: read}
	}
	for _, c := range []struct {
		slots []core.SlotLoad
		want  string
	}{
		{[]core.SlotLoad{slot(false, 0, 0)}, ""},
		{[]core.SlotLoad{slot(true, 12400, 8100)}, "reading the prompt: 8.1k of 12.4k tokens"},
		{[]core.SlotLoad{slot(true, 12400, 8100), slot(true, 30000, 1000), slot(false, 0, 0)}, "reading the prompt: 8.1k of 12.4k tokens, beside 1 other request"},
		{[]core.SlotLoad{slot(true, 40000, 3000)}, "the server is busy with another request; yours waits"},
		{[]core.SlotLoad{slot(true, 40000, 3000), slot(true, 900, 900), slot(true, 50000, 0)}, "the server's 3 slots are all busy with other requests; yours waits"},
		{[]core.SlotLoad{slot(true, 40000, 3000), slot(false, 0, 0), slot(false, 0, 0)}, "the server is also answering 1 other request"},
		{[]core.SlotLoad{slot(true, 40000, 3000), slot(true, 900, 900), slot(false, 0, 0)}, "the server is also answering 2 other requests"},
	} {
		if got := loadText(core.ServerLoad{Slots: c.slots}, 12000); got != c.want {
			t.Errorf("loadText(%+v) = %q, want %q", c.slots, got, c.want)
		}
	}
	for n, want := range map[int]string{950: "950", 8100: "8.1k", 12000: "12k", 131072: "131.1k"} {
		if got := kTokens(n); got != want {
			t.Errorf("kTokens(%d) = %q, want %q", n, got, want)
		}
	}
}
