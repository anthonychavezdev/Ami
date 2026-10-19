//go:build darwin

package main

import (
	"encoding/binary"

	"golang.org/x/sys/unix"
)

// hostStats reads memory and load through sysctl. macOS keeps most memory
// busy with caches on purpose, so "available" is what its memory-pressure
// accounting calls the available level, a percentage of the total; failing
// that, the free and speculative pages, what it would hand a program at once.
func hostStats() host {
	var h host
	if total, err := unix.SysctlUint64("hw.memsize"); err == nil {
		h.memTotal = int64(total)
	}
	if level, err := unix.SysctlUint32("kern.memorystatus_level"); err == nil && level <= 100 {
		h.memAvailable = h.memTotal * int64(level) / 100
	} else {
		page, _ := unix.SysctlUint64("hw.pagesize")
		free, _ := unix.SysctlUint32("vm.page_free_count")
		speculative, _ := unix.SysctlUint32("vm.page_speculative_count")
		h.memAvailable = (int64(free) + int64(speculative)) * int64(page)
	}
	// struct loadavg: three fixed-point uint32 averages, padding, then the int64 scale.
	if raw, err := unix.SysctlRaw("vm.loadavg"); err == nil && len(raw) >= 24 {
		if scale := float64(binary.NativeEndian.Uint64(raw[16:24])); scale > 0 {
			for i := range h.load {
				h.load[i] = float64(binary.NativeEndian.Uint32(raw[4*i:])) / scale
			}
		}
	}
	return h
}
