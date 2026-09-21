// Package compose ports cli/lib/composefile.bash — the YAML surgery that turns
// the compose templates into a project's generated compose files.
//
// The bash original shells out to `yq` once per mutation. This package parses
// once into a yaml.Node tree, applies every mutation in memory, and writes
// once. Node-level editing is required rather than incidental: the generated
// files carry comments that users are expected to read and edit, and *disabled*
// optional mounts are rendered as commented-out YAML inside a foot comment so
// they can be switched on by uncommenting. See plans/2026-08-07-go-port.md §3.1.
//
// yq is itself Go and built on yaml.v3, so with the encoder set to 2-space
// indent the output is byte-identical to what the bash pipeline produced.
package compose

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// File is a parsed compose document.
type File struct {
	doc *yaml.Node
}

// Load parses a compose file.
func Load(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &File{doc: &doc}, nil
}

// Bytes renders the document.
//
// The 2-space indent is what makes the output match yq's; the default of 4
// would reformat every generated file.
func (f *File) Bytes() ([]byte, error) {
	var sb strings.Builder
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(f.doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte(stripBlankBeforeIndented(sb.String())), nil
}

// stripBlankBeforeIndented removes a blank line that is immediately followed
// by an indented line.
//
// The YAML encoder emits a blank line between a node's foot comment and the
// next sibling, which shows up in the generated file as a gap in the middle of
// the volumes list — right where a disabled mount was rendered as a comment.
// The bash pipeline scrubs it with
//
//	sed '/^$/{ N; /^\n[[:space:]]/{ s/^\n//; }; }'
//
// and this reproduces that rule exactly, including its pairwise consumption:
// sed's N pulls in the next line and the cycle then restarts past both.
func stripBlankBeforeIndented(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if lines[i] != "" || i+1 >= len(lines) {
			out = append(out, lines[i])
			continue
		}
		next := lines[i+1]
		if next != "" && (next[0] == ' ' || next[0] == '\t') {
			out = append(out, next) // drop the blank line
		} else {
			out = append(out, lines[i], next)
		}
		i++ // both lines are consumed, as sed's N does
	}
	return strings.Join(out, "\n")
}

// Save writes the document back to path.
func (f *File) Save(path string) error {
	b, err := f.Bytes()
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// root returns the document's top-level mapping.
func (f *File) root() *yaml.Node {
	if f.doc.Kind == yaml.DocumentNode && len(f.doc.Content) > 0 {
		return f.doc.Content[0]
	}
	return f.doc
}

// get walks a mapping path, returning nil when any element is missing.
func get(n *yaml.Node, path ...string) *yaml.Node {
	cur := n
	for _, key := range path {
		if cur == nil || cur.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(cur.Content); i += 2 {
			if cur.Content[i].Value == key {
				next = cur.Content[i+1]
				break
			}
		}
		cur = next
	}
	return cur
}

// ensure walks a mapping path, creating empty mappings for missing elements.
func ensure(n *yaml.Node, path ...string) *yaml.Node {
	cur := n
	for _, key := range path {
		next := get(cur, key)
		if next == nil {
			next = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			cur.Content = append(cur.Content, scalar(key), next)
		}
		cur = next
	}
	return cur
}

// ensureSeq returns the sequence at path, creating it when absent.
func ensureSeq(n *yaml.Node, path ...string) *yaml.Node {
	if len(path) == 0 {
		return nil
	}
	parent := ensure(n, path[:len(path)-1]...)
	key := path[len(path)-1]
	seq := get(parent, key)
	if seq == nil {
		seq = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		parent.Content = append(parent.Content, scalar(key), seq)
	}
	return seq
}

func scalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

// setScalar sets a scalar value at path, replacing any existing value.
func (f *File) setScalar(value string, path ...string) {
	parent := ensure(f.root(), path[:len(path)-1]...)
	key := path[len(path)-1]
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			parent.Content[i+1].Kind = yaml.ScalarNode
			parent.Content[i+1].Tag = "!!str"
			parent.Content[i+1].Value = value
			parent.Content[i+1].Content = nil
			return
		}
	}
	parent.Content = append(parent.Content, scalar(key), scalar(value))
}

// delete removes a key from the mapping at path.
func (f *File) delete(path ...string) {
	parent := get(f.root(), path[:len(path)-1]...)
	if parent == nil || parent.Kind != yaml.MappingNode {
		return
	}
	key := path[len(path)-1]
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			parent.Content = append(parent.Content[:i], parent.Content[i+2:]...)
			return
		}
	}
}

// appendFootComment adds text to the last item of a sequence, accumulating
// onto any comment already there with a newline separator.
//
// This is how disabled optional mounts are rendered: the commented-out YAML
// rides along as a foot comment on the last *active* entry, so the user can
// uncomment it later. Mirrors add_foot_comment's
// `(existing // "") + "\n" + comment | sub("^\n", "")`.
func appendFootComment(seq *yaml.Node, text string) error {
	if seq == nil || len(seq.Content) == 0 {
		return fmt.Errorf("cannot add foot comment to an empty sequence")
	}
	last := seq.Content[len(seq.Content)-1]
	if last.FootComment == "" {
		last.FootComment = text
		return nil
	}
	last.FootComment += "\n" + text
	return nil
}
