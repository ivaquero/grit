package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func tarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
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

func zipArchive(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type recorder struct {
	mu    sync.Mutex
	paths map[string]int
}

func (r *recorder) add(p string) {
	r.mu.Lock()
	r.paths[p]++
	r.mu.Unlock()
}

func (r *recorder) get(p string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.paths[p]
}

// newServer 模拟 release 列表、tag 端点与两个下载资产。
func newServer(t *testing.T, tag, assetName string, archive []byte, sums string) (*httptest.Server, *recorder) {
	t.Helper()
	rec := &recorder{paths: map[string]int{}}

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r.URL.Path)
		base := "http://" + r.Host
		rel := map[string]any{
			"tag_name": tag,
			"assets": []map[string]string{
				{"name": assetName, "browser_download_url": base + "/dl/" + assetName},
				{"name": "sha256sums.txt", "browser_download_url": base + "/dl/sha256sums.txt"},
			},
		}

		switch {
		case r.URL.Path == "/repos/o/r/releases":
			if err := json.NewEncoder(w).Encode([]any{rel}); err != nil {
				t.Errorf("encode list: %v", err)
			}
		case strings.HasPrefix(r.URL.Path, "/repos/o/r/releases/tags/"):
			if err := json.NewEncoder(w).Encode(rel); err != nil {
				t.Errorf("encode tag: %v", err)
			}
		case r.URL.Path == "/dl/"+assetName:
			if _, err := w.Write(archive); err != nil {
				t.Errorf("write archive: %v", err)
			}
		case r.URL.Path == "/dl/sha256sums.txt":
			if _, err := w.Write([]byte(sums)); err != nil {
				t.Errorf("write sums: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func sumsFor(name string, archive []byte) string {
	return fmt.Sprintf("%s  %s\n", sha256Hex(archive), name)
}

func newExecutable(t *testing.T, name, content string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(exe, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0-dev", "0.1.0", -1},
		{"0.1.0", "0.1.0", 0},
		{"v1.2.3", "1.2.3", 0},
		{"1.2", "1.2.0", 0},
		{"1.10.0", "1.9.9", 1},
		{"1.2.3-rc1", "1.2.3", -1},
		{"1.2.3", "1.2.3-next", 1},
		{"dev", "1.0.0", -1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Fatalf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	cases := map[string]string{
		"linux/amd64":   "grit_1.2.3_linux_amd64.tar.gz",
		"darwin/arm64":  "grit_1.2.3_darwin_arm64.tar.gz",
		"windows/amd64": "grit_1.2.3_windows_amd64.zip",
	}
	for key, want := range cases {
		parts := strings.Split(key, "/")
		if got := assetName("v1.2.3", parts[0], parts[1]); got != want {
			t.Fatalf("%s: got %q want %q", key, got, want)
		}
	}
}

func TestRunInstallsRelease(t *testing.T) {
	const name = "grit_1.2.3_linux_amd64.tar.gz"
	archive := tarGz(t, "grit", []byte("new-binary"))
	srv, rec := newServer(t, "v1.2.3", name, archive, sumsFor(name, archive))

	exe := newExecutable(t, "grit", "old-binary")

	err := Run(Options{
		Repo:       "o/r",
		Current:    "1.0.0",
		Out:        io.Discard,
		APIBase:    srv.URL,
		GOOS:       "linux",
		GOARCH:     "amd64",
		Executable: exe,
		Client:     srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, exe); got != "new-binary" {
		t.Fatalf("binary was not replaced: %q", got)
	}
	if rec.get("/dl/"+name) != 1 {
		t.Fatal("archive was not downloaded exactly once")
	}
}

// Windows 用 zip，且替换逻辑要走「先改名再落位」的分支。
func TestRunInstallsZip(t *testing.T) {
	const name = "grit_1.2.3_windows_amd64.zip"
	archive := zipArchive(t, "grit.exe", []byte("new-exe"))
	srv, _ := newServer(t, "v1.2.3", name, archive, sumsFor(name, archive))

	exe := newExecutable(t, "grit.exe", "old-exe")

	err := Run(Options{
		Repo:       "o/r",
		Current:    "1.0.0",
		Out:        io.Discard,
		APIBase:    srv.URL,
		GOOS:       "windows",
		GOARCH:     "amd64",
		Executable: exe,
		Client:     srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, exe); got != "new-exe" {
		t.Fatalf("binary was not replaced: %q", got)
	}
}

// 已是最新版本时不应该下载归档。
func TestRunSkipsWhenUpToDate(t *testing.T) {
	const name = "grit_1.2.3_linux_amd64.tar.gz"
	archive := tarGz(t, "grit", []byte("new"))
	srv, rec := newServer(t, "v1.2.3", name, archive, sumsFor(name, archive))

	exe := newExecutable(t, "grit", "old")
	err := Run(Options{
		Repo: "o/r", Current: "1.2.3", Out: io.Discard,
		APIBase: srv.URL, GOOS: "linux", GOARCH: "amd64",
		Executable: exe, Client: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.get("/dl/"+name) != 0 {
		t.Fatal("archive should not be downloaded when up to date")
	}
	if got := readFile(t, exe); got != "old" {
		t.Fatalf("binary changed: %q", got)
	}
}

// 校验和不匹配必须拒绝安装，且不能动旧二进制。
func TestRunRejectsBadChecksum(t *testing.T) {
	const name = "grit_1.2.3_linux_amd64.tar.gz"
	archive := tarGz(t, "grit", []byte("new"))
	srv, _ := newServer(t, "v1.2.3", name, archive, strings.Repeat("0", 64)+"  "+name+"\n")

	exe := newExecutable(t, "grit", "old")
	err := Run(Options{
		Repo: "o/r", Current: "1.0.0", Out: io.Discard,
		APIBase: srv.URL, GOOS: "linux", GOARCH: "amd64",
		Executable: exe, Client: srv.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected a checksum error, got %v", err)
	}
	if got := readFile(t, exe); got != "old" {
		t.Fatalf("binary changed despite a bad checksum: %q", got)
	}
}

func TestRunDryRun(t *testing.T) {
	const name = "grit_1.2.3_linux_amd64.tar.gz"
	archive := tarGz(t, "grit", []byte("new"))
	srv, rec := newServer(t, "v1.2.3", name, archive, sumsFor(name, archive))

	exe := newExecutable(t, "grit", "old")
	var out bytes.Buffer
	err := Run(Options{
		Repo: "o/r", Current: "1.0.0", DryRun: true, Out: &out,
		APIBase: srv.URL, GOOS: "linux", GOARCH: "amd64",
		Executable: exe, Client: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[dry]") {
		t.Fatalf("output = %q", out.String())
	}
	if rec.get("/dl/"+name) != 0 {
		t.Fatal("dry run downloaded the archive")
	}
	if got := readFile(t, exe); got != "old" {
		t.Fatalf("dry run changed the binary: %q", got)
	}
}

// 指定版本走 tag 端点，不拉列表。
func TestRunSpecificVersion(t *testing.T) {
	const name = "grit_1.2.3_linux_amd64.tar.gz"
	archive := tarGz(t, "grit", []byte("new"))
	srv, rec := newServer(t, "v1.2.3", name, archive, sumsFor(name, archive))

	exe := newExecutable(t, "grit", "old")
	err := Run(Options{
		Repo: "o/r", Version: "1.2.3", Current: "9.9.9", Force: true, Out: io.Discard,
		APIBase: srv.URL, GOOS: "linux", GOARCH: "amd64",
		Executable: exe, Client: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.get("/repos/o/r/releases/tags/v1.2.3") != 1 {
		t.Fatal("tag endpoint was not used")
	}
	if got := readFile(t, exe); got != "new" {
		t.Fatalf("binary was not replaced: %q", got)
	}
}

func TestParseChecksums(t *testing.T) {
	body := []byte(strings.Join([]string{
		"ABCDEF  grit_1.0.0_linux_amd64.tar.gz",
		"012345 *grit_1.0.0_windows_amd64.zip",
		"malformed-line",
		"",
	}, "\n"))
	got := parseChecksums(body)
	if got["grit_1.0.0_linux_amd64.tar.gz"] != "abcdef" {
		t.Fatalf("hash not normalized: %v", got)
	}
	if got["grit_1.0.0_windows_amd64.zip"] != "012345" {
		t.Fatalf("binary marker not stripped: %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("malformed lines should be ignored: %v", got)
	}
}

func TestResolveRepo(t *testing.T) {
	t.Setenv("GRIT_REPO", "")
	if got, err := resolveRepo(""); err != nil || got != defaultRepo {
		t.Fatalf("default: %q %v", got, err)
	}
	t.Setenv("GRIT_REPO", "fork/grit")
	if got, err := resolveRepo(""); err != nil || got != "fork/grit" {
		t.Fatalf("env: %q %v", got, err)
	}
	for _, bad := range []string{"../x", "a/b/c", "https://github.com/a/b", ""} {
		if bad == "" {
			continue
		}
		if _, err := resolveRepo(bad); err == nil {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}

// 没有正式版时给出明确错误，而不是静默成功。
func TestNoStableRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0.0-rc1","prerelease":true},{"tag_name":"nightly"}]`))
	}))
	defer srv.Close()

	err := Run(Options{
		Repo: "o/r", Current: "0.0.0", Out: io.Discard,
		APIBase: srv.URL, Client: srv.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "no stable release") {
		t.Fatalf("got %v", err)
	}
}

// 指定版本不存在时要把 404 翻译成可读信息，而不是甩一大段 JSON。
func TestMissingTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer srv.Close()

	err := Run(Options{
		Repo: "o/r", Version: "9.9.9", Current: "0.0.0", Out: io.Discard,
		APIBase: srv.URL, Client: srv.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "release v9.9.9 not found in o/r") {
		t.Fatalf("got %v", err)
	}
}

// 归档里没有可执行文件时报错，不能写出空文件。
func TestExtractBinaryMissing(t *testing.T) {
	archive := tarGz(t, "README.md", []byte("docs"))
	if _, err := extractBinary(archive, "linux"); err == nil {
		t.Fatal("expected an error for an archive without grit")
	}
	if _, err := extractBinary([]byte("not an archive"), "linux"); err == nil {
		t.Fatal("expected an error for a corrupt archive")
	}
}

func TestCleanupOld(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "grit")
	for _, name := range []string{"grit.old", "grit.old-1234", "grit", "keep.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cleanupOld(exe)

	for _, name := range []string{"grit.old", "grit.old-1234"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s should have been removed", name)
		}
	}
	for _, name := range []string{"grit", "keep.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s should have been kept", name)
		}
	}
}

// .old 被占用删不掉时（这里是不可删的目录），更新仍要成功。
func TestSwapSurvivesStaleOld(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "grit.exe")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(exe+".old", "block")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fresh := filepath.Join(dir, "fresh")
	if err := os.WriteFile(fresh, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := swap(exe, fresh); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, exe); got != "new" {
		t.Fatalf("binary was not replaced: %q", got)
	}
}
