package compose

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jehoctor/snadcat/internal/jsonfile"
	"github.com/jehoctor/snadcat/internal/project"
)

// ReadUpstreamCABundles returns the merged `upstream_ca_bundles` list from
// the user settings and the project's settings.local.json, in that order.
//
// A missing file or a file that does not parse contributes nothing: the bash
// discards yq's stderr here on purpose so a corrupted settings file reads as
// "not configured" rather than aborting init.
func ReadUpstreamCABundles(userSettings, projectDir string) []string {
	var out []string
	for _, f := range []string{userSettings, filepath.Join(projectDir, project.Dir, "settings.local.json")} {
		obj, err := jsonfile.Load(f)
		if err != nil {
			continue
		}
		v, _ := obj.Get("upstream_ca_bundles")
		arr, _ := v.([]any)
		for _, e := range arr {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// ValidateUpstreamCABundle checks that path is absolute, readable, and holds
// at least one PEM certificate block.
func ValidateUpstreamCABundle(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("upstream_ca_bundles: expected absolute path, got '%s'", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("upstream_ca_bundles: file not found: %s", path)
		}
		return fmt.Errorf("upstream_ca_bundles: file not readable: %s", path)
	}
	if !bytes.Contains(b, []byte("-----BEGIN CERTIFICATE-----")) {
		return fmt.Errorf("upstream_ca_bundles: no PEM certificate block in %s", path)
	}
	return nil
}

// caInstall is prepended to the mitmproxy entrypoint. Two separate installs
// are needed: mitmproxy loads its trust store from certifi, not the OS store,
// so update-ca-certificates alone would not make it trust the CAs on the
// upstream leg.
const caInstall = `cp /upstream-ca/*.crt /usr/local/share/ca-certificates/ && update-ca-certificates >/dev/null && cat /upstream-ca/*.crt >> "$(python3 -c 'import certifi; print(certifi.where())')"`

// ApplyUpstreamCABundles bind-mounts the given CA bundles read-only into the
// mitmproxy service and prepends their installation to its entrypoint.
//
// Every bundle is validated first; on any failure the document is left
// untouched. The install is joined with `|| exit 1; ` rather than `&&`
// because the template's entrypoint ends in `… & exec docker-entrypoint.sh`
// and `&` binds looser than `&&` — an `&&` join would put the install inside
// the backgrounded list and race mitmproxy's start. The explicit exit keeps a
// broken mount fail-loud instead of starting with unpatched trust.
func (f *File) ApplyUpstreamCABundles(bundles []string) error {
	if len(bundles) == 0 {
		return nil
	}
	for _, b := range bundles {
		if err := ValidateUpstreamCABundle(b); err != nil {
			return err
		}
	}

	entrypoint := get(f.root(), "services", "mitmproxy", "entrypoint")
	if entrypoint == nil || entrypoint.Kind != yaml.SequenceNode || len(entrypoint.Content) < 3 {
		return fmt.Errorf("upstream_ca_bundles: mitmproxy entrypoint not found")
	}
	existing := entrypoint.Content[2].Value
	if existing == "" {
		return fmt.Errorf("upstream_ca_bundles: mitmproxy entrypoint not found")
	}

	volumes := ensureSeq(f.root(), "services", "mitmproxy", "volumes")
	for i, b := range bundles {
		base := strings.TrimSuffix(filepath.Base(b), filepath.Ext(b))
		volumes.Content = append(volumes.Content,
			scalar(fmt.Sprintf("%s:/upstream-ca/%03d-%s.crt:ro", b, i, base)))
	}

	// Replace the whole array as `.entrypoint = [...]` does; a yq array literal
	// is flow style, which is also how the template writes it.
	mitmproxy := ensure(f.root(), "services", "mitmproxy")
	fresh := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle, Content: []*yaml.Node{
		scalar("/bin/sh"), scalar("-c"), scalar(caInstall + " || exit 1; " + existing), scalar("sh"),
	}}
	for i := 0; i+1 < len(mitmproxy.Content); i += 2 {
		if mitmproxy.Content[i].Value == "entrypoint" {
			mitmproxy.Content[i+1] = fresh
			break
		}
	}
	return nil
}
