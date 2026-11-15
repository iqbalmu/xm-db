package commands

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const updateRepo = "iqbalmu/xm-db"

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update xmdb to the latest release",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUpdate()
	},
}

// XMDB_UPDATE_BASE overrides the release source (api and download URLs), used
// for testing against a local server.
func updateBase() string {
	if base := os.Getenv("XMDB_UPDATE_BASE"); base != "" {
		return base
	}
	return "https://github.com/" + updateRepo
}

// latestTag resolves the newest release tag. GitHub's JSON API rate-limits
// per IP, so the default source follows the releases/latest redirect to the
// tag page instead; custom bases keep serving their own JSON manifest.
func latestTag(base string) (string, error) {
	defaultBase := "https://github.com/" + updateRepo
	url := defaultBase + "/releases/latest"
	if base != defaultBase {
		url = base + "/latest.json"
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	if base != defaultBase {
		var release struct {
			TagName string `json:"tag_name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return "", err
		}
		if release.TagName == "" {
			return "", fmt.Errorf("no tag_name in response from %s", url)
		}
		return release.TagName, nil
	}

	// The redirect lands on .../tag/<name>; read the tag back off the URL.
	tag := path.Base(resp.Request.URL.Path)
	if !strings.HasPrefix(tag, "v") {
		return "", fmt.Errorf("could not determine the latest tag from %s", url)
	}
	return tag, nil
}

func isUpToDate(current, tag string) bool {
	return strings.TrimPrefix(tag, "v") == current
}

func assetNames(tag string) (string, string) {
	ver := strings.TrimPrefix(tag, "v")
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	archive := fmt.Sprintf("xm-db_%s_%s_%s.%s", ver, runtime.GOOS, runtime.GOARCH, ext)
	binary := "xmdb"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	return archive, binary
}

func findChecksum(checksums, archive string) (string, error) {
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archive {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum found for %s", archive)
}

func download(client *http.Client, url, dest string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func extractBinary(archivePath, member, dest string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractFromZip(archivePath, member, dest)
	}
	return extractFromTarGz(archivePath, member, dest)
}

func extractFromTarGz(archivePath, member, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("%s not found in archive", member)
		}
		if err != nil {
			return err
		}
		if filepath.Base(header.Name) == member {
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, tr)
			return err
		}
	}
}

func extractFromZip(archivePath, member, dest string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, f := range reader.File {
		if filepath.Base(f.Name) == member {
			src, err := f.Open()
			if err != nil {
				return err
			}
			defer src.Close()
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode())
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, src)
			return err
		}
	}
	return fmt.Errorf("%s not found in archive", member)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func runUpdate() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating the running binary: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolving the binary path: %w", err)
	}

	base := updateBase()
	color.Cyan("\nChecking for updates…\n")

	tag, err := latestTag(base)
	if err != nil {
		return fmt.Errorf("fetching the latest release: %w", err)
	}

	if isUpToDate(Version, tag) {
		color.Green("Already on the latest version (%s).", Version)
		return nil
	}

	archive, member := assetNames(tag)
	downloadBase := base + "/releases/download/" + tag
	client := &http.Client{Timeout: 120 * time.Second}

	tmpDir := filepath.Join(filepath.Dir(execPath), ".xmdb-update")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("no write permission next to %s; rerun elevated: sudo xmdb update", execPath)
		}
		return fmt.Errorf("creating temp directory next to the binary: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, archive)
	color.White("==> Downloading %s…", archive)
	if err := download(client, downloadBase+"/"+archive, archivePath); err != nil {
		return fmt.Errorf("downloading: %w", err)
	}

	color.White("==> Verifying checksum…")
	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	if err := download(client, downloadBase+"/checksums.txt", checksumsPath); err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}
	checksums, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("reading checksums: %w", err)
	}
	want, err := findChecksum(string(checksums), archive)
	if err != nil {
		return err
	}
	got, err := sha256File(archivePath)
	if err != nil {
		return fmt.Errorf("hashing the download: %w", err)
	}
	if got != want {
		return fmt.Errorf("checksum mismatch (want %s, got %s), aborting", want, got)
	}

	binaryPath := filepath.Join(tmpDir, member)
	if err := extractBinary(archivePath, member, binaryPath); err != nil {
		return fmt.Errorf("extracting %s: %w", member, err)
	}
	if err := os.Chmod(binaryPath, 0o755); err != nil {
		return fmt.Errorf("setting permissions: %w", err)
	}

	color.White("==> Replacing %s…", execPath)
	staged := filepath.Join(filepath.Dir(execPath), ".xmdb-new")
	_ = os.Remove(staged)
	if err := os.Rename(binaryPath, staged); err != nil {
		// Both paths live next to the binary, so cross-device rename cannot happen.
		return fmt.Errorf("staging the new binary: %w", err)
	}

	if runtime.GOOS == "windows" {
		// Windows refuses to overwrite a running executable: move the old one
		// aside, then clean it up best-effort.
		old := execPath + ".old"
		_ = os.Remove(old)
		if err := os.Rename(execPath, old); err != nil {
			return fmt.Errorf("backing up the current binary: %w", err)
		}
		if err := os.Rename(staged, execPath); err != nil {
			_ = os.Rename(old, execPath)
			return fmt.Errorf("installing the new binary: %w", err)
		}
		_ = os.Remove(old)
	} else {
		if err := os.Rename(staged, execPath); err != nil {
			if os.IsPermission(err) {
				return fmt.Errorf("permission denied replacing %s; rerun elevated: sudo xmdb update", execPath)
			}
			return fmt.Errorf("installing the new binary: %w", err)
		}
	}

	color.Green("\nUpdated: %s -> %s", Version, strings.TrimPrefix(tag, "v"))
	fmt.Println("   Run `xmdb version` to confirm.")
	return nil
}
