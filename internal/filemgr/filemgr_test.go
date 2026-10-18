package filemgr

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/ivaquero/goboot/internal/ospath"
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

// fakeDefaults 模拟二进制内嵌的默认模板。
func fakeDefaults() fstest.MapFS {
	return fstest.MapFS{
		"defaults/cfg": &fstest.MapFile{Data: []byte("defaults")},
	}
}

// newFileSet 造一个隔离的四张表：home/cfg 是"真实配置"，defaults/cfg 是"默认模板"。
func newFileSet(t *testing.T) Set {
	t.Helper()
	root := t.TempDir()
	s := Set{
		Element:  map[string]string{"cfg": filepath.Join(root, "home", "cfg")},
		Oxide:    map[string]string{"bkcfg": "shell/cfg"},
		Oxygen:   map[string]string{"gbcfg": "defaults/cfg"},
		Backup:   filepath.Join(root, "backup"),
		Defaults: fakeDefaults(),
	}
	for _, d := range []string{filepath.Dir(s.Element["cfg"]), s.Backup} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func backupPath(s Set) string {
	return ospath.Join(s.Backup, "shell/cfg", s.IsWin)
}

func runFile(t *testing.T, s Set, a Action, keys ...string) error {
	t.Helper()
	return Run(s, a, keys, Options{Out: io.Discard})
}

func TestBackup(t *testing.T) {
	s := newFileSet(t)
	writeFile(t, s.Element["cfg"], "live-config")

	if err := runFile(t, s, Backup, "cfg"); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if got := readFile(t, backupPath(s)); got != "live-config" {
		t.Fatalf("backup content = %q", got)
	}
}

func TestRestore(t *testing.T) {
	s := newFileSet(t)
	writeFile(t, backupPath(s), "from-backup")
	writeFile(t, s.Element["cfg"], "stale")

	if err := runFile(t, s, Restore, "cfg"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := readFile(t, s.Element["cfg"]); got != "from-backup" {
		t.Fatalf("restored content = %q", got)
	}
}

func TestCatalyze(t *testing.T) {
	s := newFileSet(t)
	writeFile(t, s.Element["cfg"], "stale")

	if err := runFile(t, s, Catalyze, "cfg"); err != nil {
		t.Fatalf("catalyze: %v", err)
	}
	if got := readFile(t, s.Element["cfg"]); got != "defaults" {
		t.Fatalf("catalyzed content = %q", got)
	}
}

func TestPropagate(t *testing.T) {
	s := newFileSet(t)

	if err := runFile(t, s, Propagate, "cfg"); err != nil {
		t.Fatalf("propagate: %v", err)
	}
	if got := readFile(t, backupPath(s)); got != "defaults" {
		t.Fatalf("propagated content = %q", got)
	}
}

// 内嵌模板写出去之后必须可编辑：embed.FS 报的权限是只读。
func TestCatalyzeProducesWritableFile(t *testing.T) {
	s := newFileSet(t)
	if err := runFile(t, s, Catalyze, "cfg"); err != nil {
		t.Fatalf("catalyze: %v", err)
	}
	f, err := os.OpenFile(s.Element["cfg"], os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatalf("result is not writable: %v", err)
	}
	f.Close()
}

// 目录的目标位置必须先清空，否则旧文件会残留。
func TestDirectoryCopyClearsDestination(t *testing.T) {
	s := newFileSet(t)
	writeFile(t, filepath.Join(s.Element["cfg"], "keep.txt"), "new")

	dir := backupPath(s)
	writeFile(t, filepath.Join(dir, "stale.txt"), "old")

	if err := runFile(t, s, Backup, "cfg"); err != nil {
		t.Fatalf("backup dir: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "keep.txt")); got != "new" {
		t.Fatalf("copied content = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "stale.txt")); !os.IsNotExist(err) {
		t.Fatal("stale file survived the directory copy")
	}
}

// 单个 key 失败不能中断其余 key。
func TestFailureIsIsolated(t *testing.T) {
	s := newFileSet(t)
	writeFile(t, s.Element["cfg"], "ok")
	s.Element["broken"] = filepath.Join(t.TempDir(), "does-not-exist")
	s.Oxide["bkbroken"] = "shell/broken"

	err := runFile(t, s, Backup, "broken", "cfg")
	if err == nil {
		t.Fatal("expected an error for the broken key")
	}
	if got := readFile(t, backupPath(s)); got != "ok" {
		t.Fatalf("second key was not processed: %q", got)
	}
}

// 未配置的 key 只跳过，不算失败。
func TestMissingEntryIsSkip(t *testing.T) {
	s := newFileSet(t)
	if err := runFile(t, s, Backup, "nope"); err != nil {
		t.Fatalf("missing entry should not fail: %v", err)
	}
}

// 绝不能把整个主目录当作目标做 RemoveAll。
func TestGuardRejectsHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	s := Set{
		Element:  map[string]string{"cfg": home},
		Oxide:    map[string]string{"bkcfg": "x"},
		Oxygen:   map[string]string{"gbcfg": "defaults/cfg"},
		Backup:   t.TempDir(),
		Defaults: fakeDefaults(),
	}
	if err := runFile(t, s, Catalyze, "cfg"); err == nil {
		t.Fatal("expected guard to reject the home directory as a destination")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	s := newFileSet(t)
	writeFile(t, s.Element["cfg"], "live")

	if err := Run(s, Backup, []string{"cfg"}, Options{DryRun: true, Out: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backupPath(s)); !os.IsNotExist(err) {
		t.Fatal("dry run created a file")
	}
}

func TestUnknownAction(t *testing.T) {
	if Valid("frobnicate") {
		t.Fatal("unknown action accepted")
	}
	for _, a := range []Action{Backup, Restore, Catalyze, Propagate} {
		if !Valid(a) {
			t.Fatalf("%s should be valid", a)
		}
	}
}

// 内嵌模板缺失时要报错，而不是静默成功。
func TestCatalyzeMissingTemplate(t *testing.T) {
	s := newFileSet(t)
	s.Defaults = nil
	if err := runFile(t, s, Catalyze, "cfg"); err == nil {
		t.Fatal("expected an error when defaults are unavailable")
	}
}
