package compose

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// The CLI-side pin and the image-build pin must agree, or generated compose
// files would reference an image tag the workflows never published. Ports
// cli/test/compat/mitmproxy_version.bats.
func TestMitmproxyVersionMatchesImageEnv(t *testing.T) {
	f, err := os.Open("../../images/mitmproxy.env")
	if err != nil {
		t.Skip("images/mitmproxy.env not present")
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if v, ok := strings.CutPrefix(line, "MITMPROXY_VERSION="); ok {
			if v != MitmproxyVersion {
				t.Errorf("images/mitmproxy.env pins %q, compose.MitmproxyVersion is %q", v, MitmproxyVersion)
			}
			return
		}
	}
	t.Error("MITMPROXY_VERSION not found in images/mitmproxy.env")
}
