package instructions_test

import (
	"context"
	"fmt"
	"testing/fstest"

	"github.com/gnosh-ai/gnosh/internal/instructions"
)

// The tree and the session of the worked example in the package's
// documentation.
func Example() {
	file := func(text string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(text), Mode: 0o644}
	}
	disk := fstest.MapFS{
		"home/ana/.config/gnosh/GNOSH.md":      file("Be brief.\n"),
		"home/ana/.config/gnosh/rules/tone.md": file("Plain words.\n"),
		"home/ana/notes/go.md":                 file("Table tests.\n"),
		"work/GNOSH.md":                        file("Everything under /work is Go.\n"),
		"work/shop/GNOSH.md":                   file("Build with @docs/build.md. My Go notes: @~/notes/go.md\n"),
		"work/shop/CLAUDE.local.md":            file("My sandbox is on port 8099.\n"),
		"work/shop/docs/build.md":              file("Run make.\n"),
		"work/shop/.gnosh/rules/tests.md":      file("Write the test first.\n"),
		"work/shop/.gnosh/rules/api.md":        file("---\npaths: \"api/**/*.go\"\n---\nHandlers return errors.\n"),
		"work/shop/api/GNOSH.md":               file("Keep handlers thin.\n"),
		"work/shop/api/server.go":              file("package api\n"),
		"work/shop/web/AGENTS.md":              file("Use pnpm.\n"),
		"work/shop/web/app.ts":                 file(""),
	}
	ctx := context.Background()
	set, err := instructions.Load(ctx, instructions.Options{
		Cwd:           "/work/shop",
		Home:          "/home/ana",
		UserFiles:     []string{"/home/ana/.config/gnosh/GNOSH.md", "/home/ana/.config/gnosh/rules"},
		ProjectNames:  []string{"GNOSH.md"},
		LocalNames:    []string{"CLAUDE.local.md"},
		FallbackNames: []string{"AGENTS.md"},
		DotDirs:       []string{".gnosh"},
		Trusted:       true,
		FS:            disk,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	show := func(files []instructions.File) {
		for _, f := range files {
			fmt.Printf("%-8s %-8s %s\n", f.Scope, f.Kind, f.Path)
		}
	}
	fmt.Println("At the start:")
	show(set.Files())
	for _, w := range set.Warnings() {
		fmt.Println("warning:", w)
	}
	for _, touched := range []string{"/work/shop/api/server.go", "/work/shop/api/server.go", "/work/shop/web/app.ts"} {
		late, _ := set.ForPath(ctx, touched)
		fmt.Printf("After %s:\n", touched)
		show(late)
	}
	// Output:
	// At the start:
	// User     main     /home/ana/.config/gnosh/GNOSH.md
	// User     rule     /home/ana/.config/gnosh/rules/tone.md
	// Project  main     /work/GNOSH.md
	// Project  main     /work/shop/GNOSH.md
	// Project  import   /work/shop/docs/build.md
	// Project  rule     /work/shop/.gnosh/rules/tests.md
	// Local    main     /work/shop/CLAUDE.local.md
	// warning: /work/shop/GNOSH.md imports /home/ana/notes/go.md, which is outside the project; that was not approved, so it was not loaded.
	// After /work/shop/api/server.go:
	// Project  main     /work/shop/api/GNOSH.md
	// Project  rule     /work/shop/.gnosh/rules/api.md
	// After /work/shop/api/server.go:
	// After /work/shop/web/app.ts:
}
