package kiss

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeUpgradeVersion(t *testing.T) {
	for input, want := range map[string]string{
		"":       "latest",
		"latest": "latest",
		"0.1.2":  "v0.1.2",
		"v0.1.2": "v0.1.2",
	} {
		got, err := normalizeUpgradeVersion(input)
		if err != nil {
			t.Fatalf("normalize %q: %v", input, err)
		}
		if got != want {
			t.Fatalf("normalize %q = %q, want %q", input, got, want)
		}
	}
	if _, err := normalizeUpgradeVersion("1.2"); err == nil {
		t.Fatal("expected invalid version rejection")
	}
}

func TestCompareSemver(t *testing.T) {
	if got, ok := compareSemver("v1.2.3", "v1.2.4"); !ok || got != -1 {
		t.Fatalf("unexpected compare result got=%d ok=%v", got, ok)
	}
	if got, ok := compareSemver("v1.3.0", "v1.2.9"); !ok || got != 1 {
		t.Fatalf("unexpected compare result got=%d ok=%v", got, ok)
	}
	if got, ok := compareSemver("v1.2.3", "1.2.3"); !ok || got != 0 {
		t.Fatalf("unexpected compare result got=%d ok=%v", got, ok)
	}
	if _, ok := compareSemver("dev", "v1.2.3"); ok {
		t.Fatal("expected dev version to be incomparable")
	}
}

func TestUpgradeArtifactName(t *testing.T) {
	archive, binary, err := upgradeArtifactName("v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if archive != "kiss_v1.2.3_linux_amd64.tar.gz" || binary != "kiss" {
		t.Fatalf("unexpected linux artifact: %q %q", archive, binary)
	}
	archive, binary, err = upgradeArtifactName("v1.2.3", "windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if archive != "kiss_v1.2.3_windows_amd64.tar.gz" || binary != "kiss.exe" {
		t.Fatalf("unexpected windows artifact: %q %q", archive, binary)
	}
	if _, _, err := upgradeArtifactName("v1.2.3", "windows", "arm64"); err == nil {
		t.Fatal("expected unsupported windows arm64")
	}
}

func TestUpgradeCLIDownloadsVerifiesAndInstallsLatest(t *testing.T) {
	version := "v9.8.7"
	archiveName := "kiss_" + version + "_linux_amd64.tar.gz"
	archive := makeUpgradeArchive(t, "kiss", []byte("fake kiss binary"))
	sum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%x  %s\n", sum, archiveName)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/wwulfric/kiss/releases/latest":
			_, _ = io.WriteString(w, `{"tag_name":"`+version+`"}`)
		case "/wwulfric/kiss/releases/download/" + version + "/" + archiveName:
			_, _ = w.Write(archive)
		case "/wwulfric/kiss/releases/download/" + version + "/checksums.txt":
			_, _ = io.WriteString(w, checksums)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	restore := stubUpgradeEnvironment(t)
	defer restore()
	githubAPIBaseURL = server.URL
	githubReleaseBaseURL = server.URL
	upgradeRuntimeGOOS = "linux"
	upgradeRuntimeGOARCH = "amd64"
	Version = "v1.0.0"
	targetPath := filepath.Join(t.TempDir(), "kiss")
	upgradeExecutablePath = func() (string, error) { return targetPath, nil }
	upgradeVerifyBinary = func(path string) (string, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if string(data) != "fake kiss binary" {
			return "", fmt.Errorf("unexpected downloaded binary content: %q", string(data))
		}
		return version, nil
	}
	var installedSource, installedTarget, installedVersion string
	upgradeInstallBinary = func(source, target, version string, out io.Writer) (bool, error) {
		installedSource = source
		installedTarget = target
		installedVersion = version
		return false, nil
	}

	var out bytes.Buffer
	if err := UpgradeCLI(UpgradeOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	if installedVersion != version || installedTarget != targetPath || installedSource == "" {
		t.Fatalf("upgrade did not install expected binary: source=%q target=%q version=%q", installedSource, installedTarget, installedVersion)
	}
	if !strings.Contains(out.String(), "Upgrading kiss from v1.0.0 to "+version) || !strings.Contains(out.String(), "Upgraded kiss to "+version) {
		t.Fatalf("unexpected upgrade output:\n%s", out.String())
	}
}

func TestUpgradeCLISkipsSameVersion(t *testing.T) {
	restore := stubUpgradeEnvironment(t)
	defer restore()
	Version = "v1.2.3"
	installed := false
	upgradeInstallBinary = func(source, target, version string, out io.Writer) (bool, error) {
		installed = true
		return false, nil
	}
	var out bytes.Buffer
	if err := UpgradeCLI(UpgradeOptions{Version: "v1.2.3"}, &out); err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("same version should not install")
	}
	if !strings.Contains(out.String(), "already installed") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func stubUpgradeEnvironment(t *testing.T) func() {
	t.Helper()
	oldAPIBase := githubAPIBaseURL
	oldReleaseBase := githubReleaseBaseURL
	oldGOOS := upgradeRuntimeGOOS
	oldGOARCH := upgradeRuntimeGOARCH
	oldExecutable := upgradeExecutablePath
	oldVerify := upgradeVerifyBinary
	oldInstall := upgradeInstallBinary
	oldVersion := Version
	return func() {
		githubAPIBaseURL = oldAPIBase
		githubReleaseBaseURL = oldReleaseBase
		upgradeRuntimeGOOS = oldGOOS
		upgradeRuntimeGOARCH = oldGOARCH
		upgradeExecutablePath = oldExecutable
		upgradeVerifyBinary = oldVerify
		upgradeInstallBinary = oldInstall
		Version = oldVersion
	}
}

func makeUpgradeArchive(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
