package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ivaquero/grit/internal/config"
	"github.com/ivaquero/grit/internal/custom"
)

func plugin(name, requires, osName string) *config.Plugin {
	return &config.Plugin{Name: name, Abbr: name[:1], Requires: requires, OS: osName}
}

func existingCustom(t *testing.T) *custom.Config {
	t.Helper()
	p := filepath.Join(t.TempDir(), "custom.toml")
	if err := os.WriteFile(p, []byte("oxide_folder = \"backup\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &custom.Config{Path: p}
}

func TestCheckReportsMissingCommand(t *testing.T) {
	cfg := &config.Config{
		Plugins: map[string]*config.Plugin{
			"ghost": plugin("ghost", "grit-no-such-binary-9f3a", ""),
			"free":  plugin("free", "", ""),
		},
		Order: []string{"ghost", "free"},
	}
	rep := Check(cfg, existingCustom(t), t.TempDir(), nil)

	if len(rep.Missing) != 1 || rep.Missing[0] != "ghost" {
		t.Fatalf("missing = %v", rep.Missing)
	}
	okPlugins := 0
	for _, e := range rep.Entries {
		if e.Scope == "plugin" && e.Level == OK {
			okPlugins++
		}
	}
	if rep.Count(Miss) != 1 || okPlugins != 1 {
		t.Fatalf("counts: ok plugins=%d miss=%d", okPlugins, rep.Count(Miss))
	}
	if !strings.Contains(rep.Summary(), "ghost") {
		t.Fatalf("summary = %q", rep.Summary())
	}
}

// 平台不匹配的插件不参与检查。
func TestCheckSkipsOtherPlatforms(t *testing.T) {
	cfg := &config.Config{
		Plugins: map[string]*config.Plugin{
			"portable": plugin("portable", "", ""),
			"foreign":  plugin("foreign", "grit-no-such-binary-9f3a", "plan9"),
		},
		Order: []string{"portable", "foreign"},
	}
	rep := Check(cfg, existingCustom(t), t.TempDir(), nil)
	for _, e := range rep.Entries {
		if e.Name == "foreign" {
			t.Fatal("plugin for another OS must be skipped")
		}
	}
}

func TestCheckOxidesReference(t *testing.T) {
	cfg := &config.Config{
		Elements: []config.Element{{Key: "both", Unix: "/a", Win: "C:/a"}},
	}
	cu := existingCustom(t)
	cu.Oxides = map[string]string{"bkboth": "shell/a", "bkghost": "shell/g"}

	rep := Check(cfg, cu, t.TempDir(), nil)

	levels := map[string]Level{}
	for _, e := range rep.Entries {
		levels[e.Name] = e.Level
	}
	if levels["bkboth"] != OK {
		t.Fatalf("bkboth = %v", levels["bkboth"])
	}
	if levels["bkghost"] != Warn {
		t.Fatalf("bkghost = %v", levels["bkghost"])
	}
}

func TestCheckOxygenTemplate(t *testing.T) {
	cfg := &config.Config{
		Elements: []config.Element{{Key: "g", Unix: "/a", Win: "C:/a"}},
		Oxygen:   map[string]string{"grg": "defaults/files/gitconfig"},
	}
	fsys := fstest.MapFS{"defaults/files/gitconfig": &fstest.MapFile{Data: []byte("x")}}

	rep := Check(cfg, existingCustom(t), t.TempDir(), fsys)
	for _, e := range rep.Entries {
		if e.Name == "grg" && e.Level != OK {
			t.Fatalf("grg = %v (%s)", e.Level, e.Detail)
		}
	}

	empty := fstest.MapFS{}
	rep = Check(cfg, existingCustom(t), t.TempDir(), empty)
	for _, e := range rep.Entries {
		if e.Name == "grg" && e.Level != Warn {
			t.Fatalf("missing template should warn, got %v", e.Level)
		}
	}
}

// 用户配置缺失只警告，不中断。
func TestCheckMissingCustomIsWarning(t *testing.T) {
	cu := &custom.Config{Path: filepath.Join(t.TempDir(), "nope.toml")}
	rep := Check(&config.Config{}, cu, t.TempDir(), nil)
	for _, e := range rep.Entries {
		if e.Name == "custom.toml" && e.Level != Warn {
			t.Fatalf("level = %v", e.Level)
		}
	}
}

// 缺失项超过 6 个时摘要要截断，否则一行会很长。
func TestSummaryTruncates(t *testing.T) {
	var r Report
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		r.Missing = append(r.Missing, n)
	}
	s := r.Summary()
	if !strings.Contains(s, "+2 more") {
		t.Fatalf("summary = %q", s)
	}
	if (Report{}).Summary() == "" {
		t.Fatal("empty report must still summarize")
	}
}
