// Package doctor 对装配结果做启动期体检。
//
// 它只读：不创建、不修改、不删除任何文件。检查项分三类：
//   - 依赖：插件声明的 requires 是否真的在 PATH 里；
//   - 配置：custom.toml 是否存在、备份根是否配置；
//   - 引用：oxides / oxygen 的键能否在 registry 与内嵌模板里找到落点。
//
// 缺命令只算 miss 不算错误：命令暂时不在 PATH 不等于用户不需要那个别名
// （conda 常装在非标准路径）。是否据此返回非零由调用方决定。
package doctor

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"runtime"
	"sort"
	"strings"

	"github.com/ivaquero/grit/internal/config"
	"github.com/ivaquero/grit/internal/custom"
)

// Level 是单项检查的结果等级。
type Level int

const (
	OK Level = iota
	Warn
	Miss
)

func (l Level) String() string {
	switch l {
	case OK:
		return "ok"
	case Warn:
		return "warn"
	default:
		return "miss"
	}
}

// Entry 是一条检查结果。
type Entry struct {
	Scope  string // config / plugin
	Name   string
	Detail string
	Level  Level
}

// Report 是全量结果。
type Report struct {
	Entries []Entry
	Missing []string // 缺失的 required 命令，按插件名排序
}

// Count 返回某个等级的结果数。
func (r Report) Count(l Level) int {
	n := 0
	for _, e := range r.Entries {
		if e.Level == l {
			n++
		}
	}
	return n
}

// Check 执行全部检查。defaults 为内嵌模板的来源，可为 nil（跳过模板存在性检查）。
func Check(cfg *config.Config, cu *custom.Config, root string, defaults fs.FS) Report {
	var r Report

	r.Entries = append(r.Entries, checkCustom(cu, root))
	r.Entries = append(r.Entries, checkOxides(cfg, cu)...)
	r.Entries = append(r.Entries, checkOxygen(cfg, defaults)...)
	r.Entries = append(r.Entries, checkPlugins(cfg)...)

	r.Missing = missingNames(r.Entries)
	return r
}

// checkCustom 检查用户配置本身。
func checkCustom(cu *custom.Config, root string) Entry {
	e := Entry{Scope: "config", Name: "custom.toml"}

	if cu == nil || cu.Path == "" || root == "" {
		e.Level = Warn
		e.Detail = "user config directory is unknown"
		return e
	}
	if _, err := os.Stat(cu.Path); err != nil {
		e.Level = Warn
		e.Detail = "not found; built-in defaults are in use (" + cu.Path + ")"
		return e
	}
	e.Level = OK
	e.Detail = cu.Path
	return e
}

// checkOxides 检查备份表：键能在 registry 找到，且当前平台有路径。
func checkOxides(cfg *config.Config, cu *custom.Config) []Entry {
	if cu == nil || len(cu.Oxides) == 0 {
		return nil
	}
	elements := elementMap(cfg)

	var out []Entry
	keys := make([]string, 0, len(cu.Oxides))
	for k := range cu.Oxides {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		name := strings.TrimPrefix(k, "bk")
		el, ok := elements[name]
		if !ok {
			out = append(out, Entry{
				Scope:  "config",
				Name:   k,
				Detail: "no element " + name + " in registry.toml",
				Level:  Warn,
			})
			continue
		}
		if !hasPath(el, runtime.GOOS) {
			out = append(out, Entry{
				Scope:  "config",
				Name:   k,
				Detail: fmt.Sprintf("element %s has no path on %s", name, runtime.GOOS),
				Level:  Warn,
			})
			continue
		}
		out = append(out, Entry{
			Scope:  "config",
			Name:   k,
			Detail: cu.Oxides[k],
			Level:  OK,
		})
	}
	return out
}

// checkOxygen 检查默认模板表的落点：元素存在，且模板真的在 defaults 里。
func checkOxygen(cfg *config.Config, defaults fs.FS) []Entry {
	if len(cfg.Oxygen) == 0 {
		return nil
	}
	elements := elementMap(cfg)

	keys := make([]string, 0, len(cfg.Oxygen))
	for k := range cfg.Oxygen {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []Entry
	for _, k := range keys {
		name := strings.TrimPrefix(k, "gr")
		if _, ok := elements[name]; !ok {
			out = append(out, Entry{
				Scope:  "config",
				Name:   k,
				Detail: "no element " + name + " in registry.toml",
				Level:  Warn,
			})
			continue
		}
		if defaults == nil {
			continue
		}
		rel := path.Clean(strings.ReplaceAll(cfg.Oxygen[k], `\`, "/"))
		if _, err := fs.Stat(defaults, rel); err != nil {
			out = append(out, Entry{
				Scope:  "config",
				Name:   k,
				Detail: "template " + rel + " is not embedded",
				Level:  Warn,
			})
			continue
		}
		out = append(out, Entry{Scope: "config", Name: k, Detail: rel, Level: OK})
	}
	return out
}

// checkPlugins 检查每个适用插件的 requires 是否在 PATH 中。
func checkPlugins(cfg *config.Config) []Entry {
	out := make([]Entry, 0, len(cfg.Order))
	for _, name := range cfg.Order {
		p := cfg.Plugins[name]
		if p.OS != "" && p.OS != runtime.GOOS {
			continue
		}
		e := Entry{Scope: "plugin", Name: p.Name, Detail: p.Abbr}
		if p.Requires == "" {
			e.Level = OK
			e.Detail = "no requirement"
			out = append(out, e)
			continue
		}
		e.Detail = p.Requires
		if _, err := exec.LookPath(p.Requires); err != nil {
			e.Level = Miss
		} else {
			e.Level = OK
		}
		out = append(out, e)
	}
	return out
}

func elementMap(cfg *config.Config) map[string]config.Element {
	m := make(map[string]config.Element, len(cfg.Elements))
	for _, e := range cfg.Elements {
		m[e.Key] = e
	}
	return m
}

func hasPath(e config.Element, goos string) bool {
	if goos == "windows" {
		return e.Win != ""
	}
	return e.Unix != ""
}

func missingNames(entries []Entry) []string {
	var out []string
	for _, e := range entries {
		if e.Level == Miss {
			out = append(out, e.Name)
		}
	}
	sort.Strings(out)
	return out
}

// Summary 生成一行摘要，供 grit init --check 使用。
func (r Report) Summary() string {
	if len(r.Missing) == 0 {
		return "all required commands are available"
	}
	shown := r.Missing
	suffix := ""
	if len(shown) > 6 {
		shown = shown[:6]
		suffix = fmt.Sprintf(", +%d more", len(r.Missing)-6)
	}
	return fmt.Sprintf("%d required command(s) missing: %s%s; run `grit doctor`",
		len(r.Missing), strings.Join(shown, ", "), suffix)
}
