package internal

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestDownloadAssetToFile_RetriesAndSucceeds(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			// First attempt: return non-OK so the helper will retry
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		// Second attempt: success
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	tmp, err := os.CreateTemp("", "test-download-*")
	if err != nil {
		t.Fatalf("create tmp: %v", err)
	}
	defer func() { tmp.Close(); _ = os.Remove(tmp.Name()) }()

	if err := downloadAssetToFile(srv.URL, tmp, 2*time.Second); err != nil {
		t.Fatalf("download failed: %v", err)
	}
	// read file
	if _, err := tmp.Seek(0, 0); err != nil {
		t.Fatalf("seek tmp: %v", err)
	}
	b, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatalf("read tmp: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("unexpected body: %s", string(b))
	}
}
