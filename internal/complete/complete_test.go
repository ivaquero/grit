package complete

import (
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ivaquero/grit/internal/config"
	"github.com/ivaquero/grit/internal/custom"
)

func TestCandidatesSubcommands(t *testing.T) {
	got, err := Candidates("subcommands", &config.Config{}, &custom.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"init", "file", "conv", "list"} {
		if !slices.Contains(got, want) {
			t.Fatalf("missing subcommand %q in %v", want, got)
		}
	}
}

// tomd 有两条规则（pandoc 与 jupytext），补全只能出现一次。
func TestCandidatesConvertsAreDeduplicated(t *testing.T) {
	cfg := &config.Config{Converts: []config.Convert{
		{Name: "tomd", Ext: "md", Tool: "pandoc"},
		{Name: "tomd", Ext: "md", Tool: "jupytext", InExt: "ipynb"},
		{Name: "tohtml", Ext: "html", Tool: "pandoc"},
	}}
	got, err := Candidates("converts", cfg, &custom.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 unique conversions", got)
	}
}

// 只提供当前平台真实存在的 element。
func TestCandidatesElementKeysFilterByPlatform(t *testing.T) {
	cfg := &config.Config{Elements: []config.Element{
		{Key: "both", Unix: "/a", Win: "C:/a"},
		{Key: "unixonly", Unix: "/b"},
		{Key: "winonly", Win: "C:/c"},
	}}
	got, err := Candidates("keys", cfg, &custom.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(got, "both") {
		t.Fatal("universal element missing")
	}
	if runtime.GOOS == "windows" {
		if !slices.Contains(got, "winonly") || slices.Contains(got, "unixonly") {
			t.Fatalf("windows filter wrong: %v", got)
		}
	} else {
		if !slices.Contains(got, "unixonly") || slices.Contains(got, "winonly") {
			t.Fatalf("unix filter wrong: %v", got)
		}
	}
}

func TestCandidatesPorts(t *testing.T) {
	cu := &custom.Config{ProxyPorts: map[string]int{"c": 7890, "m": 7898}}
	got, err := Candidates("ports", &config.Config{}, cu)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "c" || got[1] != "m" {
		t.Fatalf("got %v", got)
	}
}

func TestCandidatesUnknownContext(t *testing.T) {
	if _, err := Candidates("nope", &config.Config{}, &custom.Config{}); err == nil {
		t.Fatal("expected an error for an unknown context")
	}
}

// state 的第二层补全需要插件名；插件不存在时静默返回空。
func TestCandidatesStateActions(t *testing.T) {
	cfg := &config.Config{
		Plugins: map[string]*config.Plugin{
			"julia": {Name: "julia", Abbr: "jl", State: map[string]string{"jeq": "x", "jleat": "y"}},
		},
	}
	got, err := Candidates("state-actions", cfg, &custom.Config{}, "julia")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "jeq" || got[1] != "jleat" {
		t.Fatalf("got %v", got)
	}

	for _, extra := range [][]string{nil, {"ghost"}} {
		got, err := Candidates("state-actions", cfg, &custom.Config{}, extra...)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("extra=%v: got %v", extra, got)
		}
	}
}

func TestCandidatesJulia(t *testing.T) {
	cu := &custom.Config{Env: custom.Env{Julia: map[string]string{"t": "/tmp/t", "b": "/tmp/b"}}}
	got, err := Candidates("julia-envs", &config.Config{}, cu)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "b" || got[1] != "t" {
		t.Fatalf("got %v", got)
	}

	ops, err := Candidates("julia-ops", &config.Config{}, cu)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"activate", "add", "leaves"} {
		if !slices.Contains(ops, want) {
			t.Fatalf("missing operation %q in %v", want, ops)
		}
	}

	vsc, err := Candidates("vscode-actions", &config.Config{}, cu)
	if err != nil {
		t.Fatal(err)
	}
	if len(vsc) != 2 || vsc[0] != "dir" || vsc[1] != "clean" {
		t.Fatalf("got %v", vsc)
	}
}

// 新增子命令必须出现在第一层补全里。
func TestSubcommandsCoverNewCommands(t *testing.T) {
	got, err := Candidates("subcommands", &config.Config{}, &custom.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"julia", "vscode", "doctor"} {
		if !slices.Contains(got, want) {
			t.Fatalf("missing subcommand %q", want)
		}
	}
	// 脚本里出现的每个上下文都必须真实存在。
	ctxs := Contexts()
	for _, c := range []string{"state-actions", "julia-ops", "julia-envs", "vscode-actions"} {
		if !slices.Contains(ctxs, c) {
			t.Fatalf("context %q not registered", c)
		}
	}
}

// 三份脚本都必须非空且包含对应的注册方式。
func TestScripts(t *testing.T) {
	cases := map[string]string{
		"bash": "complete -F",
		"zsh":  "compdef",
		"pwsh": "Register-ArgumentCompleter",
	}
	for name, marker := range cases {
		s, err := Script(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if s == "" {
			t.Fatalf("%s: empty script", name)
		}
		if !strings.Contains(s, marker) {
			t.Fatalf("%s script does not contain %q", name, marker)
		}
	}
	if _, err := Script("fish"); err == nil {
		t.Fatal("expected an error for an unsupported shell")
	}
}
