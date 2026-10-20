package vscode

import (
	"os"
	"path/filepath"
	"testing"
)

// fixture 造一套与真实数据目录同形的缓存。
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, p := range []string{
		"Cache/x", "CachedData/y", "User/History/-h", "User/profiles/-p", "User/workspaceStorage/w",
	} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(full, "data"), []byte("xx"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// workspaceStorage 只有在 -a 时才动。
func TestCleanGatesWorkspaceStorage(t *testing.T) {
	dir := fixture(t)
	entries, err := Clean(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(entries))
	}
	if !exists(filepath.Join(dir, "User", "workspaceStorage", "w")) {
		t.Fatal("workspaceStorage must survive without -a")
	}
	if exists(filepath.Join(dir, "Cache", "x")) {
		t.Fatal("cache should be gone")
	}
}

func TestCleanAll(t *testing.T) {
	dir := fixture(t)
	entries, err := Clean(Options{DataDir: dir, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("got %d entries, want 5", len(entries))
	}
	for _, e := range entries {
		if e.Err != nil {
			t.Fatalf("%s: %v", e.Path, e.Err)
		}
		if exists(e.Path) {
			t.Fatalf("%s survived", e.Path)
		}
	}
}

func TestDryRunDeletesNothing(t *testing.T) {
	dir := fixture(t)
	entries, err := Clean(Options{DataDir: dir, All: true, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("got %d entries", len(entries))
	}
	for _, e := range entries {
		if !exists(e.Path) {
			t.Fatalf("%s was removed during a dry run", e.Path)
		}
		if e.Size == 0 {
			t.Fatalf("%s: size not reported", e.Path)
		}
	}
}

func TestMissingDataDir(t *testing.T) {
	if _, err := Clean(Options{DataDir: filepath.Join(t.TempDir(), "nope")}); err == nil {
		t.Fatal("expected an error for a missing data directory")
	}
}

// guard 只做字符串判断，可以在不触碰磁盘的前提下验证。
func TestGuard(t *testing.T) {
	if err := guard(t.TempDir()); err != nil {
		t.Fatalf("a temp dir should pass: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	if guard(home) == nil {
		t.Fatal("home directory must be rejected")
	}
}

func TestResolveDataDirExplicit(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveDataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("got %q want %q", got, abs)
	}
}
