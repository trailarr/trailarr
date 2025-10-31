package internal

import (
	"os"
	"sync"
)

var (
	regMutex    sync.Mutex
	regTempDirs []string
)

// RegisterTempDir records a temp directory so test harness can remove it
// once the full test run completes. It's safe to call from multiple goroutines.
func RegisterTempDir(path string) {
	if path == "" {
		return
	}
	regMutex.Lock()
	regTempDirs = append(regTempDirs, path)
	regMutex.Unlock()
}

// RemoveRegisteredTempDirs attempts to remove all registered temp dirs.
// It ignores errors to avoid masking test results, but best-effort cleans them up.
func RemoveRegisteredTempDirs() {
	regMutex.Lock()
	dirs := make([]string, len(regTempDirs))
	copy(dirs, regTempDirs)
	regTempDirs = nil
	regMutex.Unlock()

	for _, d := range dirs {
		_ = os.RemoveAll(d)
	}
}
