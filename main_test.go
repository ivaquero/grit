package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/ivaquero/grit/internal/config"
	"github.com/ivaquero/grit/internal/shell"
)

// main.go 是 1000+ 行的编排层，此前 0% 覆盖。这里锁定其中不依赖外部子进程、
// 可以纯函数化的关键逻辑：占位符渲染、平台匹配、执行器校验、字节格式化。
// 真正的风险面（生成给 shell eval 的字符串）尤其需要断言。

func mustShell(t *testing.T, name string) shell.Shell {
	t.Helper()
	sh, err := shell.Get(name)
	if err != nil {
		t.Fatalf("shell.Get(%q): %v", name, err)
	}
	return sh
}

func TestRenderTemplatePlaceholders(t *testing.T) {
	bash := mustShell(t, "bash")

	// {shell} 换成解释器名，{0} 换成逐个引号包裹的参数。
	got := renderTemplate(bash, "conda shell.{shell} activate {0}", []string{"kaggle"})
	if want := "conda shell.bash activate 'kaggle'"; got != want {
		t.Fatalf("render got %q want %q", got, want)
	}

	// 多个参数各自安全引号，空格不会把参数拆开。
	got = renderTemplate(bash, "echo {0}", []string{"a b", "c"})
	if want := "echo 'a b' 'c'"; got != want {
		t.Fatalf("multi-arg render got %q want %q", got, want)
	}

	// 无参数时，占位符连同前导空格一起去掉，模板退化成无参调用。
	got = renderTemplate(bash, "conda install {0}", nil)
	if want := "conda install"; got != want {
		t.Fatalf("no-arg render got %q want %q", got, want)
	}
}

// 用户参数必须整体交给 Quote 包裹，否则含单引号/命令分隔符的输入会被 shell 拆开执行。
func TestRenderTemplateQuotesInjection(t *testing.T) {
	bash := mustShell(t, "bash")
	hostile := "x'; rm -rf /; '"
	got := renderTemplate(bash, "conda activate {0}", []string{hostile})
	want := "conda activate " + bash.Quote(hostile)
	if got != want {
		t.Fatalf("render must delegate arg to Quote.\ngot  %q\nwant %q", got, want)
	}
}

func TestRenderTemplatePwshQuoting(t *testing.T) {
	pw := mustShell(t, "pwsh")
	got := renderTemplate(pw, "echo {0}", []string{"it's"})
	if want := "echo 'it''s'"; got != want {
		t.Fatalf("pwsh render got %q want %q", got, want)
	}
}

// {self} 应被替换成二进制自身路径；os.Executable 可用时不能残留占位符。
func TestRenderTemplateSelf(t *testing.T) {
	bash := mustShell(t, "bash")
	got := renderTemplate(bash, "{self} complete keys", nil)
	if strings.Contains(got, "{self}") {
		t.Skipf("os.Executable unavailable in this environment: %q", got)
	}
	if !strings.Contains(got, "complete keys") {
		t.Fatalf("unexpected render: %q", got)
	}
}

func TestMatchesOS(t *testing.T) {
	if !matchesOS("") {
		t.Fatal("empty os must match every platform")
	}
	if !matchesOS(runtime.GOOS) {
		t.Fatalf("runtime.GOOS %q must match", runtime.GOOS)
	}
	if matchesOS("not-a-real-os") {
		t.Fatal("bogus os must not match")
	}
}

func TestSortedKeysAndRunKeys(t *testing.T) {
	got := sortedKeys(map[string]string{"b": "2", "a": "1", "c": "3"})
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("sortedKeys = %v", got)
	}

	run := map[string]config.Run{
		"jlis":  {Runner: "julia", Op: "add"},
		"jleat": {Runner: "julia", Op: "activate", State: true},
		"jlmt":  {Runner: "julia", Op: "maturity"},
	}
	// 直接执行通道（state=false）与 eval 通道（state=true）互斥且各自有序。
	if got := strings.Join(sortedRunKeys(run, false), ","); got != "jlis,jlmt" {
		t.Fatalf("non-state run keys = %q", got)
	}
	if got := strings.Join(sortedRunKeys(run, true), ","); got != "jleat" {
		t.Fatalf("state run keys = %q", got)
	}
}

// checkRunners 必须在生成前拒绝未知执行器，
// 否则会产生一条一调用就报 unknown runner 的坏别名。
func TestCheckRunners(t *testing.T) {
	valid := &config.Config{
		Order: []string{"p"},
		Plugins: map[string]*config.Plugin{"p": {Name: "p", Run: map[string]config.Run{
			"pa": {Runner: runnerJulia, Op: "add"},
			"pv": {Runner: runnerVSCode, Op: "clean"},
		}}},
	}
	if err := checkRunners(valid); err != nil {
		t.Fatalf("known runners must pass: %v", err)
	}

	bad := &config.Config{
		Order: []string{"p"},
		Plugins: map[string]*config.Plugin{"p": {Name: "p", Run: map[string]config.Run{
			"px": {Runner: "nonexistent", Op: "x"},
		}}},
	}
	err := checkRunners(bad)
	if err == nil || !strings.Contains(err.Error(), "unknown runner") {
		t.Fatalf("unknown runner must error, got %v", err)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1048576, "1.0 MiB"},
		{1073741824, "1.0 GiB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.n); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
