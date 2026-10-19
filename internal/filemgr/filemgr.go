// Package filemgr 实现 L2 计算型操作：备份、恢复、用默认值覆盖、把默认值写入备份。
//
// 这四个动作原本是 epf/ipf/iif/ppgf，在 bash 与 PowerShell 里各写一份。
// 现在只有这一份实现，且不依赖任何 shell。
package filemgr

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ivaquero/grit/internal/ospath"
)

// Action 是四个文件动作之一。
type Action string

const (
	Backup    Action = "backup"    // 原 epf / oxf：ELEMENT -> BACKUP/OXIDE
	Restore   Action = "restore"   // 原 ipf / rdf：BACKUP/OXIDE -> ELEMENT
	Catalyze  Action = "catalyze"  // 原 iif / clzf：OXYGEN -> ELEMENT
	Propagate Action = "propagate" // 原 ppgf：      OXYGEN -> BACKUP/OXIDE
)

// Valid 判断动作名是否合法。
func Valid(a Action) bool {
	switch a {
	case Backup, Restore, Catalyze, Propagate:
		return true
	}
	return false
}

// Set 是四张表，路径均已展开。键带各自的前缀（bk / grit）。
type Set struct {
	Element map[string]string // 真实配置路径
	Oxide   map[string]string // 备份相对路径
	Oxygen  map[string]string // 默认模板在 Defaults 中的路径
	Backup  string            // 备份根目录
	IsWin   bool              // 目标 shell 是否 Windows 风格
	// Defaults 是默认模板的只读来源（二进制内嵌），
	// 因此 catalyze / propagate 不依赖仓库目录是否存在。
	Defaults fs.FS
}

// Options 控制执行行为。
type Options struct {
	DryRun bool
	Out    io.Writer
}

// Result 是单个 key 的执行结果。
type Result struct {
	Key  string
	Src  string
	Dst  string
	Skip string
	Err  error
}

// Run 依次处理 keys，单个失败不影响其余，最后汇报失败数。
func Run(s Set, action Action, keys []string, opt Options) error {
	if opt.Out == nil {
		opt.Out = io.Discard
	}
	if len(keys) == 0 {
		return fmt.Errorf("no keys given")
	}

	failed := 0
	for _, key := range keys {
		r := s.do(action, key, opt)
		switch {
		case r.Err != nil:
			failed++
			fmt.Fprintf(opt.Out, "%-9s %-4s error: %v\n", action, key, r.Err)
		case r.Skip != "":
			fmt.Fprintf(opt.Out, "%-9s %-4s skip: %s\n", action, key, r.Skip)
		default:
			mark := ""
			if opt.DryRun {
				mark = "[dry] "
			}
			fmt.Fprintf(opt.Out, "%s%-9s %-4s %s -> %s\n", mark, action, key, r.Src, r.Dst)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d failed", failed, len(keys))
	}
	return nil
}

func (s Set) do(action Action, key string, opt Options) Result {
	res := Result{Key: key}

	src, dst, fromDefaults, err := s.resolve(action, key)
	if err != nil {
		res.Skip = err.Error()
		return res
	}
	res.Src, res.Dst = src, dst

	if err := guard(src, dst); err != nil {
		res.Err = err
		return res
	}

	if fromDefaults {
		return s.fromDefaults(res, src, dst, opt)
	}

	info, err := os.Stat(src)
	if err != nil {
		res.Err = fmt.Errorf("source unavailable: %w", err)
		return res
	}

	if opt.DryRun {
		if info.IsDir() {
			res.Src += "/"
		}
		return res
	}

	if err := place(src, dst, info.IsDir()); err != nil {
		res.Err = err
	}
	return res
}

// fromDefaults 把内嵌模板写到目标位置。模板都是单文件，不支持目录。
func (s Set) fromDefaults(res Result, src, dst string, opt Options) Result {
	if s.Defaults == nil {
		res.Err = errors.New("no embedded defaults available")
		return res
	}
	f, err := s.Defaults.Open(src)
	if err != nil {
		res.Err = fmt.Errorf("default template %s: %w", src, err)
		return res
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		res.Err = err
		return res
	}
	if info.IsDir() {
		res.Err = fmt.Errorf("default template %s is a directory", src)
		return res
	}
	if opt.DryRun {
		return res
	}
	// 默认模板的来源常报零权限（fstest.MapFS）或只读权限（embed.FS），
	// 直接照搬会写出用户既改不了、也读不了的文件，统一补齐读写位。
	res.Err = copyReader(f, dst, info.Mode().Perm()|0o644)
	return res
}

func copyReader(r io.Reader, dst string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create parent: %w", err)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// resolve 按动作把 key 映射到源与目标。
// fromDefaults 为真时表示源在 Set.Defaults 里，而不是本地磁盘。
func (s Set) resolve(action Action, key string) (string, string, bool, error) {
	bkKey := "bk" + key
	grKey := "gr" + key

	need := func(m map[string]string, k, table string) (string, error) {
		v, ok := m[k]
		if !ok || v == "" {
			return "", fmt.Errorf("no %s entry for %q", table, k)
		}
		return v, nil
	}

	switch action {
	case Backup:
		src, err := need(s.Element, key, "element")
		if err != nil {
			return "", "", false, err
		}
		rel, err := need(s.Oxide, bkKey, "oxides")
		if err != nil {
			return "", "", false, err
		}
		return src, ospath.Join(s.Backup, rel, s.IsWin), false, nil

	case Restore:
		rel, err := need(s.Oxide, bkKey, "oxides")
		if err != nil {
			return "", "", false, err
		}
		dst, err := need(s.Element, key, "element")
		if err != nil {
			return "", "", false, err
		}
		return ospath.Join(s.Backup, rel, s.IsWin), dst, false, nil

	case Catalyze:
		src, err := need(s.Oxygen, grKey, "oxygen")
		if err != nil {
			return "", "", false, err
		}
		dst, err := need(s.Element, key, "element")
		if err != nil {
			return "", "", false, err
		}
		return src, dst, true, nil

	case Propagate:
		src, err := need(s.Oxygen, grKey, "oxygen")
		if err != nil {
			return "", "", false, err
		}
		rel, err := need(s.Oxide, bkKey, "oxides")
		if err != nil {
			return "", "", false, err
		}
		return src, ospath.Join(s.Backup, rel, s.IsWin), true, nil
	}
	return "", "", false, fmt.Errorf("unknown action %q", action)
}

// guard 拒绝会让 RemoveAll 落到危险位置的组合。
func guard(src, dst string) error {
	if src == "" || dst == "" {
		return errors.New("empty source or destination")
	}
	cs, cd := filepath.Clean(src), filepath.Clean(dst)
	if strings.EqualFold(cs, cd) {
		return fmt.Errorf("source and destination are the same: %s", cs)
	}
	if cd == "." || cd == string(filepath.Separator) {
		return fmt.Errorf("refusing to write to %s", cd)
	}
	if vol := filepath.VolumeName(cd); vol != "" && cd == vol {
		return fmt.Errorf("refusing to write to drive root %s", cd)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if strings.EqualFold(filepath.Clean(home), cd) {
			return errors.New("refusing to write to the home directory itself")
		}
	}
	return nil
}

func place(src, dst string, isDir bool) error {
	if isDir {
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("clear destination: %w", err)
		}
		return copyDir(src, dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create parent: %w", err)
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// 补上属主读写位：恢复出来的配置若沿袭备份的只读权限，用户就改不动了。
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm()|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		}
		return copyFile(p, target)
	})
}
