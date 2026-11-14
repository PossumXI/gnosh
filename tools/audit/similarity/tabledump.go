//go:build ignore

// Tabledump prints config.Table(): every key, provider type, hook event and
// environment variable of gnosh's configuration, as the program itself lists
// them, one per line with where the builder says the name comes from. The
// similarity audit reads the same tables from the source text
// (configsurface.py) and uses this to check that it read them all.
//
// It imports an internal package, so it runs only from inside the module:
// run.sh copies go.mod and internal/config into a scratch folder, puts this
// file beside them and runs it there. It is no part of the program.
package main

import (
	"fmt"

	"github.com/gnosh-ai/gnosh/internal/config"
)

func main() {
	for _, e := range config.Table() {
		fmt.Printf("%s\t%s\t%s\n", e.Name, e.From, e.Feature)
	}
}
