//go:build !linux && !darwin

package main

import "time"

// Other platforms report no host figures; the app shows them as unknown.
func hostStats() host                      { return host{} }
func diskStats(string) (total, free int64) { return 0, 0 }
func processCPUTime() time.Duration        { return 0 }
