package dockercli

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// FormatBytes must match cache.bash's awk: integer bytes under 1 KB, one
// decimal place above. The values appear in `sandcat cache list` output.
func TestFormatBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{13_300_000_000, "12.4 GB"},
		{1 << 50, "1024.0 TB"},
	}
	for _, tc := range tests {
		if got := FormatBytes(tc.in); got != tc.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Cross-check against the original awk when available.
func TestFormatBytesMatchesAwk(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash unavailable")
	}
	if _, err := os.Stat("../../cli/lib/cache.bash"); err != nil {
		t.Skip("bash originals unavailable")
	}
	for _, n := range []int64{0, 999, 1024, 1500, 1048576, 5_000_000_000, 1 << 40} {
		cmd := exec.Command("bash", "-c", `source ../../cli/lib/cache.bash; sct_cache_format_bytes "$1"`, "bash", strconv.FormatInt(n, 10))
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := FormatBytes(n), strings.TrimSpace(string(out)); got != want {
			t.Errorf("FormatBytes(%d) = %q, awk says %q", n, got, want)
		}
	}
}

func TestParseDockerTime(t *testing.T) {
	for _, s := range []string{
		"2026-08-07T21:55:04Z",
		"2026-08-07T21:55:04.123456789Z",
		"2026-08-07T21:55:04-05:00",
		"2026-08-07 21:55:04 +0000 UTC",
	} {
		if _, err := parseDockerTime(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	if _, err := parseDockerTime("yesterday"); err == nil {
		t.Error("garbage should not parse")
	}
}

// Run/Output must put the compose file before user args, or docker parses
// the user's args as its own global flags.
func TestComposeArgOrder(t *testing.T) {
	rec := &recorder{}
	c := Compose{File: "/p/compose-all.yml", Runner: rec}
	_ = c.Run("up", "-d")
	if want := "docker compose -f /p/compose-all.yml up -d"; rec.last != want {
		t.Errorf("got %q, want %q", rec.last, want)
	}
}

type recorder struct{ last string }

func (r *recorder) Run(name string, args ...string) error {
	r.last = name + " " + strings.Join(args, " ")
	return nil
}
func (r *recorder) OutputQuiet(name string, args ...string) (string, error) {
	return r.Output(name, args...)
}

func (r *recorder) Output(name string, args ...string) (string, error) {
	r.last = name + " " + strings.Join(args, " ")
	return "", nil
}
