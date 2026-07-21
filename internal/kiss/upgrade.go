package kiss

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const defaultUpgradeRepo = "wwulfric/kiss"

var githubReleaseBaseURL = "https://github.com"

var upgradeRuntimeGOOS = runtime.GOOS
var upgradeRuntimeGOARCH = runtime.GOARCH
var upgradeExecutablePath = os.Executable
var upgradeVerifyBinary = runBinaryVersion
var upgradeInstallBinary = installUpgradeBinary

var semverPattern = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)$`)

// UpgradeOptions 描述 KISS CLI 自升级的目标版本和覆盖策略。
type UpgradeOptions struct {
	Version string
	Force   bool
}

// UpgradeCLI 下载、校验并安装新的 KISS CLI。
func UpgradeCLI(options UpgradeOptions, out io.Writer) error {
	writer := errWriter{out: out}
	repo := strings.TrimSpace(os.Getenv("KISS_REPO"))
	if repo == "" {
		repo = defaultUpgradeRepo
	}
	if err := validateUpgradeRepo(repo); err != nil {
		return err
	}
	targetVersion, err := resolveUpgradeVersion(repo, options.Version)
	if err != nil {
		return err
	}
	if !options.Force {
		if cmp, ok := compareSemver(Version, targetVersion); ok {
			switch {
			case cmp == 0:
				writer.printf("kiss %s is already installed; target %s is the same. Skipping upgrade.\n", Version, targetVersion)
				return writer.err
			case cmp > 0:
				writer.printf("Installed kiss %s is newer than target %s. Skipping upgrade.\n", Version, targetVersion)
				return writer.err
			default:
				writer.printf("Upgrading kiss from %s to %s.\n", Version, targetVersion)
			}
		} else {
			writer.printf("Cannot compare installed version %s with target %s; continuing with upgrade.\n", Version, targetVersion)
		}
	}

	archiveName, binaryName, err := upgradeArtifactName(targetVersion, upgradeRuntimeGOOS, upgradeRuntimeGOARCH)
	if err != nil {
		return err
	}
	targetPath, err := upgradeTargetPath()
	if err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp("", "kiss-upgrade-")
	if err != nil {
		return err
	}
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	baseURL := fmt.Sprintf("%s/%s/releases/download/%s", strings.TrimRight(githubReleaseBaseURL, "/"), repo, targetVersion)
	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadUpgradeFile(baseURL+"/checksums.txt", checksumsPath, maxGitHubAPIResponseBytes); err != nil {
		return err
	}
	expectedSum, err := checksumForArchive(checksumsPath, archiveName)
	if err != nil {
		return err
	}
	archivePath := filepath.Join(tmpDir, archiveName)
	if err := downloadUpgradeFile(baseURL+"/"+archiveName, archivePath, maxRemoteArchiveBytes); err != nil {
		return err
	}
	actualSum, err := fileSHA256(archivePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expectedSum, actualSum) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", archiveName, expectedSum, actualSum)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return err
	}
	if err := extractTarGz(archivePath, extractDir); err != nil {
		return err
	}
	newBinary := filepath.Join(extractDir, binaryName)
	if _, err := os.Stat(newBinary); err != nil {
		return fmt.Errorf("release archive did not contain %s: %w", binaryName, err)
	}
	actualVersion, err := upgradeVerifyBinary(newBinary)
	if err != nil {
		return err
	}
	if actualVersion != targetVersion {
		return fmt.Errorf("upgrade verification failed: expected %s, got %s", targetVersion, actualVersion)
	}

	keep, err := upgradeInstallBinary(newBinary, targetPath, targetVersion, out)
	if err != nil {
		return err
	}
	keepTemp = keep
	if keepTemp {
		writer.printf("Scheduled kiss upgrade to %s for %s. It will complete after this process exits.\n", targetVersion, targetPath)
	} else {
		writer.printf("Upgraded kiss to %s at %s.\n", targetVersion, targetPath)
	}
	return writer.err
}

func validateUpgradeRepo(repo string) error {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("KISS_REPO must be owner/repo, got %q", repo)
	}
	return nil
}

func resolveUpgradeVersion(repo, requested string) (string, error) {
	normalized, err := normalizeUpgradeVersion(requested)
	if err != nil {
		return "", err
	}
	if normalized != "latest" {
		return normalized, nil
	}
	latest, err := latestUpgradeVersion(repo)
	if err != nil {
		return "", err
	}
	return normalizeUpgradeVersion(latest)
}

func normalizeUpgradeVersion(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "latest" {
		return "latest", nil
	}
	match := semverPattern.FindStringSubmatch(value)
	if match == nil {
		return "", fmt.Errorf("upgrade version must be latest, vX.Y.Z, or X.Y.Z")
	}
	return "v" + match[1] + "." + match[2] + "." + match[3], nil
}

func latestUpgradeVersion(repo string) (string, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/releases/latest", strings.TrimRight(githubAPIBaseURL, "/"), repo)
	client := remoteHTTPClient(configuredRemoteTimeout("KISS_HTTP_TIMEOUT", githubAPITimeout))
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kiss-upgrade")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("resolve latest release failed: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxGitHubAPIResponseBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxGitHubAPIResponseBytes {
		return "", fmt.Errorf("GitHub latest release response exceeds maximum allowed size of %d bytes", maxGitHubAPIResponseBytes)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return "", err
	}
	if strings.TrimSpace(body.TagName) == "" {
		return "", fmt.Errorf("resolve latest release failed: response did not contain tag_name")
	}
	return body.TagName, nil
}

func compareSemver(left, right string) (int, bool) {
	leftParts, ok := parseSemver(left)
	if !ok {
		return 0, false
	}
	rightParts, ok := parseSemver(right)
	if !ok {
		return 0, false
	}
	for i := range leftParts {
		switch {
		case leftParts[i] < rightParts[i]:
			return -1, true
		case leftParts[i] > rightParts[i]:
			return 1, true
		}
	}
	return 0, true
}

func parseSemver(value string) ([3]int, bool) {
	var result [3]int
	match := semverPattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return result, false
	}
	for i := 0; i < 3; i++ {
		part, err := strconv.Atoi(match[i+1])
		if err != nil {
			return result, false
		}
		result[i] = part
	}
	return result, true
}

func upgradeArtifactName(version, goos, goarch string) (string, string, error) {
	artifactArch := ""
	switch goarch {
	case "amd64", "arm64":
		artifactArch = goarch
	default:
		return "", "", fmt.Errorf("unsupported architecture for upgrade: %s", goarch)
	}
	binaryName := "kiss"
	switch goos {
	case "darwin", "linux":
	case "windows":
		binaryName = "kiss.exe"
		if artifactArch != "amd64" {
			return "", "", fmt.Errorf("unsupported Windows architecture for upgrade: %s", goarch)
		}
	default:
		return "", "", fmt.Errorf("unsupported OS for upgrade: %s", goos)
	}
	return fmt.Sprintf("kiss_%s_%s_%s.tar.gz", version, goos, artifactArch), binaryName, nil
}

func upgradeTargetPath() (string, error) {
	executable, err := upgradeExecutablePath()
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(executable)
	if err != nil {
		return "", err
	}
	if upgradeRuntimeGOOS != "windows" {
		if resolved, err := filepath.EvalSymlinks(target); err == nil {
			target = resolved
		}
	}
	return target, nil
}

func downloadUpgradeFile(fileURL, dest string, maxBytes int64) error {
	client := remoteHTTPClient(configuredRemoteTimeout("KISS_DOWNLOAD_TIMEOUT", archiveDownloadTimeout))
	resp, err := client.Get(fileURL)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed for %s: %s", fileURL, resp.Status)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(out, io.LimitReader(resp.Body, maxBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dest)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dest)
		return closeErr
	}
	if n > maxBytes {
		_ = os.Remove(dest)
		return fmt.Errorf("download exceeds maximum allowed size of %d bytes", maxBytes)
	}
	return nil
}

func checksumForArchive(checksumsPath, archiveName string) (string, error) {
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == archiveName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("missing checksum for %s", archiveName)
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func runBinaryVersion(path string) (string, error) {
	output, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("verify downloaded kiss binary: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return firstOutputLine(output), nil
}

func firstOutputLine(output []byte) string {
	line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	return strings.TrimSpace(line)
}

func installUpgradeBinary(source, target, version string, out io.Writer) (bool, error) {
	if upgradeRuntimeGOOS == "windows" {
		return scheduleWindowsUpgrade(source, target, version)
	}
	return false, replaceExecutable(source, target)
}

func replaceExecutable(source, target string) error {
	info, err := os.Stat(target)
	mode := os.FileMode(0o755)
	if err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, ".kiss-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temporary executable next to %s: %w", target, err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := copyFile(source, tmpPath, mode); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace %s: %w", target, err)
	}
	return nil
}

func scheduleWindowsUpgrade(source, target, version string) (bool, error) {
	workDir := filepath.Dir(source)
	scriptPath := filepath.Join(workDir, "kiss-upgrade.ps1")
	if err := os.WriteFile(scriptPath, []byte(windowsUpgradeScript), 0o600); err != nil {
		return false, err
	}
	shell, err := findPowerShell()
	if err != nil {
		return false, err
	}
	cmd := exec.Command(
		shell,
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		scriptPath,
		"-Source",
		source,
		"-Target",
		target,
		"-ParentPid",
		strconv.Itoa(os.Getpid()),
		"-ExpectedVersion",
		version,
		"-WorkDir",
		workDir,
	)
	if err := cmd.Start(); err != nil {
		return false, err
	}
	return true, nil
}

func findPowerShell() (string, error) {
	for _, name := range []string{"powershell.exe", "powershell", "pwsh.exe", "pwsh"} {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("powershell is required to complete Windows upgrade")
}

const windowsUpgradeScript = `param(
    [string]$Source,
    [string]$Target,
    [int]$ParentPid,
    [string]$ExpectedVersion,
    [string]$WorkDir
)

$ErrorActionPreference = "Stop"

while (Get-Process -Id $ParentPid -ErrorAction SilentlyContinue) {
    Start-Sleep -Milliseconds 200
}

Move-Item -Force -Path $Source -Destination $Target

$actual = (& $Target --version | Select-Object -First 1).Trim()
if ($actual -ne $ExpectedVersion) {
    throw "upgrade verification failed: expected $ExpectedVersion, got $actual"
}

Remove-Item -Recurse -Force $WorkDir -ErrorAction SilentlyContinue
`
