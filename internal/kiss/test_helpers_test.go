package kiss

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeSkill(t *testing.T, name, _ string, instructions string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(instructions+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func withGitHubTestServer(t *testing.T, sha string, archive []byte, archivePath *string) func() {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
		case strings.Contains(r.URL.Path, "/tar.gz/"):
			if archivePath != nil {
				*archivePath = r.URL.Path
			}
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	oldTransport := remoteHTTPTransport
	oldAPIBase := githubAPIBaseURL
	oldCodeloadBase := githubCodeloadBaseURL
	remoteHTTPTransport = server.Client().Transport
	githubAPIBaseURL = server.URL
	githubCodeloadBaseURL = server.URL
	return func() {
		remoteHTTPTransport = oldTransport
		githubAPIBaseURL = oldAPIBase
		githubCodeloadBaseURL = oldCodeloadBase
		server.Close()
	}
}

type githubRefArchive struct {
	sha     string
	archive []byte
}

func withGitHubArchiveByRefTestServer(t *testing.T, refs map[string]githubRefArchive, archivePath *string) func() {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			ref := pathBase(r.URL.Path)
			entry, ok := refs[ref]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sha":"` + entry.sha + `"}`))
		case strings.Contains(r.URL.Path, "/tar.gz/"):
			if archivePath != nil {
				*archivePath = r.URL.Path
			}
			sha := pathBase(r.URL.Path)
			for _, entry := range refs {
				if entry.sha == sha {
					_, _ = w.Write(entry.archive)
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	oldTransport := remoteHTTPTransport
	oldAPIBase := githubAPIBaseURL
	oldCodeloadBase := githubCodeloadBaseURL
	remoteHTTPTransport = server.Client().Transport
	githubAPIBaseURL = server.URL
	githubCodeloadBaseURL = server.URL
	return func() {
		remoteHTTPTransport = oldTransport
		githubAPIBaseURL = oldAPIBase
		githubCodeloadBaseURL = oldCodeloadBase
		server.Close()
	}
}

func pathBase(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failingReadCloser struct {
	data    []byte
	read    bool
	failErr error
}

func (f *failingReadCloser) Read(p []byte) (int, error) {
	if !f.read {
		f.read = true
		n := copy(p, f.data)
		return n, nil
	}
	return 0, f.failErr
}

func (f *failingReadCloser) Close() error {
	return nil
}

func makeSkillArchive(t *testing.T, root, name, _ string, instructions string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	addDir := func(path string) {
		t.Helper()
		if err := tw.WriteHeader(&tar.Header{Name: path, Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
			t.Fatal(err)
		}
	}
	addFile := func(path, body string) {
		t.Helper()
		if err := tw.WriteHeader(&tar.Header{Name: path, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	addDir(root + "/")
	addFile(root+"/SKILL.md", instructions+"\n")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeNestedSkillArchive(t *testing.T, root, subdir, name, _ string, instructions string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	addDir := func(path string) {
		t.Helper()
		if err := tw.WriteHeader(&tar.Header{Name: path, Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
			t.Fatal(err)
		}
	}
	addFile := func(path, body string) {
		t.Helper()
		if err := tw.WriteHeader(&tar.Header{Name: path, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	addDir(root + "/")
	current := root
	for _, segment := range strings.Split(subdir, "/") {
		current += "/" + segment
		addDir(current + "/")
	}
	addFile(current+"/SKILL.md", instructions+"\n")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
