//go:build windows
// +build windows

package internal

import "strings"

// buildDisks on Windows returns a best-effort list with N/A for sizes.
// syscall.Statfs is not available on Windows in the syscall package, and
// implementing an accurate Windows disk usage requires calling Windows APIs.
// For cross-compiling and server scenarios, return N/A values so the
// status endpoint remains usable.
func buildDisks(pathSet map[string]bool) []DiskInfo {
	var disks []DiskInfo
	for p := range pathSet {
		if strings.TrimSpace(p) == "" {
			continue
		}
		di := DiskInfo{
			Location: p,
			TotalStr: "N/A",
			FreeStr:  "N/A",
			UsedPct:  0,
		}
		disks = append(disks, di)
	}
	return disks
}
