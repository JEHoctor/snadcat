package stacks

import (
	"reflect"
	"testing"
)

func TestNamesMatchBashOrder(t *testing.T) {
	want := []string{"node", "python", "java", "rust", "go", "scala", "ruby", "dotnet", "zig"}
	if got := Names(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// scala depends on java, and dependencies must come first so devbox installs
// the JDK before the Scala tooling that assumes it.
func TestResolveExpandsDepsFirst(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"no deps", []string{"python"}, []string{"python"}},
		{"scala pulls java in front", []string{"scala"}, []string{"java", "scala"}},
		{"explicit java is not duplicated", []string{"java", "scala"}, []string{"java", "scala"}},
		{"java after scala keeps first position", []string{"scala", "java"}, []string{"java", "scala"}},
		{"repeats collapse", []string{"go", "go"}, []string{"go"}},
		{"order otherwise preserved", []string{"rust", "node"}, []string{"rust", "node"}},
		{"empty", nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Resolve(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	if err := Validate([]string{"node", "scala"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	err := Validate([]string{"node", "cobol"})
	if err == nil {
		t.Fatal("expected an error for an unknown stack")
	}
	want := "Invalid stack: cobol (expected: node,python,java,rust,go,scala,ruby,dotnet,zig)"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err, want)
	}
}

// Only the JVM stacks contribute shared caches today; everything else must
// contribute none, or projects would get mounts for caches that never fill.
func TestSharedCaches(t *testing.T) {
	if got := SharedCaches([]string{"node", "python", "go"}); got != nil {
		t.Errorf("non-JVM stacks contributed caches: %v", got)
	}

	got := SharedCaches([]string{"java"})
	if len(got) != 6 {
		t.Fatalf("got %d java caches, want 6", len(got))
	}
	if got[0].String() != "sandcat-cache-maven:/home/vscode/.m2/repository" {
		t.Errorf("unexpected first cache: %q", got[0])
	}
}

// scala inherits java's caches through Resolve, and the union must not
// double-count them.
func TestSharedCachesDedupeAcrossResolvedStacks(t *testing.T) {
	got := SharedCaches(Resolve([]string{"scala", "java"}))
	if len(got) != 6 {
		t.Errorf("got %d caches, want 6 (deduped)", len(got))
	}
}

func TestDevboxPackages(t *testing.T) {
	got := DevboxPackages(Resolve([]string{"scala"}))
	want := []string{"temurin-bin-25@latest", "scala@latest", "sbt@latest", "scala-cli@latest"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Regression guard for upstream #89: `nodejs@lts` is not a valid devbox
// specifier and made `--stacks node` unbuildable.
func TestNodeStackHasNoLtsSpecifier(t *testing.T) {
	s, ok := Get("node")
	if !ok {
		t.Fatal("node stack missing")
	}
	if want := []string{"nodejs"}; !reflect.DeepEqual(s.DevboxPackages, want) {
		t.Errorf("got %v, want %v", s.DevboxPackages, want)
	}
}

// node has no extension; the others do. A stack contributing an empty id must
// not emit a stray entry into devcontainer.json's extension list.
func TestExtensionsSkipStacksWithout(t *testing.T) {
	got := Extensions([]string{"node", "python", "go"})
	want := []string{"ms-python.python", "golang.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Only python contributes environment today (uv's TLS opt-in); a stray entry
// from another stack would land in every generated compose file.
func TestEnvEntries(t *testing.T) {
	if got := EnvEntries([]string{"python", "go"}); !reflect.DeepEqual(got, []string{"UV_SYSTEM_CERTS=1"}) {
		t.Errorf("got %v", got)
	}
	if got := EnvEntries([]string{"node", "java", "rust"}); got != nil {
		t.Errorf("unexpected env from non-python stacks: %v", got)
	}
}
