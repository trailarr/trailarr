package internal

import (
	"path/filepath"
	"runtime"
)

// FfmpegCmd is the binary name for ffmpeg
const FfmpegCmd = "ffmpeg"

// FfmpegPath is the full path to the ffmpeg binary managed by Trailarr.
// It defaults to `TrailarrRoot/bin/ffmpeg` (or `ffmpeg.exe` on Windows).
var FfmpegPath string

// UpdateFfmpegPath recomputes FfmpegPath based on the current TrailarrRoot.
func UpdateFfmpegPath() {
	exe := FfmpegCmd
	if runtime.GOOS == "windows" {
		exe = exe + ".exe"
	}
	FfmpegPath = filepath.Join(TrailarrRoot, "bin", exe)
}

func init() {
	UpdateFfmpegPath()
}
