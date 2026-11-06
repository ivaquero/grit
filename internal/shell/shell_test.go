package shell

import (
	"strings"
	"testing"
)

// 这套测试锁住整个项目最要命的一层：把 TOML 声明翻成各 shell 的代码。
// 渲染回归不会崩溃在编译期，只会在用户 eval 的那刻炸出语法错误或语义错位，
// 所以每条生成路径都要有期望字符串兜底。

func TestGetKnownShells(t *testing.T) {
	cases := []struct {
		in   string
		name string
		intp string
	}{
		{"bash", "bash", "bash"},
		{"zsh", "zsh", "zsh"},
		{"pwsh", "pwsh", "powershell"},
		{"powershell", "pwsh", "powershell"},
		{"BASH", "bash", "bash"}, // 名字大小写不敏感
	}
	for _, c := range cases {
		sh, err := Get(c.in)
		if err != nil {
			t.Fatalf("Get(%q): %v", c.in, err)
		}
		if sh.Name() != c.name || sh.Interpreter() != c.intp {
			t.Fatalf("Get(%q) = %s/%s, want %s/%s", c.in, sh.Name(), sh.Interpreter(), c.name, c.intp)
		}
	}
}

func TestGetUnknownShell(t *testing.T) {
	if _, err := Get("fish"); err == nil {
		t.Fatal("expected an error for an unsupported shell")
	}
}

func TestPosixAlias(t *testing.T) {
	sh, _ := Get("bash")
	// 无占位符：纯 alias，参数由 shell 自动追加。
	if got := sh.Alias("ll", "lsd -l"); got != "alias ll='lsd -l'" {
		t.Fatalf("plain alias: got %q", got)
	}
	// 占位符在末尾：仍是 alias，去掉占位符即可。
	if got := sh.Alias("cis", "conda install {0}"); got != "alias cis='conda install'" {
		t.Fatalf("trailing placeholder alias: got %q", got)
	}
	// 占位符在中间：必须生成函数把参数插回原位。
	if got := sh.Alias("e", "echo {0} done"); got != `e() { echo "$@" done; }` {
		t.Fatalf("mid placeholder -> function: got %q", got)
	}
}

func TestZshAliasMatchesBash(t *testing.T) {
	bash, _ := Get("bash")
	zsh, _ := Get("zsh")
	if bash.Alias("ll", "lsd -l") != zsh.Alias("ll", "lsd -l") {
		t.Fatal("bash and zsh must render aliases identically")
	}
	if bash.Assoc("E", [][2]string{{"k", "v"}}) != zsh.Assoc("E", [][2]string{{"k", "v"}}) {
		t.Fatal("bash and zsh must render assoc arrays identically")
	}
}

func TestPwshAlias(t *testing.T) {
	sh, _ := Get("pwsh")
	// 无占位符时补 @args；有占位符时把它换成 @args。Set-Alias 带不了参数，所以一律 function。
	if got := sh.Alias("ll", "lsd -l"); got != "function ll { lsd -l @args }" {
		t.Fatalf("plain: got %q", got)
	}
	if got := sh.Alias("cis", "conda install {0}"); got != "function cis { conda install @args }" {
		t.Fatalf("placeholder: got %q", got)
	}
}

func TestStateWrapper(t *testing.T) {
	if posix, _ := Get("bash"); posix.State("ceat", "conda", "ceat") !=
		`ceat() { eval "$(grit state conda ceat "$@")"; }` {
		t.Fatalf("posix state: got %q", posix.State("ceat", "conda", "ceat"))
	}
	if pw, _ := Get("pwsh"); pw.State("ceat", "conda", "ceat") !=
		"function ceat { grit state conda ceat @args | Out-String | Invoke-Expression }" {
		t.Fatalf("pwsh state: got %q", pw.State("ceat", "conda", "ceat"))
	}
}

func TestExportUnset(t *testing.T) {
	posix, _ := Get("bash")
	pw, _ := Get("pwsh")

	if got := posix.Export("GRIT", "/root"); got != "export GRIT='/root'" {
		t.Fatalf("posix export: got %q", got)
	}
	if got := posix.Unset("JULIA_PROJECT"); got != "unset JULIA_PROJECT" {
		t.Fatalf("posix unset: got %q", got)
	}
	if got := pw.Export("GRIT", "C:\\x"); got != `$env:GRIT = 'C:\x'` {
		t.Fatalf("pwsh export: got %q", got)
	}
	if got := pw.Unset("JULIA_PROJECT"); got != "Remove-Item Env:JULIA_PROJECT -ErrorAction SilentlyContinue" {
		t.Fatalf("pwsh unset: got %q", got)
	}
	// 空值导出与 Unset 语义不同：这里验证 Export(key,"") 仍可生成赋值。
	if got := posix.Export("P", ""); got != "export P=''" {
		t.Fatalf("posix empty export: got %q", got)
	}
}

func TestAssoc(t *testing.T) {
	posix, _ := Get("bash")
	pairs := [][2]string{{"zs", "/root/.zshrc"}, {"bs", "'quoted'"}}
	got := posix.Assoc("GRIT_ELEMENT", pairs)
	// 声明行 + 每个键值一行，键与值都走 posixQuote 单引号包裹。
	if !strings.Contains(got, "declare -A GRIT_ELEMENT") {
		t.Fatalf("posix assoc missing declare: %q", got)
	}
	if !strings.Contains(got, "GRIT_ELEMENT['zs']='/root/.zshrc'") {
		t.Fatalf("posix assoc missing pair: %q", got)
	}
	// 含单引号的值必须被安全转义，不能提前闭合外层引号；键同样加引号，
	// 与 pwsh.Assoc 对齐。这里锁定的是「Assoc 用 Quote 同时包裹键与值」这一契约，
	// 具体转义序列由 TestQuoteSafety 独立验证。
	if want := "GRIT_ELEMENT[" + posix.Quote("bs") + "]=" + posix.Quote("'quoted'"); !strings.Contains(got, want) {
		t.Fatalf("posix assoc did not wrap key/value via Quote: got %q want substring %q", got, want)
	}

	pw, _ := Get("pwsh")
	pwgot := pw.Assoc("GRIT_ELEMENT", pairs)
	if !strings.Contains(pwgot, "$Global:GRIT_ELEMENT = @{") || !strings.Contains(pwgot, "    'zs' = '/root/.zshrc'") {
		t.Fatalf("pwsh assoc: got %q", pwgot)
	}
}

func TestSource(t *testing.T) {
	posix, _ := Get("bash")
	if got := posix.Source("/path/to file"); got != ". '/path/to file'" {
		t.Fatalf("posix source: got %q", got)
	}
}

// 引号注入是这套生成器的核心安全边界：值里的单引号绝不能提前闭合外层引号。
func TestQuoteSafety(t *testing.T) {
	posix, _ := Get("bash")
	if got := posix.Quote("it's"); got != `'it'\''s'` {
		t.Fatalf("posix quote: got %q", got)
	}
	pw, _ := Get("pwsh")
	if got := pw.Quote("it's"); got != "'it''s'" {
		t.Fatalf("pwsh quote: got %q", got)
	}
}

func TestSplitArg(t *testing.T) {
	cases := []struct {
		cmd       string
		prefix    string
		needsFunc bool
	}{
		{"lsd -l", "lsd -l", false},
		{"conda install {0}", "conda install", false}, // 占位符在末尾
		{"echo {0} done", "echo {0} done", true},      // 占位符在中间，保留原串
	}
	for _, c := range cases {
		p, nf := splitArg(c.cmd)
		if p != c.prefix || nf != c.needsFunc {
			t.Fatalf("splitArg(%q) = %q,%v want %q,%v", c.cmd, p, nf, c.prefix, c.needsFunc)
		}
	}
}
