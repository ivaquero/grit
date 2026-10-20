// Package vscode 清理 VS Code 的缓存目录。
//
// 缓存是纯派生数据：删掉之后编辑器会重建，因此不需要备份，也不需要确认。
// 但「派生」不等于「可乱删」—— 每个目标都必须落在 Code 数据目录之内，
// 且不允许把数据目录本身当作目标。
package vscode

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ivaquero/grit/internal/ospath"
)

// target 是一条清理规则。all 为真时只有显式 -a 才执行。
type target struct {
	glob string
	desc string
	all  bool
}

// 顺序即输出顺序。前三项是无条件清理项，最后一项要 -a。
var targets = []target{
	{"Cache/*", "cache", false},
	{"CachedData/*", "compiled data cache", false},
	{"User/History/-*", "orphaned history", false},
	{"User/profiles/-*", "orphaned profiles", false},
	{"User/workspaceStorage/*", "workspace storage", true},
}

// Options 控制清理行为。
type Options struct {
	DataDir string // 显式指定数据目录，空则按平台推断
	All     bool   // 同时清理 workspaceStorage
	DryRun  bool   // 只报告，不删除
	Out     io.Writer
}

// Entry 是单个被清理的路径。
type Entry struct {
	Path string
	Desc string
	Size int64 // 删除前的字节数，便于估算回收量
	Err  error
}

// ResolveDataDir 按平台推断 VS Code 的用户数据目录。
// 顺序：显式参数 → GRIT_VSCODE_DATA → 平台默认值。
func ResolveDataDir(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	if v := os.Getenv("GRIT_VSCODE_DATA"); v != "" {
		return filepath.Abs(v)
	}
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			return "", errors.New("APPDATA is not set; pass --data-dir")
		}
		return filepath.Join(base, "Code"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "Code"), nil
	default:
		if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
			return filepath.Join(base, "Code"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", "Code"), nil
	}
}

// Clean 删除命中的缓存目录，返回每一条结果。
// 单个目标失败不影响其余目标。
func Clean(opt Options) ([]Entry, error) {
	dir, err := ResolveDataDir(opt.DataDir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("no VS Code data directory at %s", dir)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	if err := guard(dir); err != nil {
		return nil, err
	}

	var out []Entry
	for _, t := range targets {
		if t.all && !opt.All {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(dir, filepath.FromSlash(t.glob)))
		if err != nil {
			return out, fmt.Errorf("bad pattern %s: %w", t.glob, err)
		}
		for _, m := range matches {
			// Glob 可能返回数据目录本身以外的路径（例如符号链接指向外面），
			// 删之前再判一次归属。
			if !ospath.Within(dir, m) || filepath.Clean(m) == filepath.Clean(dir) {
				continue
			}
			e := Entry{Path: m, Desc: t.desc, Size: dirSize(m)}
			if !opt.DryRun {
				e.Err = os.RemoveAll(m)
			}
			out = append(out, e)
		}
	}
	return out, nil
}

// guard 拒绝把根目录或主目录当成数据目录。
func guard(dir string) error {
	clean := filepath.Clean(dir)
	if clean == "." || clean == string(filepath.Separator) {
		return fmt.Errorf("refusing to clean %s", clean)
	}
	if vol := filepath.VolumeName(clean); vol != "" && clean == vol {
		return fmt.Errorf("refusing to clean drive root %s", clean)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if filepath.Clean(home) == clean {
			return errors.New("refusing to clean the home directory")
		}
	}
	return nil
}

// dirSize 累加目录下的字节数；无法读取的条目按 0 计。
func dirSize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}
