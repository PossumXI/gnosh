//go:build race

package diff

// underRace says that the tests are running under the race detector, which
// makes everything some ten times slower. The package starts no goroutines,
// so there is nothing for the detector to find in a longer run that it would
// not find in a shorter one: the tests that repeat a check thousands of
// times repeat it fewer times.
const underRace = true
