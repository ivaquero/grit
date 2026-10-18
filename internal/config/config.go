// Package config 定义 Goboot 的唯一真源：全部行为由 TOML 声明，shell 语法不出现于此。
package config

import (
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/pelletier/go-toml/v2"
)

// Alias 是纯转发的命令：不需要任何 shell 状态，生成 alias 即可。
type Alias struct {
	Name string `toml:"name"`
	Cmd  string `toml:"cmd"`
	OS   string `toml:"os"` // 空 = 全平台；其余取 runtime.GOOS 的值
}

// Plugin 描述一个包管理器或工具的能力矩阵。
// Actions 走子进程直接执行；State 走 eval 通道回写父 shell。
type Plugin struct {
	Name     string            `toml:"name"`
	Abbr     string            `toml:"abbr"`
	Requires string            `toml:"requires"`
	OS       string            `toml:"os"`
	Actions  map[string]string `toml:"actions"`
	State    map[string]string `toml:"state"`
}

// Convert 描述一条格式转换：工具 + 固定参数 + 输出扩展名。
// 输入为 in_ext 时优先匹配，否则用不带 in_ext 的规则作默认。
type Convert struct {
	Name     string   `toml:"name"`
	Ext      string   `toml:"ext"`
	InExt    string   `toml:"in_ext"`
	Tool     string   `toml:"tool"`
	Requires string   `toml:"requires"`
	Args     []string `toml:"args"`
	Engine   bool     `toml:"engine"` // 需要探测 pdf 引擎
	Stdout   bool     `toml:"stdout"` // 命令把结果写到 stdout，由 goboot 落盘
	OS       string   `toml:"os"`
}

// Element 是 GOBOOT_ELEMENT 注册表中的一项，平台差异在这里表达，不在代码里。
type Element struct {
	Key  string `toml:"key"`
	Unix string `toml:"unix"`
	Win  string `toml:"win"`
}

// Config 是合并后的运行期配置。
type Config struct {
	Aliases  []Alias
	Elements []Element
	Plugins  map[string]*Plugin
	Converts []Convert
	Oxygen   map[string]string // goboot<key> -> defaults 内的模板相对路径
	Order    []string          // 插件名，按字典序，保证生成结果稳定
}

type aliasesFile struct {
	Alias []Alias `toml:"alias"`
}

type elementsFile struct {
	Element []Element `toml:"element"`
}

type convertsFile struct {
	Convert []Convert `toml:"convert"`
}

// Load 从 fsys 的 dir 下读取全部声明。fsys 可以是 embed.FS，也可以是磁盘。
func Load(fsys fs.FS, dir string) (*Config, error) {
	cfg := &Config{Plugins: map[string]*Plugin{}}

	var a aliasesFile
	if err := readTOML(fsys, path.Join(dir, "aliases.toml"), &a); err != nil {
		return nil, err
	}
	cfg.Aliases = a.Alias

	var e elementsFile
	if err := readTOML(fsys, path.Join(dir, "registry.toml"), &e); err != nil {
		return nil, err
	}
	cfg.Elements = e.Element

	// oxygen.toml 是可选的：没有它时 catalyze / propagate 无从执行，
	// 但不影响其余功能。
	oxPath := path.Join(dir, "oxygen.toml")
	if _, statErr := fs.Stat(fsys, oxPath); statErr == nil {
		var o struct {
			Files map[string]string `toml:"files"`
		}
		if err := readTOML(fsys, oxPath, &o); err != nil {
			return nil, err
		}
		cfg.Oxygen = o.Files
	}

	convPath := path.Join(dir, "convert.toml")
	if _, statErr := fs.Stat(fsys, convPath); statErr == nil {
		var c convertsFile
		if err := readTOML(fsys, convPath, &c); err != nil {
			return nil, err
		}
		cfg.Converts = c.Convert
	}

	matches, err := fs.Glob(fsys, path.Join(dir, "plugins", "*.toml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	for _, m := range matches {
		var p Plugin
		if err := readTOML(fsys, m, &p); err != nil {
			return nil, err
		}
		if err := p.validate(m); err != nil {
			return nil, err
		}
		cfg.Plugins[p.Name] = &p
		cfg.Order = append(cfg.Order, p.Name)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func readTOML(fsys fs.FS, name string, v any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	if err := toml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	return nil
}

func (p *Plugin) validate(file string) error {
	if p.Name == "" {
		return fmt.Errorf("%s: missing name", file)
	}
	if p.Abbr == "" {
		return fmt.Errorf("%s: missing abbr", file)
	}
	for k, v := range p.Actions {
		if v == "" {
			return fmt.Errorf("%s: action %q is empty", file, k)
		}
	}
	for k, v := range p.State {
		if v == "" {
			return fmt.Errorf("%s: state %q is empty", file, k)
		}
	}
	return nil
}

// validate 拒绝重复的别名与命令名，避免后加载的静默覆盖。
func (c *Config) validate() error {
	seen := map[string]string{}
	for _, a := range c.Aliases {
		if a.Name == "" || a.Cmd == "" {
			return fmt.Errorf("aliases.toml: alias with empty name or cmd")
		}
		if prev, ok := seen[a.Name]; ok {
			return fmt.Errorf("duplicate command %q (alias and %s)", a.Name, prev)
		}
		seen[a.Name] = "aliases.toml"
	}
	for _, name := range c.Order {
		p := c.Plugins[name]
		for suffix := range p.Actions {
			cmd := p.Abbr + suffix
			if prev, ok := seen[cmd]; ok {
				return fmt.Errorf("duplicate command %q (%s/%s and %s)", cmd, name, suffix, prev)
			}
			seen[cmd] = name + "/actions." + suffix
		}
		for suffix := range p.State {
			// state 的 key 已经是完整命令名：前缀不规则（ceat 而非 cat）。
			cmd := suffix
			if prev, ok := seen[cmd]; ok {
				return fmt.Errorf("duplicate command %q (%s/state.%s and %s)", cmd, name, suffix, prev)
			}
			seen[cmd] = name + "/state." + suffix
		}
	}
	return nil
}

// Plugin 按名字取插件。
func (c *Config) Plugin(name string) (*Plugin, bool) {
	p, ok := c.Plugins[name]
	return p, ok
}

// ConvertFor 按命令名与输入扩展名选规则。
// in_ext 精确匹配优先；没有 in_ext 的规则作默认。
func (c *Config) ConvertFor(name, inExt string) (*Convert, bool) {
	var fallback *Convert
	for i := range c.Converts {
		cv := &c.Converts[i]
		if cv.Name != name {
			continue
		}
		if cv.InExt != "" && cv.InExt == inExt {
			return cv, true
		}
		if cv.InExt == "" && fallback == nil {
			fallback = cv
		}
	}
	return fallback, fallback != nil
}
