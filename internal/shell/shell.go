// Package shell 把「平台与 shell 语法差异」全部吸收在渲染层。
// 上层只认识 alias / state / export 这些概念，不关心 bash 还是 PowerShell。
package shell

import (
	"fmt"
	"strings"
)

// Shell 是一套目标 shell 的代码生成器。
type Shell interface {
	// Name 是 grit 内部标识，也是写进 GRIT_SHELL 的值。
	Name() string
	// Interpreter 是给外部工具用的 shell 名（conda shell.<x>）。
	Interpreter() string
	// Alias 生成一条纯转发命令。
	Alias(name, cmd string) string
	// State 生成一个走 eval 通道的包装函数。
	State(cmdName, plugin, action string) string
	// Export 生成一条环境变量赋值。
	Export(key, value string) string
	// Assoc 生成一个关联数组。
	Assoc(name string, pairs [][2]string) string
	// Source 生成 source 一行。
	Source(path string) string
	// Quote 把值转成该 shell 的字面量。
	Quote(value string) string
}

// Get 按名字返回渲染器。
func Get(name string) (Shell, error) {
	switch strings.ToLower(name) {
	case "bash":
		return &posix{name: "bash", interp: "bash"}, nil
	case "zsh":
		return &posix{name: "zsh", interp: "zsh"}, nil
	case "pwsh", "powershell":
		return &pwsh{}, nil
	default:
		return nil, fmt.Errorf("unsupported shell %q (want bash, zsh or pwsh)", name)
	}
}

// splitArg 把 "conda install {0}" 拆成前缀与是否需要函数。
// 占位符在末尾时可以用 alias；在中间时必须用函数转发。
func splitArg(cmd string) (prefix string, needsFunc bool) {
	const ph = "{0}"
	i := strings.Index(cmd, ph)
	if i < 0 {
		return cmd, false
	}
	rest := strings.TrimSpace(cmd[i+len(ph):])
	if rest == "" {
		return strings.TrimSpace(cmd[:i]), false
	}
	return cmd, true
}

func posixQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func pwshQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
