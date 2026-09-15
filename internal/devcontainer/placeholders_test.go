package devcontainer

import "testing"

func TestApplyLinePlaceholdersReplacesWholeLine(t *testing.T) {
	in := "a\n# __X__ trailing text\nb\n"
	got := ApplyLinePlaceholders(in, Pair{"__X__", "line1\nline2"})
	want := "a\nline1\nline2\nb\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// An empty replacement removes the line rather than leaving a blank one; an
// agent with no VS Code settings must not leave a gap in devcontainer.json.
func TestApplyLinePlaceholdersEmptyDropsLine(t *testing.T) {
	got := ApplyLinePlaceholders("a\n__X__\nb\n", Pair{"__X__", ""})
	if want := "a\nb\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyLinePlaceholdersFirstTokenWins(t *testing.T) {
	got := ApplyLinePlaceholders("__A__ __B__\n", Pair{"__A__", "A"}, Pair{"__B__", "B"})
	if want := "A\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// `while read` + `printf '%s\n'` always terminates the last line.
func TestApplyLinePlaceholdersTerminatesLastLine(t *testing.T) {
	if got := ApplyLinePlaceholders("no newline"); got != "no newline\n" {
		t.Errorf("got %q", got)
	}
	if got := ApplyLinePlaceholders(""); got != "" {
		t.Errorf("empty input should stay empty, got %q", got)
	}
}

func TestApplyInlinePlaceholdersReplacesAllOccurrences(t *testing.T) {
	got := ApplyInlinePlaceholders("x __T__ y __T__\n", Pair{"__T__", "v"})
	if want := "x v y v\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyIDECustomizations(t *testing.T) {
	in := "before\n// __CUSTOMIZATIONS_START__\n\t\"customizations\": {}\n// __CUSTOMIZATIONS_END__\nafter\n"
	tests := []struct{ ide, want string }{
		{"vscode", "before\n\t\"customizations\": {}\nafter\n"},
		{"none", "before\nafter\n"},
		{"jetbrains", "before\n" + jetbrainsCustomizations + "after\n"},
	}
	for _, tc := range tests {
		t.Run(tc.ide, func(t *testing.T) {
			if got := ApplyIDECustomizations(in, tc.ide); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStackExtensionLines(t *testing.T) {
	got := StackExtensionLines([]string{"node", "python", "go"})
	want := "\t\t\t\t\"ms-python.python\",\n\t\t\t\t\"golang.go\","
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := StackExtensionLines([]string{"node"}); got != "" {
		t.Errorf("no-extension stacks should yield empty, got %q", got)
	}
}

func TestCustomizePlugins(t *testing.T) {
	in := "\t\t\t\"plugins\": [],\n"
	got := CustomizePlugins(in, []string{"java", "python", "scala"})
	want := "\t\t\t\"plugins\": [\"PythonCore\", \"org.intellij.scala\"],\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// Stacks with bundled language support leave the empty array alone.
	if got := CustomizePlugins(in, []string{"node", "java"}); got != in {
		t.Errorf("expected no change, got %q", got)
	}
}
