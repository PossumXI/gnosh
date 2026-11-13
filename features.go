package main

import (
	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/memjoin"
)

// features lists the parts of gnosh that are joined to the wiring, in the
// order they are joined in. It is the one place a feature is registered:
// a part that brings tools, prompt lines, turn-end checks, slash commands
// or subcommands of its own (the GnoVM machines and mem, the peers, the
// swarm) is written against app.Feature in a package of its own and added
// to this list, and nothing in internal/app changes for it.
//
// The order matters where two features speak at the same moment: see the
// ordering table at the top of internal/app/feature.go.
func features() []app.Feature {
	return []app.Feature{
		memjoin.New(memjoin.Options{}), // the GnoVM machines, mem and frames
	}
}
