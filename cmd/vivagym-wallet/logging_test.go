package main

import (
	"log/slog"
	"testing"
)

func TestNormalizeJournalField(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"method", "METHOD"},
		{"duration_ms", "DURATION_MS"},
		{"Content-Type", "CONTENT_TYPE"},
		{"trace id", "TRACE_ID"},
		{"_leading", "LEADING"},
		{"a.b/c", "A_B_C"},
		{"message", ""},  // reserved
		{"priority", ""}, // reserved
		{"syslog_identifier", ""},
		{"code_file", ""},
		{"-", ""},
	}
	for _, c := range cases {
		if got := normalizeJournalField(c.in); got != c.want {
			t.Errorf("normalizeJournalField(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeJournalFieldLength(t *testing.T) {
	long := "x"
	for i := 0; i < 80; i++ {
		long += "abcdefgh"
	}
	if got := normalizeJournalField(long); len(got) != 64 {
		t.Errorf("expected 64-char cap, got %d: %q", len(got), got)
	}
}

// TestReplaceAttrNormalizesKeys exercises the ReplaceAttr callback the way the
// handler will, ensuring lowercase slog keys become valid journal fields.
func TestReplaceAttrNormalizesKeys(t *testing.T) {
	rep := func(_ []string, a slog.Attr) slog.Attr {
		a.Key = normalizeJournalField(a.Key)
		return a
	}
	for key, want := range map[string]string{
		"path":        "PATH",
		"trace_id":    "TRACE_ID",
		"duration_ms": "DURATION_MS",
	} {
		a := rep(nil, slog.String(key, "v"))
		if a.Key != want {
			t.Errorf("ReplaceAttr(%q) -> %q, want %q", key, a.Key, want)
		}
	}
}
