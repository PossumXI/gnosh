package shelltool

import (
	"fmt"
	"os"
	"sync"
)

// output is everything a command has printed: a capture, for the view the
// model is shown, and the spool file, which holds the output itself up to
// a limit. The command's pump writes it; results and TaskOutput read it,
// possibly while the pump is still writing.
type output struct {
	path string // the spool file
	max  int64  // the most the spool file may hold; negative for no limit
	// where is added after the spool file's path wherever the model is
	// told of it: " on this machine", for the output of a command that ran
	// on another one.
	where string

	mu      sync.Mutex
	cap     *capture
	file    *os.File // the spool file, open for writing; nil once closed
	spooled int64    // bytes written to the spool file
	err     error    // why writing the spool file stopped early, if it did
}

// newOutput makes the output of a command that spools to file, which is at
// path and may hold limit bytes. keep is how many bytes of each end of the
// output are held in memory.
func newOutput(path string, file *os.File, limit int64, keep int) *output {
	return &output{path: path, max: limit, cap: newCapture(keep), file: file}
}

// add takes the next piece of output and returns how much there has been
// in all.
func (o *output) add(p []byte) int64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cap.Write(p)
	if o.file != nil && o.err == nil {
		if o.max >= 0 {
			p = p[:min(int64(len(p)), max(o.max-o.spooled, 0))]
		}
		if len(p) > 0 {
			n, err := o.file.Write(p)
			o.spooled += int64(n)
			o.err = err
		}
	}
	return o.cap.total
}

// closeFile closes the spool file once nothing more will be written to it.
func (o *output) closeFile() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.file != nil {
		if err := o.file.Close(); err != nil && o.err == nil {
			o.err = err
		}
		o.file = nil
	}
}

// size returns how much has been printed in all, how much of that is in
// the spool file, and why the spool file stopped taking it, if it did
// before its limit.
func (o *output) size() (total, spooled int64, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cap.total, o.spooled, o.err
}

// view returns the output as the model is shown it, at most limit
// characters of it, and whether anything was left out; and, as of the same
// moment, how much has been printed in all and how much of that is in the
// spool file.
func (o *output) view(limit int) (text string, cut bool, total, spooled int64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	text, cut = o.cap.view(limit, o.gap)
	return text, cut, o.cap.total, o.spooled
}

// end returns the last limit characters of the output.
func (o *output) end(limit int) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cap.end(limit)
}

// gap makes the line that stands for the output left out of a view. It
// must be called with mu held.
func (o *output) gap(lines int64) string {
	return gapLine(lines, o.path+o.where, o.spooled, o.cap.total)
}

// gapLine makes the line that stands for output left out of a view: how
// many lines, and which file holds all of it, or how much of it when the
// file took only the start.
func gapLine(lines int64, path string, spooled, total int64) string {
	what := "output omitted here"
	if lines > 0 {
		what = plural(lines, "line") + " omitted"
	}
	switch {
	case path == "" || spooled == 0:
		return "[" + what + "]"
	case spooled < total:
		return fmt.Sprintf("[%s, the first %s of the output are in %s]", what, byteSize(spooled), path)
	}
	return fmt.Sprintf("[%s, full output in %s]", what, path)
}

// from describes the output from offset off to its end as it stands now:
// how far the spool file reaches, the bytes past the file that are still
// held in memory, how many bytes between the two are gone, and where the
// output ends.
func (o *output) from(off int64) (fileEnd int64, rest []byte, lost int64, total int64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	total = o.cap.total
	fileEnd = o.spooled
	if total > fileEnd {
		rest, lost = o.cap.since(max(off, fileEnd))
	}
	return fileEnd, rest, lost, total
}
