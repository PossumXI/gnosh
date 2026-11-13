//go:build race

package main

// raceEnabled says the tests were built with the race detector, so the
// binary they run is built with it too.
const raceEnabled = true
