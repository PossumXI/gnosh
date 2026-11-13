//go:build race

package transcript

// raceEnabled says the tests were built with the race detector, under
// which everything runs several times slower: the tests that load or
// build something large make it smaller then.
const raceEnabled = true
