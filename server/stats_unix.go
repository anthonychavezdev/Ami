//go:build linux || darwin

package main

import (
	"time"

	"golang.org/x/sys/unix"
)

// diskStats is the size of the volume holding path and what is still free on
// it for ordinary programs.
func diskStats(path string) (total, free int64) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, 0
	}
	return int64(st.Blocks) * int64(st.Bsize), int64(st.Bavail) * int64(st.Bsize)
}

// processCPUTime is the CPU time this process has used so far, user and system.
func processCPUTime() time.Duration {
	var ru unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}
