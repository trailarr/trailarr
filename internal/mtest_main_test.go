package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain sets environment needed for tests.
// It ensures embedded store is skipped during the test run; tests that need the external store
// should rely on a running instance or mock the interface.
func TestMain(m *testing.M) {
	// Create a package-level temp dir for tests and set TrailarrRoot/ConfigPath
	// We also set TMPDIR/TEMP/TMP to this directory so that calls to
	// os.MkdirTemp("", ...) and testing.T.TempDir() will create their
	// temp directories under the package temp root. This keeps all test
	// artifacts in a single place and makes cleanup predictable.
	tmp, err := os.MkdirTemp("", "trailarr_test")
	if err != nil {
		os.Exit(1)
	}
	// Ensure parent config dir exists so background writes don't fail
	TrailarrRoot = tmp
	// Ensure subsequent calls to os.MkdirTemp with an empty dir and
	// testing.T.TempDir use our package temp root.
	_ = os.Setenv("TMPDIR", TrailarrRoot)
	_ = os.Setenv("TEMP", TrailarrRoot)
	_ = os.Setenv("TMP", TrailarrRoot)
	ConfigPath = filepath.Join(TrailarrRoot, "config", "config.yml")
	// Redefine derived globals so they point into the test temp root.
	MediaCoverPath = filepath.Join(TrailarrRoot, "MediaCover")
	CookiesFile = filepath.Join(TrailarrRoot, "cookies.txt")
	LogsDir = filepath.Join(TrailarrRoot, "logs")
	_ = os.MkdirAll(filepath.Dir(ConfigPath), 0o755)
	// Ensure directories used by other components exist
	_ = os.MkdirAll(MediaCoverPath, 0o755)
	_ = os.MkdirAll(LogsDir, 0o755)

	// Use the fake runner for yt-dlp to avoid launching external processes in tests
	oldRunner := ytDlpRunner
	ytDlpRunner = &fakeRunner{}

	// Shorten queue-related delays for faster tests
	QueueItemRemoveDelay = 10 * time.Millisecond
	QueuePollInterval = 10 * time.Millisecond
	// Shorten other package-level sleeps so tests run quickly
	DownloadQueueWatcherInterval = 5 * time.Millisecond
	TooManyRequestsPauseDuration = 100 * time.Millisecond
	TooManyRequestsPauseLogInterval = 10 * time.Millisecond
	TasksDepsWaitInterval = 10 * time.Millisecond
	TasksInitialDelay = 10 * time.Millisecond

	// Run tests
	code := m.Run()

	// Restore state
	ytDlpRunner = oldRunner

	// If the environment variable TRAILARR_KEEP_TEST_TMP is set to "1" or
	// "true" we keep the temp directory for post-test inspection. This is
	// useful when debugging failing tests locally. Otherwise remove temp
	// dirs and the package-level tmp dir as usual.
	keep := strings.ToLower(os.Getenv("TRAILARR_KEEP_TEST_TMP"))
	if keep == "1" || keep == "true" {
		fmt.Printf("Keeping test tmp dir for inspection: %s\n", tmp)
	} else {
		// Remove any temp dirs created during tests (eg. yt-dlp temp dirs)
		RemoveRegisteredTempDirs()
		_ = os.RemoveAll(tmp)
	}
	os.Exit(code)
}
