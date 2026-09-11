package dockercli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jehoctor/snadcat/internal/compose"
	"github.com/jehoctor/snadcat/internal/log"
)

// staleHomeTolerance absorbs the few seconds by which the image is naturally
// newer than the agent-home volume on a fresh install, where Compose creates
// named volumes before it builds images. A genuine post-install rebuild
// leaves a much larger gap.
const staleHomeTolerance = 60 * time.Second

// WarnStaleHomeVolume warns when the agent image was rebuilt after the
// agent-home volume was created, since packages installed at build time are
// then hidden under the stale volume overlay at /home/vscode.
//
// Every failure path is silent: this is advisory, and a missing volume
// (first run) or unparsable timestamp must never block `sandcat run`.
func WarnStaleHomeVolume(d Docker, composeFile string) {
	cf, err := compose.Load(composeFile)
	if err != nil {
		return
	}
	project := cf.ProjectName()
	if project == "" {
		return
	}
	volume := project + "_agent-home"
	image := project + "-agent"

	volumeTime, err := d.OutputQuiet("volume", "inspect", "--format", "{{.CreatedAt}}", volume)
	if err != nil {
		return
	}
	imageTime, err := d.OutputQuiet("image", "inspect", "--format", "{{.Created}}", image)
	if err != nil {
		return
	}
	vt, err1 := parseDockerTime(volumeTime)
	it, err2 := parseDockerTime(imageTime)
	if err1 != nil || err2 != nil {
		return
	}
	if it.Sub(vt) <= staleHomeTolerance {
		return
	}

	log.Warn("The agent image was rebuilt since the agent-home volume was created.")
	log.Warn("Packages installed during the build may not be visible.")
	log.Warn("To fix, stop containers and remove the volume:")
	log.Warn("  sandcat compose down && docker volume rm %s", volume)
}

// parseDockerTime accepts the timestamp shapes docker inspect emits:
// RFC3339 with or without fractional seconds, and the Go default layout
// `2006-01-02 15:04:05 -0700 MST` some versions use for CreatedAt.
func parseDockerTime(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05 -0700 MST"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised timestamp %q", s)
}

// SharedCacheLabel marks volumes created by sandcat for the `cache` commands.
const SharedCacheLabel = "sandcat-shared-cache=true"

// EnsureSharedCacheVolumes creates every sandcat-cache-* external volume the
// compose file references, so `compose up` doesn't fail with "external volume
// not found". `volume create` is idempotent, so it runs unconditionally.
func EnsureSharedCacheVolumes(d Docker, composeFile string) {
	cf, err := compose.Load(composeFile)
	if err != nil {
		return
	}
	for _, name := range cf.ExternalCacheVolumes() {
		// Best-effort, as in the bash: a failure here surfaces more usefully
		// from `compose up` itself.
		_, _ = d.OutputQuiet("volume", "create", "--label", SharedCacheLabel, name)
	}
}

// CacheVolumeNames lists the shared-cache volumes on this host, sorted.
func CacheVolumeNames(d Docker) []string {
	out, err := d.OutputQuiet("volume", "ls", "-q", "--filter", "label="+SharedCacheLabel)
	if err != nil || out == "" {
		return nil
	}
	names := strings.Split(out, "\n")
	sort.Strings(names)
	return names
}

// CacheVolumeSizeBytes measures a volume by mounting it into a throwaway
// alpine container and running du. Docker Desktop's VM makes host-side
// inspection unreliable, so the bash goes through a container and so does this.
func CacheVolumeSizeBytes(d Docker, name string) int64 {
	out, err := d.OutputQuiet("run", "--rm", "-v", name+":/mnt", "alpine", "du", "-sb", "/mnt")
	if err != nil {
		return 0
	}
	return firstInt(out)
}

// CacheVolumeFileCount counts regular files in a volume, the same way.
func CacheVolumeFileCount(d Docker, name string) int64 {
	out, err := d.Output("run", "--rm", "-v", name+":/mnt", "alpine", "sh", "-c",
		"find /mnt -type f 2>/dev/null | wc -l")
	if err != nil {
		return 0
	}
	return firstInt(out)
}

// firstInt parses the leading whitespace-delimited field as an integer, the
// `awk '{print $1}'` of the bash helpers. Anything unparsable is 0.
func firstInt(s string) int64 {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0
	}
	n, _ := strconv.ParseInt(f[0], 10, 64)
	return n
}

// CacheVolumeContainers returns the names of containers mounting the volume.
// Empty means it is free to remove.
func CacheVolumeContainers(d Docker, name string) []string {
	out, err := d.OutputQuiet("ps", "-a", "--filter", "volume="+name, "--format", "{{.Names}}")
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// FormatBytes renders a byte count like the awk in cache.bash: integer bytes
// below 1 KB, otherwise one decimal place.
func FormatBytes(b int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v := float64(b)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", b, units[i])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
