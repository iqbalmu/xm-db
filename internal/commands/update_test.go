package commands

import (
	"runtime"
	"testing"
)

func TestIsUpToDate(t *testing.T) {
	cases := []struct {
		current, tag string
		want         bool
	}{
		{"1.0.0", "v1.0.0", true},
		{"1.0.0", "v1.1.0", false},
		{"1.0.0", "v0.9.0", false},
		{"1.0.0", "", false},
	}
	for _, c := range cases {
		if got := isUpToDate(c.current, c.tag); got != c.want {
			t.Errorf("isUpToDate(%q, %q) = %v, want %v", c.current, c.tag, got, c.want)
		}
	}
}

func TestAssetNames(t *testing.T) {
	archive, binary := assetNames("v1.2.3")
	wantArchive := "xmdb_1.2.3_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		wantArchive += ".zip"
	} else {
		wantArchive += ".tar.gz"
	}
	if archive != wantArchive {
		t.Errorf("archive = %q, want %q", archive, wantArchive)
	}
	wantBinary := "xmdb"
	if runtime.GOOS == "windows" {
		wantBinary += ".exe"
	}
	if binary != wantBinary {
		t.Errorf("binary = %q, want %q", binary, wantBinary)
	}
}

func TestFindChecksum(t *testing.T) {
	body := `1111111111111111111111111111111111111111111111111111111111111111  xmdb_1.0.0_linux_amd64.tar.gz
2222222222222222222222222222222222222222222222222222222222222222  xmdb_1.0.0_darwin_arm64.tar.gz
`
	got, err := findChecksum(body, "xmdb_1.0.0_darwin_arm64.tar.gz")
	if err != nil {
		t.Fatalf("findChecksum: %v", err)
	}
	if got != "2222222222222222222222222222222222222222222222222222222222222222" {
		t.Errorf("checksum = %q, want the darwin hash", got)
	}
	if _, err := findChecksum(body, "missing.tar.gz"); err == nil {
		t.Error("missing entry should error")
	}
}
