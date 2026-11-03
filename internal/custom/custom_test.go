package custom

import (
	"os"
	"path/filepath"
	"testing"
)

// custom.Load 是全项目唯一读取用户配置的入口：文件缺失必须是「空配置而非错误」，
// 否则没配过 custom.toml 的新用户第一次 `grit init` 就会被卡住。
func TestLoadMissingFileReturnsEmptyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.toml")
	c, err := Load(path)
	if err != nil {
		t.Fatalf("a missing config must not error: %v", err)
	}
	if c.Path != path {
		t.Fatalf("Path = %q, want %q", c.Path, path)
	}
	// init() 必须把三张表建成非 nil，调用方才能直接读、直接写。
	if c.Oxides == nil || c.PluginsPlus == nil || c.ProxyPorts == nil {
		t.Fatalf("maps must be initialized: %+v", c)
	}
}

func TestLoadValidAndParse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.toml")
	body := "" +
		"oxide_folder = \"~/backup\"\n" +
		"oxide_folder_win = \"D:/backup\"\n" +
		"[proxy_ports]\nc = 7890\n" +
		"[env.julia]\nt = \"proj\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.OxideFolder != "~/backup" {
		t.Fatalf("oxide_folder = %q", c.OxideFolder)
	}
	if c.ProxyPorts["c"] != 7890 {
		t.Fatalf("proxy_ports = %+v", c.ProxyPorts)
	}
	if c.Env.Julia["t"] != "proj" {
		t.Fatalf("env.julia = %+v", c.Env.Julia)
	}
}

// 坏 TOML 必须报错并带上路径，不能静默当成空配置——用户会以为改动生效了。
func TestLoadCorruptFileReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.toml")
	if err := os.WriteFile(path, []byte("not = valid = toml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected a parse error for malformed TOML")
	}
}

func TestBackupRootFallsBackToUnix(t *testing.T) {
	c := &Config{OxideFolder: "~/backup"}
	if got := c.BackupRoot(true); got != "~/backup" {
		t.Fatalf("win with empty oxide_folder_win should fall back: %q", got)
	}
	c.OxideFolderWin = "D:/backup"
	if got := c.BackupRoot(true); got != "D:/backup" {
		t.Fatalf("win should prefer oxide_folder_win: %q", got)
	}
	if got := c.BackupRoot(false); got != "~/backup" {
		t.Fatalf("non-win must ignore oxide_folder_win: %q", got)
	}
}

// 只有 Julia 的短名被真正消费；conda 分支只是兼容读取，julia_win 需非空才覆盖 julia。
func TestEnvShortcuts(t *testing.T) {
	c := &Config{Env: Env{
		Conda:    map[string]string{"k": "kaggle"},
		Julia:    map[string]string{"t": "linux-proj"},
		JuliaWin: map[string]string{"t": "win-proj"},
	}}
	if got := c.EnvShortcuts("conda", false)["k"]; got != "kaggle" {
		t.Fatalf("conda shortcut: %q", got)
	}
	if got := c.EnvShortcuts("julia", true)["t"]; got != "win-proj" {
		t.Fatalf("julia on win should use julia_win: %q", got)
	}
	if got := c.EnvShortcuts("julia", false)["t"]; got != "linux-proj" {
		t.Fatalf("julia on unix: %q", got)
	}
	if c.EnvShortcuts("mystery", false) != nil {
		t.Fatal("unknown kind must return nil")
	}

	// julia_win 为空时，Windows 上回退到 julia。
	fallback := &Config{Env: Env{Julia: map[string]string{"t": "p"}}}
	if got := fallback.EnvShortcuts("julia", true)["t"]; got != "p" {
		t.Fatalf("empty julia_win should fall back to julia: %q", got)
	}
}

// Marshal → Load 往返必须稳定，且 Path 不参与序列化（toml:"-"）。
func TestMarshalRoundTrip(t *testing.T) {
	c := &Config{
		OxideFolder: "~/backup",
		Oxides:      map[string]string{"bkzs": "shell/.zshrc"},
		ProxyPorts:  map[string]int{"c": 7890},
		Path:        "/should/not/be/serialized",
	}
	b, err := c.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.toml")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatalf("reload failed: %v\n%s", err, b)
	}
	if back.OxideFolder != "~/backup" || back.Oxides["bkzs"] != "shell/.zshrc" || back.ProxyPorts["c"] != 7890 {
		t.Fatalf("round-trip mismatch: %+v", back)
	}
}
