package compose

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// ProjectName returns the top-level `name`, or "" when unset.
func (f *File) ProjectName() string {
	n := get(f.root(), "name")
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

// ExternalCacheVolumes returns the names of top-level volumes declared
// `external: true` whose name starts with sandcat-cache-. These are the shared
// dependency caches `sandcat run` has to create before `compose up`, and
// deliberately only those — other external volumes are the user's business.
//
// Mirrors `.volumes | to_entries[] | select(.value.external == true) |
// .value.name // .key`.
func (f *File) ExternalCacheVolumes() []string {
	volumes := get(f.root(), "volumes")
	if volumes == nil || volumes.Kind != yaml.MappingNode {
		return nil
	}
	var out []string
	for i := 0; i+1 < len(volumes.Content); i += 2 {
		key, decl := volumes.Content[i], volumes.Content[i+1]
		ext := get(decl, "external")
		if ext == nil || ext.Value != "true" {
			continue
		}
		name := key.Value
		if n := get(decl, "name"); n != nil && n.Value != "" {
			name = n.Value
		}
		if strings.HasPrefix(name, "snadcat-cache-") {
			out = append(out, name)
		}
	}
	return out
}
