package api

import (
	"strings"
	"testing"
)

func TestSanitizeLogValue(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // "" 表示需要含特定前缀
	}{
		{"empty", "", ""},
		{"plain business key", "F20260618-abc", "F20260618-abc"},
		{"all digits id", "12345678", "12345678"},
		{"multiline card", "CARD-AAA\nCARD-BBB", ""},
		{"long base64 token", "Z3JlZW5fYXBwcm92ZWRfY2FyZF9maWVsZA==", ""},
		{"tight alphanumeric", "ABCD1234EFGH5678", ""},
		{"short word", "hello", "hello"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitizeLogValue(c.in)
			if c.want != "" {
				if got != c.want {
					t.Fatalf("sanitize(%q) = %q, want %q", c.in, got, c.want)
				}
				return
			}
			if c.in == "" {
				if got != "" {
					t.Fatalf("sanitize(\"\") = %q, want empty", got)
				}
				return
			}
			// expect redacted
			if !strings.HasPrefix(got, "[redacted:") {
				t.Fatalf("sanitize(%q) = %q, want [redacted:* prefix", c.in, got)
			}
		})
	}
}
