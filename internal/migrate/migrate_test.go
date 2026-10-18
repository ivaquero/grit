package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivaquero/goboot/internal/custom"
)

const legacyJSON = `{
  "description": {"oxide_folder": "backup root folder"},
  "plugins_load": ["cli_espanso", "lang_rust"],
  "plugins_plus": {},
  "plugins_load_plus": [],
  "proxy_ports": {"c": 7890, "m": 7898, "v": 1080},
  "conda_env_shortcuts": {"b": "base", "k": "kaggle"},
  "julia_env_shortcuts": {"t": "Documents/GitHub/blog-julia/pluto"},
  "julia_env_shortcuts_win": {"t": "D:\\GitHub\\blog-julia\\pluto"},
  "startup_folder": "",
  "download_folder": "Documents",
  "oxide_folder": "Documents/Github/backup",
  "oxide_folder_win": "D:\\Github\\backup",
  "oxides": {"bkox": "../notes/private/custom.json", "bkzs": "shell/.zshrc"}
}`

func TestFromJSON(t *testing.T) {
	c, err := FromJSON([]byte(legacyJSON))
	if err != nil {
		t.Fatal(err)
	}
	if c.OxideFolderWin != "D:/Github/backup" {
		t.Fatalf("oxide_folder_win not normalized: %q", c.OxideFolderWin)
	}
	if got := c.Env.JuliaWin["t"]; got != "D:/GitHub/blog-julia/pluto" {
		t.Fatalf("julia_env_shortcuts_win not normalized: %q", got)
	}
	if c.Env.Conda["k"] != "kaggle" {
		t.Fatalf("conda shortcuts lost: %v", c.Env.Conda)
	}
	if c.ProxyPorts["m"] != 7898 {
		t.Fatalf("proxy ports lost: %v", c.ProxyPorts)
	}
	if len(c.PluginsLoad) != 2 {
		t.Fatalf("plugins_load lost: %v", c.PluginsLoad)
	}
}

// 迁移必须无损：写出去的 TOML 读回来要和转换结果一致。
func TestRoundTripThroughTOML(t *testing.T) {
	src, err := FromJSON([]byte(legacyJSON))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Render(src)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "custom.toml")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	back, err := custom.Load(path)
	if err != nil {
		t.Fatalf("re-read failed: %v\n%s", err, out)
	}

	if back.OxideFolder != src.OxideFolder {
		t.Errorf("oxide_folder: %q != %q", back.OxideFolder, src.OxideFolder)
	}
	if back.OxideFolderWin != src.OxideFolderWin {
		t.Errorf("oxide_folder_win: %q != %q", back.OxideFolderWin, src.OxideFolderWin)
	}
	if back.DownloadFolder != src.DownloadFolder {
		t.Errorf("download_folder: %q != %q", back.DownloadFolder, src.DownloadFolder)
	}
	if len(back.Oxides) != len(src.Oxides) {
		t.Errorf("oxides size: %d != %d", len(back.Oxides), len(src.Oxides))
	}
	for k, v := range src.Oxides {
		if back.Oxides[k] != v {
			t.Errorf("oxides[%s]: %q != %q", k, back.Oxides[k], v)
		}
	}
	if len(back.PluginsLoad) != len(src.PluginsLoad) {
		t.Errorf("plugins_load: %v != %v", back.PluginsLoad, src.PluginsLoad)
	}
	if back.ProxyPorts["v"] != 1080 {
		t.Errorf("proxy_ports: %v", back.ProxyPorts)
	}
	if back.Env.Conda["b"] != "base" {
		t.Errorf("env.conda: %v", back.Env.Conda)
	}
	if back.Env.Julia["t"] != src.Env.Julia["t"] {
		t.Errorf("env.julia: %v", back.Env.Julia)
	}
	if back.Env.JuliaWin["t"] != src.Env.JuliaWin["t"] {
		t.Errorf("env.julia_win: %v", back.Env.JuliaWin)
	}
}

// 已经是正斜杠的路径不能被改动。
func TestSlashIsIdempotent(t *testing.T) {
	c, err := FromJSON([]byte(`{"oxide_folder": "Documents/backup"}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.OxideFolder != "Documents/backup" {
		t.Fatalf("got %q", c.OxideFolder)
	}
}

func TestEmptyJSON(t *testing.T) {
	c, err := FromJSON([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "custom.json") {
		t.Fatal("header missing")
	}
}

// 坏输入必须报错而不是写出半个文件。
func TestMalformedJSON(t *testing.T) {
	if _, err := FromJSON([]byte(`{"oxide_folder": `)); err == nil {
		t.Fatal("expected a parse error")
	}
}

// slash 只处理分隔符，不动其他字符。
func TestSlash(t *testing.T) {
	if got := slash(`D:\a\b`); got != "D:/a/b" {
		t.Fatalf("got %q", got)
	}
	if got := slash("a/b"); got != "a/b" {
		t.Fatalf("got %q", got)
	}
}
