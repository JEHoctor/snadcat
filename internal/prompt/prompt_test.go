package prompt

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func withIO(t *testing.T, input string) *bytes.Buffer {
	t.Helper()
	var out bytes.Buffer
	oldIn, oldOut := In, Out
	In, Out = strings.NewReader(input), &out
	t.Cleanup(func() { In, Out = oldIn, oldOut })
	return &out
}

func TestOptionSelectsByNumber(t *testing.T) {
	withIO(t, "2\n")
	got, err := Option("Select agent:", []string{"claude", "cursor", "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "cursor" {
		t.Errorf("got %q, want %q", got, "cursor")
	}
}

// select_option treats empty input as "take the first option".
func TestOptionEmptyInputTakesFirst(t *testing.T) {
	withIO(t, "\n")
	got, err := Option("Select agent:", []string{"claude", "cursor"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "claude" {
		t.Errorf("got %q, want %q", got, "claude")
	}
}

func TestOptionRepromptsOnInvalidInput(t *testing.T) {
	out := withIO(t, "9\nzero\n1\n")
	got, err := Option("Select agent:", []string{"claude", "cursor"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "claude" {
		t.Errorf("got %q, want %q", got, "claude")
	}
	if n := strings.Count(out.String(), "Invalid selection, try again."); n != 2 {
		t.Errorf("got %d retry messages, want 2", n)
	}
}

func TestOptionListsWithDefaultMarker(t *testing.T) {
	out := withIO(t, "\n")
	if _, err := Option("Select IDE:", []string{"vscode", "jetbrains", "none"}); err != nil {
		t.Fatal(err)
	}
	want := "Select IDE: [vscode]\n  1) vscode\n  2) jetbrains\n  3) none\n> "
	if got := out.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMultipleParsesCommaSeparatedIndices(t *testing.T) {
	withIO(t, "1, 3\n")
	got, err := Multiple("Pick:", []string{"a", "b", "c"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Empty input returns the defaults, not an empty selection.
func TestMultipleEmptyInputReturnsDefaults(t *testing.T) {
	withIO(t, "\n")
	defaults := []string{"b"}
	got, err := Multiple("Pick:", []string{"a", "b"}, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, defaults) {
		t.Errorf("got %v, want %v", got, defaults)
	}
}

func TestMultipleRepromptsOnInvalidIndex(t *testing.T) {
	out := withIO(t, "1,7\n2\n")
	got, err := Multiple("Pick:", []string{"a", "b"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if !strings.Contains(out.String(), "Invalid selection: 7") {
		t.Errorf("missing rejection message, got %q", out.String())
	}
}

func TestMultipleMarksDefaults(t *testing.T) {
	out := withIO(t, "\n")
	if _, err := Multiple("Pick:", []string{"a", "b"}, []string{"b"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "  2) b (default)") {
		t.Errorf("missing default marker, got %q", out.String())
	}
}

func TestLineReturnsTrimmedInput(t *testing.T) {
	withIO(t, "my-project\r\n")
	got, err := Line("Project name:")
	if err != nil {
		t.Fatal(err)
	}
	if got != "my-project" {
		t.Errorf("got %q, want %q", got, "my-project")
	}
}

// A bare EOF (no trailing newline) still yields whatever was typed.
func TestLineHandlesEOFWithoutNewline(t *testing.T) {
	withIO(t, "typed")
	got, err := Line("Project name:")
	if err != nil {
		t.Fatal(err)
	}
	if got != "typed" {
		t.Errorf("got %q, want %q", got, "typed")
	}
}
