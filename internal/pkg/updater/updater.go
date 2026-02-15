package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
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

	"github.com/Masterminds/semver/v3"

	"github.com/tgifai/friday"
	"github.com/tgifai/friday/internal/pkg/logs"
)

const (
	repoOwner = "TGIFAI"
	repoName  = "friday"
)

// Release represents a GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a release asset.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Updater checks for and applies updates from GitHub releases.
type Updater struct {
	currentVersion string
	httpClient     *http.Client
}

// New creates a new Updater with the current build version.
func New() *Updater {
	return &Updater{
		currentVersion: friday.VERSION,
		httpClient:     &http.Client{},
	}
}

// NeedsUpdate checks if a newer version is available.
func (u *Updater) NeedsUpdate(ctx context.Context) (bool, *Release, error) {
	release, err := u.CheckLatest(ctx)
	if err != nil {
		return false, nil, err
	}
	if release == nil {
		return false, nil, nil
	}
	return true, release, nil
}

// CheckLatest fetches the latest release and returns it if newer than current version.
// Returns nil if the current version is already up to date.
func (u *Updater) CheckLatest(ctx context.Context) (*Release, error) {
	if u.currentVersion == "n/a" || u.currentVersion == "" {
		return nil, fmt.Errorf("current version unknown (built without -ldflags); cannot check for updates")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}

	local, err := semver.NewVersion(u.currentVersion)
	if err != nil {
		return nil, fmt.Errorf("parse local version %q: %w", u.currentVersion, err)
	}
	remote, err := semver.NewVersion(release.TagName)
	if err != nil {
		return nil, fmt.Errorf("parse remote version %q: %w", release.TagName, err)
	}

	if !remote.GreaterThan(local) {
		return nil, nil // already up to date
	}
	return &release, nil
}

// Download fetches the appropriate binary asset for the current platform to targetDir,
// verifies its checksum, and returns the path to the extracted binary.
func (u *Updater) Download(ctx context.Context, release *Release, targetDir string) (string, error) {
	archiveName := fmt.Sprintf("friday_%s_%s_%s.tar.gz", release.TagName, runtime.GOOS, runtime.GOARCH)
	var archiveURL string
	var checksumURL string

	for _, a := range release.Assets {
		if a.Name == archiveName {
			archiveURL = a.BrowserDownloadURL
		}
		if a.Name == "checksums.txt" {
			checksumURL = a.BrowserDownloadURL
		}
	}
	if archiveURL == "" {
		return "", fmt.Errorf("no asset found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	// Download checksums
	var expectedHash string
	if checksumURL != "" {
		hash, err := u.fetchExpectedHash(ctx, checksumURL, archiveName)
		if err != nil {
			logs.CtxWarn(ctx, "failed to fetch checksums, skipping verification: %v", err)
		} else {
			expectedHash = hash
		}
	}

	// Download archive
	archivePath := filepath.Join(targetDir, archiveName)
	if err := u.downloadFile(ctx, archiveURL, archivePath); err != nil {
		return "", fmt.Errorf("download archive: %w", err)
	}

	// Verify checksum
	if expectedHash != "" {
		actualHash, err := fileSHA256(archivePath)
		if err != nil {
			return "", fmt.Errorf("compute checksum: %w", err)
		}
		if actualHash != expectedHash {
			os.Remove(archivePath)
			return "", fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
		}
	}

	// Extract binary from tar.gz
	binaryPath, err := extractBinary(archivePath, targetDir)
	if err != nil {
		return "", fmt.Errorf("extract binary: %w", err)
	}

	os.Remove(archivePath)
	return binaryPath, nil
}

// Apply replaces the current binary with the new one at newBinaryPath.
// It backs up the current binary and rolls back on failure.
func (u *Updater) Apply(newBinaryPath string) error {
	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get current executable path: %w", err)
	}
	currentPath, err = filepath.EvalSymlinks(currentPath)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	backupPath := currentPath + ".bak"

	// Remove any leftover backup
	os.Remove(backupPath)

	// Backup current binary
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}

	// Move new binary into place
	if err := os.Rename(newBinaryPath, currentPath); err != nil {
		// Rollback
		if rbErr := os.Rename(backupPath, currentPath); rbErr != nil {
			return fmt.Errorf("apply failed (%v) and rollback also failed (%v)", err, rbErr)
		}
		return fmt.Errorf("apply new binary (rolled back): %w", err)
	}

	// Make executable
	if err := os.Chmod(currentPath, 0o755); err != nil {
		// Not fatal on all platforms but log it
		logs.Warn("chmod new binary: %v", err)
	}

	// Clean up backup
	os.Remove(backupPath)
	return nil
}

func (u *Updater) fetchExpectedHash(ctx context.Context, checksumURL, archiveName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == archiveName {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("no checksum found for %s", archiveName)
}

func (u *Updater) downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func fileSHA256(path string) (string, error) {
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

func extractBinary(archivePath, targetDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		// The binary is the only regular file in the archive
		destPath := filepath.Join(targetDir, filepath.Base(header.Name))
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return "", err
		}
		out.Close()
		return destPath, nil
	}

	return "", fmt.Errorf("no binary found in archive")
}
