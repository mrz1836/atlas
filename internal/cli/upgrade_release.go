// Package cli provides the command-line interface for atlas.
package cli

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	atlasErrors "github.com/mrz1836/atlas/internal/errors"
)

// GitHubRelease represents a GitHub release API response.
type GitHubRelease struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Assets      []ReleaseAsset `json:"assets"`
	PreRelease  bool           `json:"prerelease"`
	Draft       bool           `json:"draft"`
	PublishedAt string         `json:"published_at"`
}

// ReleaseAsset represents a release asset from GitHub.
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
}

// ReleaseClient defines the interface for fetching GitHub release information.
// This allows mocking in tests.
type ReleaseClient interface {
	// GetLatestRelease fetches the latest release from GitHub.
	GetLatestRelease(ctx context.Context, owner, repo string) (*GitHubRelease, error)
}

// ReleaseDownloader defines the interface for downloading release assets.
// This allows mocking in tests.
type ReleaseDownloader interface {
	// DownloadFile downloads a file from a URL to a temporary location.
	// Returns the path to the downloaded file.
	DownloadFile(ctx context.Context, url string) (string, error)
}

// AtlasReleaseUpgrader handles upgrading atlas via GitHub releases.
type AtlasReleaseUpgrader struct {
	client     ReleaseClient
	downloader ReleaseDownloader
	executor   config.CommandExecutor
}

// NewAtlasReleaseUpgrader creates a new AtlasReleaseUpgrader with default implementations.
func NewAtlasReleaseUpgrader(executor config.CommandExecutor) *AtlasReleaseUpgrader {
	return &AtlasReleaseUpgrader{
		client:     NewDefaultReleaseClient(executor),
		downloader: NewDefaultReleaseDownloader(executor),
		executor:   executor,
	}
}

// NewAtlasReleaseUpgraderWithDeps creates a new AtlasReleaseUpgrader with custom dependencies.
// This is used for testing.
func NewAtlasReleaseUpgraderWithDeps(client ReleaseClient, downloader ReleaseDownloader, executor config.CommandExecutor) *AtlasReleaseUpgrader {
	return &AtlasReleaseUpgrader{
		client:     client,
		downloader: downloader,
		executor:   executor,
	}
}

// UpgradeAtlas upgrades atlas to the latest release.
// Returns the new version string on success.
func (u *AtlasReleaseUpgrader) UpgradeAtlas(ctx context.Context, currentVersion string) (string, error) {
	// 1. Get latest release
	release, err := u.client.GetLatestRelease(ctx, constants.GitHubOwner, constants.GitHubRepo)
	if err != nil {
		return "", fmt.Errorf("%w: %v", atlasErrors.ErrUpgradeNoRelease, err) //nolint:errorlint // intentional hybrid wrap
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	// 2. Compare versions - skip if already on latest
	if !isNewerVersion(currentVersion, latestVersion) {
		return currentVersion, nil // Already on latest
	}

	// 3. Find the correct binary asset for this platform
	binaryAssetName := getBinaryAssetName(release.TagName)
	checksumAssetName := getChecksumAssetName(release.TagName)

	var binaryAsset, checksumAsset *ReleaseAsset
	for i := range release.Assets {
		if release.Assets[i].Name == binaryAssetName {
			binaryAsset = &release.Assets[i]
		}
		if release.Assets[i].Name == checksumAssetName {
			checksumAsset = &release.Assets[i]
		}
	}

	if binaryAsset == nil {
		return "", fmt.Errorf("%w: %s (available: %s)", atlasErrors.ErrUpgradeAssetNotFound, binaryAssetName, listAssetNames(release.Assets))
	}

	if checksumAsset == nil {
		return "", fmt.Errorf("%w: checksum file %s", atlasErrors.ErrUpgradeAssetNotFound, checksumAssetName)
	}

	// 4. Download checksum file first
	checksumPath, err := u.downloader.DownloadFile(ctx, checksumAsset.BrowserDownloadURL)
	if err != nil {
		return "", fmt.Errorf("%w: failed to download checksums: %v", atlasErrors.ErrUpgradeDownloadFailed, err) //nolint:errorlint // intentional hybrid wrap
	}
	defer os.Remove(checksumPath) //nolint:errcheck // cleanup best-effort

	// 5. Parse expected checksum
	expectedChecksum, err := extractChecksumForFile(checksumPath, binaryAssetName)
	if err != nil {
		return "", fmt.Errorf("%w: failed to parse checksums: %v", atlasErrors.ErrUpgradeChecksumMismatch, err) //nolint:errorlint // intentional hybrid wrap
	}

	// 6. Download binary archive
	archivePath, err := u.downloader.DownloadFile(ctx, binaryAsset.BrowserDownloadURL)
	if err != nil {
		return "", fmt.Errorf("%w: failed to download binary: %v", atlasErrors.ErrUpgradeDownloadFailed, err) //nolint:errorlint // intentional hybrid wrap
	}
	defer os.Remove(archivePath) //nolint:errcheck // cleanup best-effort

	// 7. Verify checksum
	if verifyErr := verifyChecksum(archivePath, expectedChecksum); verifyErr != nil {
		return "", verifyErr
	}

	// 8. Extract binary from archive
	binaryPath, err := extractBinaryFromArchive(archivePath, binaryAssetName)
	if err != nil {
		return "", fmt.Errorf("%w: failed to extract binary: %v", atlasErrors.ErrUpgradeDownloadFailed, err) //nolint:errorlint // intentional hybrid wrap
	}
	defer os.Remove(binaryPath) //nolint:errcheck // cleanup best-effort

	// 9. Find current binary location
	currentBinaryPath, err := u.executor.LookPath("atlas")
	if err != nil {
		return "", fmt.Errorf("%w: cannot find current atlas binary: %v", atlasErrors.ErrUpgradeReplaceFailed, err) //nolint:errorlint // intentional hybrid wrap
	}

	// 10. Atomic replace
	if err := atomicReplaceBinary(currentBinaryPath, binaryPath); err != nil {
		return "", err
	}

	return latestVersion, nil
}

// GetLatestVersion fetches the latest version from GitHub without upgrading.
func (u *AtlasReleaseUpgrader) GetLatestVersion(ctx context.Context) (string, error) {
	release, err := u.client.GetLatestRelease(ctx, constants.GitHubOwner, constants.GitHubRepo)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(release.TagName, "v"), nil
}

// DefaultReleaseClient implements ReleaseClient using gh CLI with HTTP fallback.
type DefaultReleaseClient struct {
	executor   config.CommandExecutor
	httpClient HTTPClient
}

// HTTPClient abstracts HTTP operations for testing.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewDefaultReleaseClient creates a new DefaultReleaseClient.
func NewDefaultReleaseClient(executor config.CommandExecutor) *DefaultReleaseClient {
	return &DefaultReleaseClient{
		executor: executor,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewDefaultReleaseClientWithHTTP creates a client with a custom HTTP client (for testing).
func NewDefaultReleaseClientWithHTTP(executor config.CommandExecutor, httpClient HTTPClient) *DefaultReleaseClient {
	return &DefaultReleaseClient{
		executor:   executor,
		httpClient: httpClient,
	}
}

// GetLatestRelease fetches the latest release, trying gh first then falling back to HTTP.
func (c *DefaultReleaseClient) GetLatestRelease(ctx context.Context, owner, repo string) (*GitHubRelease, error) {
	// Try gh CLI first (authenticated, better rate limits)
	release, err := c.getLatestReleaseViaGH(ctx, owner, repo)
	if err == nil {
		return release, nil
	}

	// Fall back to HTTP API (unauthenticated)
	return c.getLatestReleaseViaHTTP(ctx, owner, repo)
}

// getLatestReleaseViaGH fetches release info using the gh CLI.
func (c *DefaultReleaseClient) getLatestReleaseViaGH(ctx context.Context, owner, repo string) (*GitHubRelease, error) {
	// Check if gh is available
	if _, err := c.executor.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh not found: %w", err)
	}

	// Use gh api to fetch release info
	apiPath := fmt.Sprintf("repos/%s/%s/releases/latest", owner, repo)
	output, err := c.executor.Run(ctx, "gh", "api", apiPath)
	if err != nil {
		return nil, fmt.Errorf("gh api failed: %w", err)
	}

	var release GitHubRelease
	if err := json.Unmarshal([]byte(output), &release); err != nil {
		return nil, fmt.Errorf("failed to parse release: %w", err)
	}

	return &release, nil
}

// getLatestReleaseViaHTTP fetches release info using direct HTTP API calls.
func (c *DefaultReleaseClient) getLatestReleaseViaHTTP(ctx context.Context, owner, repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", constants.GitHubAPIBaseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "atlas-cli")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // HTTP response body close

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("%w: status %d (failed to read response body: %w)", atlasErrors.ErrUpgradeDownloadFailed, resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("%w: status %d: %s", atlasErrors.ErrUpgradeDownloadFailed, resp.StatusCode, string(body))
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &release, nil
}

// DefaultReleaseDownloader implements ReleaseDownloader using gh CLI with HTTP fallback.
type DefaultReleaseDownloader struct {
	executor   config.CommandExecutor
	httpClient HTTPClient
}

// NewDefaultReleaseDownloader creates a new DefaultReleaseDownloader.
func NewDefaultReleaseDownloader(executor config.CommandExecutor) *DefaultReleaseDownloader {
	return &DefaultReleaseDownloader{
		executor: executor,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Longer timeout for downloads
		},
	}
}

// NewDefaultReleaseDownloaderWithHTTP creates a downloader with a custom HTTP client.
func NewDefaultReleaseDownloaderWithHTTP(executor config.CommandExecutor, httpClient HTTPClient) *DefaultReleaseDownloader {
	return &DefaultReleaseDownloader{
		executor:   executor,
		httpClient: httpClient,
	}
}

// DownloadFile downloads a file from URL to a temporary location.
// Tries gh CLI first (authenticated, works with private repos), falls back to HTTP.
func (d *DefaultReleaseDownloader) DownloadFile(ctx context.Context, url string) (string, error) {
	// Try gh CLI first (authenticated, works with private repos)
	if d.executor != nil {
		path, err := d.downloadFileViaGH(ctx, url)
		if err == nil {
			return path, nil
		}
		// Fall through to HTTP if gh fails
	}

	// Fall back to HTTP (unauthenticated)
	return d.downloadFileViaHTTP(ctx, url)
}

// downloadFileViaGH downloads a release asset using gh CLI.
// URL format: https://github.com/OWNER/REPO/releases/download/TAG/ASSET
func (d *DefaultReleaseDownloader) downloadFileViaGH(ctx context.Context, url string) (string, error) {
	// Check if gh is available
	if _, err := d.executor.LookPath("gh"); err != nil {
		return "", fmt.Errorf("gh not found: %w", err)
	}

	// Parse URL to extract owner, repo, tag, and asset name
	// Format: https://github.com/OWNER/REPO/releases/download/TAG/ASSET
	owner, repo, tag, assetName, err := parseGitHubReleaseURL(url)
	if err != nil {
		return "", fmt.Errorf("cannot parse GitHub URL: %w", err)
	}

	// Create temp directory for download
	tmpDir, err := os.MkdirTemp("", "atlas-download-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Download using gh release download
	repoArg := fmt.Sprintf("%s/%s", owner, repo)
	_, err = d.executor.Run(ctx, "gh", "release", "download", tag,
		"--repo", repoArg,
		"--pattern", assetName,
		"--dir", tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("gh release download failed: %w", err)
	}

	// Return path to downloaded file
	downloadedPath := filepath.Join(tmpDir, assetName)
	if _, statErr := os.Stat(downloadedPath); statErr != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("downloaded file not found: %w", statErr)
	}

	return downloadedPath, nil
}

// parseGitHubReleaseURL parses a GitHub release download URL.
// Returns owner, repo, tag, and asset name.
// URL format: https://github.com/OWNER/REPO/releases/download/TAG/ASSET
func parseGitHubReleaseURL(url string) (owner, repo, tag, assetName string, err error) {
	// Expected format: https://github.com/OWNER/REPO/releases/download/TAG/ASSET
	const prefix = "https://github.com/"
	if !strings.HasPrefix(url, prefix) {
		return "", "", "", "", fmt.Errorf("%w: not a github.com URL", atlasErrors.ErrInvalidURL)
	}

	// Remove prefix and split
	path := strings.TrimPrefix(url, prefix)
	parts := strings.Split(path, "/")

	// Need at least: OWNER/REPO/releases/download/TAG/ASSET (6 parts)
	if len(parts) < 6 {
		return "", "", "", "", fmt.Errorf("%w: invalid release URL format", atlasErrors.ErrInvalidURL)
	}

	if parts[2] != "releases" || parts[3] != "download" {
		return "", "", "", "", fmt.Errorf("%w: not a release download URL", atlasErrors.ErrInvalidURL)
	}

	return parts[0], parts[1], parts[4], parts[5], nil
}

// downloadFileViaHTTP downloads a file using HTTP (unauthenticated).
func (d *DefaultReleaseDownloader) downloadFileViaHTTP(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "atlas-cli")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // HTTP response body close

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: status %d", atlasErrors.ErrUpgradeDownloadFailed, resp.StatusCode)
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "atlas-upgrade-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close() //nolint:errcheck // temp file close

	// Copy content
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		_ = os.Remove(tmpFile.Name()) //nolint:gosec // G703: path from os.CreateTemp, not user input
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return tmpFile.Name(), nil
}

// getBinaryAssetName returns the expected asset name for the current platform.
func getBinaryAssetName(tagName string) string {
	version := strings.TrimPrefix(tagName, "v")
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("atlas_%s_%s_%s%s", version, runtime.GOOS, runtime.GOARCH, ext)
}

// getChecksumAssetName returns the expected checksum file name.
func getChecksumAssetName(tagName string) string {
	version := strings.TrimPrefix(tagName, "v")
	return fmt.Sprintf("atlas_%s_checksums.txt", version)
}

// listAssetNames returns a comma-separated list of asset names for error messages.
func listAssetNames(assets []ReleaseAsset) string {
	names := make([]string, len(assets))
	for i, a := range assets {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}

// extractChecksumForFile extracts the checksum for a specific file from a checksums file.
func extractChecksumForFile(checksumFilePath, targetFile string) (string, error) {
	file, err := os.Open(checksumFilePath) //nolint:gosec // path from controlled source
	if err != nil {
		return "", err
	}
	defer file.Close() //nolint:errcheck // file close in reader

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Format: "checksum  filename" or "checksum filename"
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			checksum := parts[0]
			filename := parts[len(parts)-1]
			if filename == targetFile {
				return checksum, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("%w: file %s not in checksums", atlasErrors.ErrUpgradeChecksumMismatch, targetFile)
}

// verifyChecksum verifies the SHA256 checksum of a file.
func verifyChecksum(filePath, expectedChecksum string) error {
	file, err := os.Open(filePath) //nolint:gosec // path from controlled source
	if err != nil {
		return fmt.Errorf("%w: cannot open file: %v", atlasErrors.ErrUpgradeChecksumMismatch, err) //nolint:errorlint // intentional hybrid wrap
	}
	defer file.Close() //nolint:errcheck // file close in reader

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("%w: cannot hash file: %v", atlasErrors.ErrUpgradeChecksumMismatch, err) //nolint:errorlint // intentional hybrid wrap
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualChecksum, expectedChecksum) {
		return fmt.Errorf("%w: expected %s, got %s", atlasErrors.ErrUpgradeChecksumMismatch, expectedChecksum, actualChecksum)
	}

	return nil
}

// extractBinaryFromArchive extracts the atlas binary from a tar.gz or zip archive.
func extractBinaryFromArchive(archivePath, archiveName string) (string, error) {
	if strings.HasSuffix(archiveName, ".zip") {
		return extractFromZip(archivePath)
	}
	return extractFromTarGz(archivePath)
}

// extractFromTarGz extracts the atlas binary from a tar.gz archive.
func extractFromTarGz(archivePath string) (string, error) { //nolint:gocognit // tar extraction inherently complex
	file, err := os.Open(archivePath) //nolint:gosec // path from controlled source
	if err != nil {
		return "", err
	}
	defer file.Close() //nolint:errcheck // file close in reader

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gzr.Close() //nolint:errcheck // gzip reader close

	tr := tar.NewReader(gzr)

	binaryName := "atlas"
	if runtime.GOOS == "windows" {
		binaryName = "atlas.exe"
	}

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read tar header: %w", err)
		}

		// Look for the atlas binary
		if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == binaryName {
			tmpFile, err := os.CreateTemp("", "atlas-binary-*")
			if err != nil {
				return "", fmt.Errorf("create temp file: %w", err)
			}

			// Use limited copy to prevent decompression bomb
			if _, copyErr := io.CopyN(tmpFile, tr, header.Size); copyErr != nil && !errors.Is(copyErr, io.EOF) {
				_ = tmpFile.Close()
				_ = os.Remove(tmpFile.Name()) //nolint:gosec // G703: path from os.CreateTemp, not user input
				return "", copyErr
			}
			_ = tmpFile.Close()

			// Make executable
			if chmodErr := os.Chmod(tmpFile.Name(), 0o755); chmodErr != nil { //nolint:gosec // executable needed
				_ = os.Remove(tmpFile.Name()) //nolint:gosec // G703: path from os.CreateTemp, not user input
				return "", chmodErr
			}

			return tmpFile.Name(), nil
		}
	}

	return "", fmt.Errorf("%w: binary %s not in archive", atlasErrors.ErrUpgradeAssetNotFound, binaryName)
}

// extractFromZip extracts the atlas binary from a zip archive.
func extractFromZip(archivePath string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("open zip archive: %w", err)
	}
	defer r.Close() //nolint:errcheck // zip reader close

	binaryName := "atlas"
	if runtime.GOOS == "windows" {
		binaryName = "atlas.exe"
	}

	for _, f := range r.File {
		if filepath.Base(f.Name) == binaryName && !f.FileInfo().IsDir() {
			return extractZipFile(f)
		}
	}

	return "", fmt.Errorf("%w: binary %s not in archive", atlasErrors.ErrUpgradeAssetNotFound, binaryName)
}

// extractZipFile extracts a single file from a zip archive.
func extractZipFile(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", fmt.Errorf("open zip entry: %w", err)
	}
	defer rc.Close() //nolint:errcheck // zip file close

	tmpFile, err := os.CreateTemp("", "atlas-binary-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	// Use limited copy to prevent decompression bomb
	if _, copyErr := io.CopyN(tmpFile, rc, int64(f.UncompressedSize64)); copyErr != nil && !errors.Is(copyErr, io.EOF) { //nolint:gosec // size from zip header
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name()) //nolint:gosec // G703: path from os.CreateTemp, not user input
		return "", copyErr
	}
	_ = tmpFile.Close()

	// Make executable
	if err := os.Chmod(tmpFile.Name(), 0o755); err != nil { //nolint:gosec // executable needed
		_ = os.Remove(tmpFile.Name()) //nolint:gosec // G703: path from os.CreateTemp, not user input
		return "", err
	}

	return tmpFile.Name(), nil
}

// atomicReplaceBinary atomically replaces the current binary with the new one.
func atomicReplaceBinary(currentPath, newPath string) error {
	// Get current binary info for permissions
	currentInfo, err := os.Stat(currentPath)
	if err != nil {
		return fmt.Errorf("%w: cannot stat current binary: %v", atlasErrors.ErrUpgradeReplaceFailed, err) //nolint:errorlint // intentional hybrid wrap
	}

	// Create backup
	backupPath := currentPath + ".backup"
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("%w: cannot backup current binary: %v", atlasErrors.ErrUpgradeReplaceFailed, err) //nolint:errorlint // intentional hybrid wrap
	}

	// Copy new binary to target location (we can't rename across filesystems)
	if copyErr := copyBinaryFile(newPath, currentPath, currentInfo.Mode()); copyErr != nil {
		// Restore from backup
		if restoreErr := os.Rename(backupPath, currentPath); restoreErr != nil {
			return fmt.Errorf("%w: install failed and restore failed: %w",
				atlasErrors.ErrUpgradeReplaceFailed, restoreErr)
		}
		return fmt.Errorf("%w: %v", atlasErrors.ErrUpgradeReplaceFailed, copyErr) //nolint:errorlint // intentional hybrid wrap
	}

	// Remove backup on success
	_ = os.Remove(backupPath)

	return nil
}

// copyBinaryFile copies a binary file preserving the specified permissions.
func copyBinaryFile(src, dst string, mode os.FileMode) error {
	srcFile, err := os.Open(src) //nolint:gosec // path from controlled source
	if err != nil {
		return fmt.Errorf("open source %s: %w", src, err)
	}
	defer srcFile.Close() //nolint:errcheck // file close in reader

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode) //nolint:gosec // path from controlled source
	if err != nil {
		return fmt.Errorf("create destination %s: %w", dst, err)
	}
	defer dstFile.Close() //nolint:errcheck // will be closed explicitly

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy to %s: %w", dst, err)
	}

	return dstFile.Close()
}

// isNewerVersion returns true if latest is newer than current.
// Uses simple string comparison - could be enhanced with semver parsing.
func isNewerVersion(current, latest string) bool {
	// Strip 'v' prefix if present
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	// Handle dev/unknown versions - always upgrade
	if current == "dev" || current == "unknown" || current == "" {
		return true
	}

	// Simple comparison - for proper semver, could use a library
	// For now, different versions means potentially newer
	return current != latest
}
