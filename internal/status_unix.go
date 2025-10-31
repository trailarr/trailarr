//go:build !windows
// +build !windows

package internal

import (
	"strings"
	"syscall"
)

// buildDisks returns disk usage info using syscall.Statfs on Unix-like systems.
func buildDisks(pathSet map[string]bool) []DiskInfo {
	var disks []DiskInfo
	for p := range pathSet {
		if strings.TrimSpace(p) == "" {
			continue
		}
		di := DiskInfo{Location: p}
		var st syscall.Statfs_t
		if err := syscall.Statfs(p, &st); err == nil {
			total := st.Blocks * uint64(st.Bsize)
			free := st.Bavail * uint64(st.Bsize)
			used := total - free
			usedPct := 0
			if total > 0 {
				usedPct = int((used * 100) / total)
			}
			di.Total = total
			di.Free = free
			di.TotalStr = humanBytes(total)
			di.FreeStr = humanBytes(free)
			di.UsedPct = usedPct
		} else {
			di.TotalStr = "N/A"
			di.FreeStr = "N/A"
			di.UsedPct = 0
		}
		disks = append(disks, di)
	}
	return disks
}
