package shell

import (
	"fmt"
	"strings"
)

// posix 覆盖 bash 与 zsh：两者在 alias / 函数 / 关联数组上语法一致。
type posix struct {
	name   string
	interp string
}

func (s *posix) Name() string        { return s.name }
func (s *posix) Interpreter() string { return s.interp }

func (s *posix) Alias(name, cmd string) string {
	prefix, needsFunc := splitArg(cmd)
	if !needsFunc {
		return fmt.Sprintf("alias %s=%s", name, posixQuote(prefix))
	}
	body := strings.ReplaceAll(cmd, "{0}", `"$@"`)
	return fmt.Sprintf("%s() { %s; }", name, body)
}

func (s *posix) State(cmdName, plugin, action string) string {
	return fmt.Sprintf(`%s() { eval "$(grit state %s %s "$@")"; }`, cmdName, plugin, action)
}

func (s *posix) Export(key, value string) string {
	return fmt.Sprintf("export %s=%s", key, posixQuote(value))
}

func (s *posix) Unset(key string) string {
	return fmt.Sprintf("unset %s", key)
}

func (s *posix) Assoc(name string, pairs [][2]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "if [[ -n ${ZSH_VERSION:-} ]]; then typeset -gA %s; else declare -A %s; fi\n", name, name)
	for _, kv := range pairs {
		fmt.Fprintf(&b, "%s[%s]=%s\n", name, kv[0], posixQuote(kv[1]))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (s *posix) Source(path string) string {
	return fmt.Sprintf(". %s", posixQuote(path))
}

func (s *posix) Quote(value string) string { return posixQuote(value) }

// pwsh 是 PowerShell 5.1 与 7.x 的渲染器。
// 注意 Set-Alias 无法携带参数，所以纯转发也一律生成 function。
type pwsh struct{}

func (s *pwsh) Name() string        { return "pwsh" }
func (s *pwsh) Interpreter() string { return "powershell" }

func (s *pwsh) Alias(name, cmd string) string {
	body := cmd
	if strings.Contains(body, "{0}") {
		body = strings.ReplaceAll(body, "{0}", "@args")
	} else {
		body += " @args"
	}
	return fmt.Sprintf("function %s { %s }", name, body)
}

func (s *pwsh) State(cmdName, plugin, action string) string {
	return fmt.Sprintf(
		"function %s { grit state %s %s @args | Out-String | Invoke-Expression }",
		cmdName, plugin, action)
}

func (s *pwsh) Export(key, value string) string {
	return fmt.Sprintf("$env:%s = %s", key, pwshQuote(value))
}

func (s *pwsh) Unset(key string) string {
	return fmt.Sprintf("Remove-Item Env:%s -ErrorAction SilentlyContinue", key)
}

func (s *pwsh) Assoc(name string, pairs [][2]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "$Global:%s = @{\n", name)
	for _, kv := range pairs {
		fmt.Fprintf(&b, "    %s = %s\n", pwshQuote(kv[0]), pwshQuote(kv[1]))
	}
	b.WriteString("}")
	return b.String()
}

func (s *pwsh) Source(path string) string {
	return fmt.Sprintf(". %s", pwshQuote(path))
}

func (s *pwsh) Quote(value string) string { return pwshQuote(value) }
