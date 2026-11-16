package app

import (
	"context"
	"slices"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// The /config picker's choices are the options /config lists, with what
// each takes: verbose is true or false.
func TestConfigChoices(t *testing.T) {
	w := apptest.NewWorld(t)
	var got []ConfigChoice
	_, r := attach(t, w, nil, func(ctx context.Context, c SessionControls, f *scripted) int {
		got = c.ConfigChoices()
		return 0
	})
	if r.code != 0 {
		t.Fatalf("exit code %d", r.code)
	}
	i := slices.IndexFunc(got, func(ch ConfigChoice) bool { return ch.Key == "verbose" })
	if i < 0 {
		t.Fatalf("no verbose among %d options", len(got))
	}
	if v := got[i]; v.Value != "false" || !slices.Equal(v.Choices, []string{"true", "false"}) || v.Origin == "" || v.What == "" {
		t.Errorf("verbose: %+v", v)
	}
}
