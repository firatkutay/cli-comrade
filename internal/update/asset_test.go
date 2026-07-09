package update

import "testing"

func TestArchiveName(t *testing.T) {
	cases := []struct {
		name    string
		project string
		version string
		goos    string
		goarch  string
		want    string
	}{
		{"linux amd64", "comrade", "0.2.0", "linux", "amd64", "comrade_0.2.0_linux_amd64.tar.gz"},
		{"darwin arm64", "comrade", "0.2.0", "darwin", "arm64", "comrade_0.2.0_darwin_arm64.tar.gz"},
		{"windows amd64 zips", "comrade", "0.2.0", "windows", "amd64", "comrade_0.2.0_windows_amd64.zip"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ArchiveName(tc.project, tc.version, tc.goos, tc.goarch)
			if got != tc.want {
				t.Errorf("ArchiveName(%q, %q, %q, %q) = %q, want %q", tc.project, tc.version, tc.goos, tc.goarch, got, tc.want)
			}
		})
	}
}

func TestBinaryName(t *testing.T) {
	if got := BinaryName("windows"); got != "comrade.exe" {
		t.Errorf("BinaryName(windows) = %q, want comrade.exe", got)
	}
	if got := BinaryName("linux"); got != "comrade" {
		t.Errorf("BinaryName(linux) = %q, want comrade", got)
	}
	if got := BinaryName("darwin"); got != "comrade" {
		t.Errorf("BinaryName(darwin) = %q, want comrade", got)
	}
}

func TestStripVersionPrefix(t *testing.T) {
	if got := StripVersionPrefix("v0.2.0"); got != "0.2.0" {
		t.Errorf("StripVersionPrefix(v0.2.0) = %q, want 0.2.0", got)
	}
	if got := StripVersionPrefix("0.2.0"); got != "0.2.0" {
		t.Errorf("StripVersionPrefix(0.2.0) = %q, want 0.2.0 (unchanged)", got)
	}
}
