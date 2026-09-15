package version

import "testing"

// A stamped release build wins over anything derived from the tree.
func TestStringPrefersInjectedVersion(t *testing.T) {
	old := Version
	Version = "v1.2.3"
	t.Cleanup(func() { Version = old })

	if got := String(); got != "v1.2.3" {
		t.Errorf("got %q, want %q", got, "v1.2.3")
	}
}

// Matches the bash format: `git log --date=format:%Y%m%d.%H%M%S` joined to
// `git describe --abbrev=7 --dirty`.
func TestRender(t *testing.T) {
	tests := []struct {
		name  string
		rev   string
		stamp string
		dirty bool
		want  string
	}{
		{
			name:  "full",
			rev:   "c16d8fda1b2c3d4e5f60718293a4b5c6d7e8f900",
			stamp: "2026-08-07T21:55:04Z",
			want:  "20260807.215504-c16d8fd",
		},
		{
			name:  "dirty tree is flagged",
			rev:   "c16d8fda1b2c3d4e5f60718293a4b5c6d7e8f900",
			stamp: "2026-08-07T21:55:04Z",
			dirty: true,
			want:  "20260807.215504-c16d8fd-dirty",
		},
		{
			name: "unparseable timestamp degrades to bare sha",
			rev:  "c16d8fda1b2c3d4e5f60718293a4b5c6d7e8f900",
			want: "c16d8fd",
		},
		{
			name: "no revision yields no version",
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := render(tc.rev, tc.stamp, tc.dirty); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
