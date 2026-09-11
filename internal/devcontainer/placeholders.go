// Package devcontainer ports cli/lib/devcontainer.bash and
// cli/libexec/init/devcontainer — copying the template tree into a project and
// expanding its placeholders.
//
// devcontainer.json is JSONC with comment markers, so it is edited line-wise
// on purpose; a JSON parser would strip the comments users are meant to read.
package devcontainer

import (
	"os"
	"strings"
)

// Pair is one placeholder token and its replacement.
type Pair struct {
	Token       string
	Replacement string
}

// lines splits file content the way `while IFS= read -r line || [[ -n "$line" ]]`
// consumes it: a trailing newline does not yield an empty final line, and a
// missing one still yields the last line.
func lines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}

// joinLines is the inverse: every line is terminated, as `printf '%s\n'` does.
func joinLines(ls []string) string {
	if len(ls) == 0 {
		return ""
	}
	return strings.Join(ls, "\n") + "\n"
}

// ApplyLinePlaceholders replaces whole lines. For each line, the first token
// it contains wins and the *entire* line is replaced by that token's
// replacement (which may span several lines). An empty replacement drops the
// line — this is how an agent with no VS Code settings leaves no gap behind.
func ApplyLinePlaceholders(content string, pairs ...Pair) string {
	var out []string
	for _, line := range lines(content) {
		matched := false
		for _, p := range pairs {
			if strings.Contains(line, p.Token) {
				matched = true
				if p.Replacement != "" {
					out = append(out, p.Replacement)
				}
				break
			}
		}
		if !matched {
			out = append(out, line)
		}
	}
	return joinLines(out)
}

// ApplyInlinePlaceholders replaces every occurrence of each token in place.
// Used where the placeholder sits inside a longer line, such as the mitmproxy
// command line.
func ApplyInlinePlaceholders(content string, pairs ...Pair) string {
	var out []string
	for _, line := range lines(content) {
		for _, p := range pairs {
			line = strings.ReplaceAll(line, p.Token, p.Replacement)
		}
		out = append(out, line)
	}
	return joinLines(out)
}

// editFile applies fn to a file's content in place.
func editFile(path string, fn func(string) string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(fn(string(b))), 0o644)
}
