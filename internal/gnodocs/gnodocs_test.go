package gnodocs

import (
	"os"
	"strings"
	"testing"
)

// Write puts both guides in the folder, whole, and puts back one that was
// changed there.
func TestWrite(t *testing.T) {
	dir := t.TempDir()
	paths, err := Write(dir)
	if err != nil || len(paths) != 2 {
		t.Fatalf("Write = %v, %v", paths, err)
	}
	for i, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil || len(data) < 10000 || !strings.HasPrefix(string(data), "# ") {
			t.Errorf("%s: %d bytes, %v", Names[i], len(data), err)
		}
	}
	if err := os.WriteFile(paths[1], []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(paths[1]); !strings.Contains(string(data), "Security") {
		t.Errorf("a changed guide was not put back")
	}
}
