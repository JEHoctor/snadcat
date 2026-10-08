// Package stacks ports cli/lib/stacks.bash — the development-stack table that
// `sandcat init --stacks` selects from.
//
// The bash original uses case functions rather than associative arrays purely
// for Bash 3.2 compatibility; that constraint dies with the port, so this is a
// plain table.
package stacks

import (
	"fmt"
	"strings"
)

// CacheVolume is a shared dependency-cache mount: a host-scoped external
// volume name and the container path it covers.
type CacheVolume struct {
	Volume string
	Path   string
}

// String renders the "<volume>:<path>" form used in compose volume entries.
func (c CacheVolume) String() string { return c.Volume + ":" + c.Path }

// Stack is one selectable development stack.
type Stack struct {
	Name string

	// DevboxPackages are "<name>@<version>" specs resolved against nixpkgs.
	// Dependency packages are deliberately absent: Deps handles transitive
	// resolution and each dep contributes its own packages exactly once, which
	// keeps devbox.stack.json free of duplicate entries that would trip
	// version-conflict detection.
	DevboxPackages []string

	// Extension is the VS Code extension id, empty when the stack has none.
	Extension string

	// JetBrainsPlugin is the Marketplace plugin id Gateway installs into the
	// backend IDE, empty when language support is bundled (node, java), the
	// primary JetBrains IDE is a standalone product with no IntelliJ plugin
	// (dotnet → Rider, rust → RustRover), or none exists.
	//
	// go and ruby are gated behind Ultimate; on a Community backend Gateway
	// silently skips them.
	JetBrainsPlugin string

	// Deps are stacks pulled in transitively by this one.
	Deps []string

	// Env are KEY=value entries the stack needs on the agent service.
	Env []string

	// SharedCaches are the dependency caches this stack contributes.
	SharedCaches []CacheVolume
}

// jvmCaches are the six shared JVM dependency caches. Only the safe
// subdirectories are exposed — .m2/settings.xml, .gradle/daemon/, .ivy2/local/
// and friends stay per-project inside agent-home.
var jvmCaches = []CacheVolume{
	{"snadcat-cache-maven", "/home/vscode/.m2/repository"},
	{"snadcat-cache-coursier", "/home/vscode/.cache/coursier"},
	{"snadcat-cache-gradle", "/home/vscode/.gradle/caches"},
	{"snadcat-cache-gradle-wrapper", "/home/vscode/.gradle/wrapper/dists"},
	{"snadcat-cache-ivy", "/home/vscode/.ivy2/cache"},
	{"snadcat-cache-sbt-boot", "/home/vscode/.sbt/boot"},
}

// all is ordered as STACK_NAMES is in stacks.bash; the interactive picker
// presents stacks in this order, so it is part of the user-visible surface.
var all = []Stack{
	{Name: "node", DevboxPackages: []string{"nodejs"}},
	// uv bundles its own root CA store and ignores the system trust store by
	// default, so it fails TLS verification against mitmproxy's intercepting
	// CA. UV_SYSTEM_CERTS (uv >= 0.11.0) fixes that; the older UV_NATIVE_TLS
	// is deprecated since 0.11.9 and prints a warning, so only the new one is
	// set.
	{Name: "python", DevboxPackages: []string{"python@latest"}, Extension: "ms-python.python", Env: []string{"UV_SYSTEM_CERTS=1"}, JetBrainsPlugin: "PythonCore"},
	{Name: "java", DevboxPackages: []string{"temurin-bin-25@latest"}, Extension: "redhat.java", SharedCaches: jvmCaches},
	{Name: "rust", DevboxPackages: []string{"rustc@latest", "cargo@latest"}, Extension: "rust-lang.rust-analyzer"},
	{Name: "go", DevboxPackages: []string{"go@latest"}, Extension: "golang.go", JetBrainsPlugin: "org.jetbrains.plugins.go"},
	{Name: "scala", DevboxPackages: []string{"scala@latest", "sbt@latest", "scala-cli@latest"}, Extension: "scalameta.metals", Deps: []string{"java"}, JetBrainsPlugin: "org.intellij.scala"},
	{Name: "ruby", DevboxPackages: []string{"ruby@latest"}, Extension: "shopify.ruby-lsp", JetBrainsPlugin: "org.jetbrains.plugins.ruby"},
	{Name: "dotnet", DevboxPackages: []string{"dotnet-sdk@latest"}, Extension: "ms-dotnettools.csdevkit"},
	{Name: "zig", DevboxPackages: []string{"zig@latest"}, Extension: "ziglang.vscode-zig", JetBrainsPlugin: "com.falsepattern.zigbrains"},
}

var byName = func() map[string]Stack {
	m := make(map[string]Stack, len(all))
	for _, s := range all {
		m[s.Name] = s
	}
	return m
}()

// Names returns the selectable stack names in presentation order.
func Names() []string {
	out := make([]string, len(all))
	for i, s := range all {
		out[i] = s.Name
	}
	return out
}

// Get looks up a stack by name.
func Get(name string) (Stack, bool) {
	s, ok := byName[name]
	return s, ok
}

// Validate reports the first unknown stack name, mirroring validate_stacks
// including its comma-joined "expected" list.
func Validate(names []string) error {
	for _, n := range names {
		if _, ok := byName[n]; !ok {
			return fmt.Errorf("Invalid stack: %s (expected: %s)", n, strings.Join(Names(), ","))
		}
	}
	return nil
}

// Resolve expands dependencies and deduplicates, emitting dependencies before
// their dependents — so `scala` becomes `java scala`. Order is significant:
// it drives devbox package ordering and shared-cache selection downstream.
//
// Unknown names are passed through rather than rejected; callers that care run
// Validate first, which is what libexec/init/init does.
func Resolve(names []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, n := range names {
		for _, dep := range byName[n].Deps {
			add(dep)
		}
		add(n)
	}
	return out
}

// SharedCaches returns the deduplicated union of caches contributed by the
// given (already resolved) stacks, in first-seen order.
//
// Scala inherits Java's caches through Resolve rather than declaring them, so
// callers must pass the resolved list.
func SharedCaches(resolved []string) []CacheVolume {
	var out []CacheVolume
	seen := map[string]bool{}
	for _, n := range resolved {
		for _, c := range byName[n].SharedCaches {
			if key := c.String(); !seen[key] {
				seen[key] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// DevboxPackages returns the packages contributed by the given resolved
// stacks, in order, without deduplication — matching write_devbox_stack_json,
// which relies on Resolve having already made the list unique.
func DevboxPackages(resolved []string) []string {
	var out []string
	for _, n := range resolved {
		out = append(out, byName[n].DevboxPackages...)
	}
	return out
}

// Extensions returns the non-empty VS Code extension ids for the given stacks,
// in order.
func Extensions(resolved []string) []string {
	var out []string
	for _, n := range resolved {
		if e := byName[n].Extension; e != "" {
			out = append(out, e)
		}
	}
	return out
}

// EnvEntries returns the KEY=value environment entries contributed by the
// given resolved stacks, in order.
func EnvEntries(resolved []string) []string {
	var out []string
	for _, n := range resolved {
		out = append(out, byName[n].Env...)
	}
	return out
}

// JetBrainsPlugins returns the non-empty Marketplace plugin ids for the given
// stacks, in order.
func JetBrainsPlugins(resolved []string) []string {
	var out []string
	for _, n := range resolved {
		if p := byName[n].JetBrainsPlugin; p != "" {
			out = append(out, p)
		}
	}
	return out
}
