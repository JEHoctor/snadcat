package log

import (
	"bytes"
	"testing"
	"time"
)

func fixedClock(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	oldOut, oldNow, oldColor := Out, now, color
	Out = &buf
	now = func() time.Time { return time.Date(2026, 8, 7, 9, 5, 3, 0, time.UTC) }
	color = false
	t.Cleanup(func() { Out, now, color = oldOut, oldNow, oldColor })
	return &buf
}

func TestLevelsMatchBashFormat(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string, ...any)
		want string
	}{
		{"info", Info, "09:05:03 [INFO] hello\n"},
		{"warn", Warn, "09:05:03 [WARN] hello\n"},
		{"error", Error, "09:05:03 [ERROR] hello\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := fixedClock(t)
			tc.fn("hello")
			if got := buf.String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// logging.bash reads its input line by line, so a multi-line message becomes
// several fully-prefixed lines rather than one prefixed line plus raw text.
func TestMultilineMessageIsPrefixedPerLine(t *testing.T) {
	buf := fixedClock(t)
	Info("first\nsecond")
	want := "09:05:03 [INFO] first\n09:05:03 [INFO] second\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A trailing newline in the message must not produce a blank trailing entry.
func TestTrailingNewlineDoesNotEmitBlankLine(t *testing.T) {
	buf := fixedClock(t)
	Info("only\n")
	want := "09:05:03 [INFO] only\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatArgsAreApplied(t *testing.T) {
	buf := fixedClock(t)
	Info("%s %d", "n", 2)
	want := "09:05:03 [INFO] n 2\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
