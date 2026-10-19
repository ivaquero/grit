// Package ospath 统一路径展开与风格归一。
// 目标 shell 决定路径风格：Windows 上的 Git Bash 需要 POSIX 风格的路径。
package ospath

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// toSlash 无条件把反斜杠换成正斜杠。
// 不用 filepath.ToSlash：它只在 Windows 宿主上替换，
// 于是同一段代码在 Linux / macOS 上对 "C:\a\b" 会原样放过，
// 三平台行为不一致。
func toSlash(p string) string {
	return strings.ReplaceAll(p, `\`, "/")
}

// Norm 把路径归一成目标平台的风格。
func Norm(p string, isWin bool) string {
	p = toSlash(p)
	if isWin {
		return filepath.FromSlash(p)
	}
	return p
}

// isAbs 判断绝对路径，同时认 Windows 的盘符与 UNC 形式，
// 这样 "C:\x" 在非 Windows 宿主上也不会被当成相对路径拼上主目录。
func isAbs(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\\`) {
		return true
	}
	return len(p) >= 3 && p[1] == ':' && p[2] == '/'
}

// Expand 展开 ~ 与相对路径，然后归一风格。
// 相对路径相对于用户主目录，与 grit 原有语义一致。
func Expand(p string, isWin bool) string {
	if p == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		h := toSlash(home)
		q := toSlash(p)
		switch {
		case q == "~":
			p = h
		case strings.HasPrefix(q, "~/"):
			p = h + "/" + strings.TrimPrefix(q, "~/")
		case !isAbs(q):
			p = h + "/" + q
		default:
			p = q
		}
	}
	return Norm(p, isWin)
}

// Join 拼接路径，并按目标 shell 的风格输出。
// 不用 filepath.Join：它总是输出宿主风格，会让 bash 侧看到反斜杠。
func Join(base, rel string, isWin bool) string {
	if base == "" {
		return ""
	}
	p := path.Join(toSlash(base), toSlash(rel))
	if isWin {
		return filepath.FromSlash(p)
	}
	return p
}

// Within 判断 target 是否位于 base 之下（含 base 的子目录，不含 base 同级）。
func Within(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
