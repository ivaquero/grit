package config

import (
	"strings"
	"testing"
	"testing/fstest"
)

// loadPlugin 用一个最小文件树加载单个插件声明。
func loadPlugin(t *testing.T, plugin string) (*Config, error) {
	t.Helper()
	fsys := fstest.MapFS{
		"d/aliases.toml":   &fstest.MapFile{Data: []byte("")},
		"d/registry.toml":  &fstest.MapFile{Data: []byte("")},
		"d/plugins/p.toml": &fstest.MapFile{Data: []byte(plugin)},
	}
	return Load(fsys, "d")
}

// [run] 的 key 是完整命令名，与 [actions] 拼出来的名字撞车时必须拒绝，
// 否则生成哪条取决于遍历顺序。
func TestLoadRejectsDuplicateRunCommand(t *testing.T) {
	_, err := loadPlugin(t, `
name = "p"
abbr = "p"

[actions]
is = "echo {0}"

[run]
pis = { runner = "julia", op = "add" }
`)
	if err == nil || !strings.Contains(err.Error(), "duplicate command") {
		t.Fatalf("got %v, want a duplicate command error", err)
	}
}

// 同一个命令名不能同时声明两条通道。
func TestLoadRejectsRunAndStateNameClash(t *testing.T) {
	_, err := loadPlugin(t, `
name = "p"
abbr = "p"

[run]
peat = { runner = "julia", op = "activate", state = true }

[state]
peat = "conda activate {0}"
`)
	if err == nil || !strings.Contains(err.Error(), "both [run] and [state]") {
		t.Fatalf("got %v, want a run/state clash error", err)
	}
}

// 缺 runner 或 op 的声明无法执行，加载期就要失败。
func TestLoadRejectsIncompleteRun(t *testing.T) {
	for _, body := range []string{
		"pr = { op = \"add\" }",
		"pr = { runner = \"julia\" }",
	} {
		_, err := loadPlugin(t, "name = \"p\"\nabbr = \"p\"\n\n[run]\n"+body+"\n")
		if err == nil {
			t.Fatalf("%s: expected an error", body)
		}
	}
}

func TestConvertForPrefersMatchingInExt(t *testing.T) {
	c := &Config{Converts: []Convert{
		{Name: "tomd", Ext: "md", Tool: "pandoc"},
		{Name: "tomd", Ext: "md", Tool: "jupytext", InExt: "ipynb"},
	}}

	got, ok := c.ConvertFor("tomd", "ipynb")
	if !ok {
		t.Fatal("expected a rule for .ipynb")
	}
	if got.Tool != "jupytext" {
		t.Fatalf("got tool %q, want jupytext", got.Tool)
	}

	got, ok = c.ConvertFor("tomd", "rst")
	if !ok {
		t.Fatal("expected the fallback rule")
	}
	if got.Tool != "pandoc" {
		t.Fatalf("got tool %q, want pandoc", got.Tool)
	}
}

// 只有 in_ext 规则时，不匹配的输入不应命中。
func TestConvertForNoFallback(t *testing.T) {
	c := &Config{Converts: []Convert{
		{Name: "ipynb2py", Ext: "py", Tool: "marimo", InExt: "ipynb"},
	}}
	if _, ok := c.ConvertFor("ipynb2py", "md"); ok {
		t.Fatal("a rule with in_ext must not match other extensions")
	}
	if _, ok := c.ConvertFor("unknown", "md"); ok {
		t.Fatal("unknown name must not match")
	}
}

func TestConvertForEmptyConfig(t *testing.T) {
	c := &Config{}
	if _, ok := c.ConvertFor("tohtml", "md"); ok {
		t.Fatal("empty config must not match anything")
	}
}
