package internal

import "testing"

func TestIsImpersonationErrorNative(t *testing.T) {
	cases := []struct {
		out  string
		want bool
	}{
		{"Impersonate target not found", true},
		{"missing dependencies required to support this target", true},
		{"all good", false},
		// Regression: a format-selection failure must NOT be treated as an
		// impersonation failure (it caused a pointless retry and a misleading log).
		{"ERROR: [youtube] mnuxv8Xzq3g: Requested format is not available. Use --list-formats for a list of available formats", false},
	}
	for _, c := range cases {
		if got := isImpersonationErrorNative(c.out); got != c.want {
			t.Fatalf("isImpersonationErrorNative(%q) = %v, want %v", c.out, got, c.want)
		}
	}
}
