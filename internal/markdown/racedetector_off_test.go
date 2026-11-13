//go:build !race

package markdown

// raceDetector says that the tests run under the race detector, which
// makes them about ten times slower: the long ones do less then.
const raceDetector = false
