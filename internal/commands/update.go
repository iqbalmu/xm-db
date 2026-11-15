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
	Run: func(cmd *cobra.Command, args []string) {
		runUpdate()
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

func latestTag(base string) (string, error) {
	url := "https://api.github.com/repos/" + updateRepo + "/releases/latest"
	if base != "https://github.com/"+updateRepo {
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

func runUpdate() {
	execPath, err := os.Executable()
	if err != nil {
		color.Red("Error locating the running binary: %v", err)
		return
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		color.Red("Error resolving the binary path: %v", err)
		return
	}

	base := updateBase()
	color.Cyan("\n⬆️  Checking for updates…\n")

	tag, err := latestTag(base)
	if err != nil {
		color.Red("Error fetching the latest release: %v", err)
		return
	}

	if isUpToDate(Version, tag) {
		color.Green("Already on the latest version (%s).", Version)
		return
	}

	archive, member := assetNames(tag)
	downloadBase := base + "/releases/download/" + tag
	client := &http.Client{Timeout: 120 * time.Second}

	tmpDir := filepath.Join(filepath.Dir(execPath), ".xmdb-update")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		if os.IsPermission(err) {
			color.Red("No write permission next to %s. Rerun elevated: sudo xmdb update", execPath)
			return
		}
		color.Red("Error creating a temp directory next to the binary: %v", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, archive)
	color.White("==> Downloading %s…", archive)
	if err := download(client, downloadBase+"/"+archive, archivePath); err != nil {
		color.Red("Error downloading: %v", err)
		return
	}

	color.White("==> Verifying checksum…")
	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	if err := download(client, downloadBase+"/checksums.txt", checksumsPath); err != nil {
		color.Red("Error downloading checksums: %v", err)
		return
	}
	checksums, err := os.ReadFile(checksumsPath)
	if err != nil {
		color.Red("Error reading checksums: %v", err)
		return
	}
	want, err := findChecksum(string(checksums), archive)
	if err != nil {
		color.Red("%v", err)
		return
	}
	got, err := sha256File(archivePath)
	if err != nil {
		color.Red("Error hashing the download: %v", err)
		return
	}
	if got != want {
		color.Red("Checksum mismatch (want %s, got %s), aborting.", want, got)
		return
	}

	binaryPath := filepath.Join(tmpDir, member)
	if err := extractBinary(archivePath, member, binaryPath); err != nil {
		color.Red("Error extracting %s: %v", member, err)
		return
	}
	if err := os.Chmod(binaryPath, 0o755); err != nil {
		color.Red("Error setting permissions: %v", err)
		return
	}

	color.White("==> Replacing %s…", execPath)
	staged := filepath.Join(filepath.Dir(execPath), ".xmdb-new")
	_ = os.Remove(staged)
	if err := os.Rename(binaryPath, staged); err != nil {
		// Both paths live next to the binary, so cross-device rename cannot happen.
		color.Red("Error staging the new binary: %v", err)
		return
	}

	if runtime.GOOS == "windows" {
		// Windows refuses to overwrite a running executable: move the old one
		// aside, then clean it up best-effort.
		old := execPath + ".old"
		_ = os.Remove(old)
		if err := os.Rename(execPath, old); err != nil {
			color.Red("Error backing up the current binary: %v", err)
			return
		}
		if err := os.Rename(staged, execPath); err != nil {
			_ = os.Rename(old, execPath)
			color.Red("Error installing the new binary: %v", err)
			return
		}
		_ = os.Remove(old)
	} else {
		if err := os.Rename(staged, execPath); err != nil {
			if os.IsPermission(err) {
				color.Red("Permission denied replacing %s. Rerun elevated: sudo xmdb update", execPath)
				return
			}
			color.Red("Error installing the new binary: %v", err)
			return
		}
	}

	color.Green("\n✓ Updated: %s → %s", Version, strings.TrimPrefix(tag, "v"))
	fmt.Println("   Run `xmdb version` to confirm.")
}
