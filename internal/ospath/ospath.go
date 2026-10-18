// Package ospath 统一路径展开与风格归一。
// 目标 shell 决定路径风格：Windows 上的 Git Bash 需要 POSIX 风格的路径。
package ospath

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Norm 把路径归一成目标平台的风格。
func Norm(p string, isWin bool) string {
	p = filepath.ToSlash(p)
	if isWin {
		return filepath.FromSlash(p)
	}
	return p
}

// Expand 展开 ~ 与相对路径，然后归一风格。
// 相对路径相对于用户主目录，与 goboot 原有语义一致。
func Expand(p string, isWin bool) string {
	if p == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		h := filepath.ToSlash(home)
		switch {
		case p == "~":
			p = h
		case strings.HasPrefix(p, "~/"):
			p = h + "/" + strings.TrimPrefix(p, "~/")
		case !filepath.IsAbs(p):
			p = h + "/" + p
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
	p := path.Join(filepath.ToSlash(base), filepath.ToSlash(rel))
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
