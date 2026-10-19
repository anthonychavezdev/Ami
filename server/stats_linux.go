//go:build linux

package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// hostStats reads memory and load from the kernel: sysinfo for the total and
// the load averages, /proc/meminfo for MemAvailable, which counts the caches
// the kernel would drop for a program that needs the memory.
func hostStats() host {
	var h host
	var si unix.Sysinfo_t
	if err := unix.Sysinfo(&si); err == nil {
		unit := int64(si.Unit)
		if unit == 0 {
			unit = 1
		}
		h.memTotal = int64(si.Totalram) * unit
		h.memAvailable = int64(si.Freeram) * unit
		for i, l := range si.Loads {
			h.load[i] = float64(l) / 65536
		}
	}
	if available, ok := memAvailable(); ok {
		h.memAvailable = available
	}
	return h
}

// memAvailable is MemAvailable from /proc/meminfo, in bytes.
func memAvailable() (int64, bool) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) >= 2 && fields[0] == "MemAvailable:" {
			kb, err := strconv.ParseInt(fields[1], 10, 64)
			return kb * 1024, err == nil
		}
	}
	return 0, false
}
