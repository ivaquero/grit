package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// 本地改动被丢弃，未跟踪文件保留 —— 与原来的 upox 行为一致。
func TestUpdateResetsLocalChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	base := t.TempDir()
	origin := filepath.Join(base, "origin")
	work := filepath.Join(base, "work")

	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, origin, "init", "-b", "master")
	writeFile(t, filepath.Join(origin, "a.txt"), "v1")
	gitCmd(t, origin, "add", "-A")
	gitCmd(t, origin, "commit", "-m", "init")

	gitCmd(t, base, "clone", origin, work)

	writeFile(t, filepath.Join(work, "a.txt"), "dirty")
	writeFile(t, filepath.Join(work, "keep.txt"), "untracked")

	if err := Run(Options{Root: work, NoPlugins: true}); err != nil {
		t.Fatalf("self update: %v", err)
	}

	if got := readFile(t, filepath.Join(work, "a.txt")); got != "v1" {
		t.Fatalf("local change survived reset: %q", got)
	}
	if _, err := os.Stat(filepath.Join(work, "keep.txt")); err != nil {
		t.Fatal("untracked file should have been kept")
	}
}

func TestNotAGitRepo(t *testing.T) {
	if err := Run(Options{Root: t.TempDir()}); err == nil {
		t.Fatal("expected an error for a non-repository directory")
	}
}

func TestEmptyRoot(t *testing.T) {
	if err := Run(Options{}); err == nil {
		t.Fatal("expected an error for an empty root")
	}
}

func TestRemoteSelection(t *testing.T) {
	pwsh := Remote("pwsh")
	posix := Remote("bash")
	if pwsh == posix {
		t.Fatal("pwsh and posix must use different plugin repositories")
	}
	if posix != Remote("zsh") {
		t.Fatal("all posix shells should share one repository")
	}
}
