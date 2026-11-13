//go:build !linux && !darwin

package shelltool

// readOutput reads the command's output from the pipe and hands each piece
// to take, until the pipe ends or is closed.
func (j *job) readOutput(buf []byte) {
	for {
		n, err := j.pipe.Read(buf)
		if n > 0 {
			j.intake.Lock()
			j.take(buf[:n])
			j.intake.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// pipeState cannot say, on this system, how the pipe stands. Callers then
// go by how long the output takes to end.
func (j *job) pipeState() (waiting bool, taken int64, writers, known bool) {
	return false, 0, false, false
}
