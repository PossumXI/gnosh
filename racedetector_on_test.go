//go:build race

package main

// raceDetector says the tests were built with the race detector, so the
// binary they run is built with it too.
const raceDetector = true
