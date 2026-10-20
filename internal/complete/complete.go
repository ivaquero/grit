// Package complete 生成 shell 补全。
//
// 设计要点：补全脚本是静态的，候选值是动态的。
// 脚本只生成一次，每次按 TAB 都调用 `grit complete <context>`，
// 所以新增转换、element、插件之后不需要重新生成脚本。
package complete

import (
	"fmt"
	"runtime"
	"sort"

	"github.com/ivaquero/grit/internal/config"
	"github.com/ivaquero/grit/internal/custom"
	"github.com/ivaquero/grit/internal/juliamgr"
)

var subcommands = []string{
	"init", "state", "file", "shell", "julia", "vscode", "doctor",
	"migrate", "self-update", "conv", "completion", "complete", "list", "version",
}

var fileActions = []string{"backup", "restore", "catalyze", "propagate"}

var listTargets = []string{"aliases", "plugins", "elements", "keys", "conv"}

var vscodeActions = []string{"dir", "clean"}

// Contexts 是 `grit complete` 接受的全部上下文。
func Contexts() []string {
	return []string{
		"subcommands", "actions", "keys", "plugins", "converts", "ports",
		"shells", "list-targets", "state-actions", "julia-ops", "julia-envs",
		"vscode-actions",
	}
}

// Candidates 返回某个上下文下的候选值。
// extra 用于需要额外参数的上下文：state-actions 取插件名。
func Candidates(ctx string, cfg *config.Config, cu *custom.Config, extra ...string) ([]string, error) {
	switch ctx {
	case "subcommands":
		return append([]string(nil), subcommands...), nil
	case "actions":
		return append([]string(nil), fileActions...), nil
	case "list-targets":
		return append([]string(nil), listTargets...), nil
	case "vscode-actions":
		return append([]string(nil), vscodeActions...), nil
	case "shells":
		return []string{"bash", "zsh", "pwsh"}, nil
	case "julia-ops":
		return append([]string(nil), juliamgr.Ops...), nil

	case "keys":
		return elementKeys(cfg), nil

	case "plugins":
		out := make([]string, 0, len(cfg.Order))
		out = append(out, cfg.Order...)
		sort.Strings(out)
		return out, nil

	case "converts":
		seen := map[string]bool{}
		out := make([]string, 0, len(cfg.Converts))
		for _, cv := range cfg.Converts {
			if seen[cv.Name] {
				continue
			}
			seen[cv.Name] = true
			out = append(out, cv.Name)
		}
		sort.Strings(out)
		return out, nil

	case "ports":
		out := make([]string, 0, len(cu.ProxyPorts))
		for k := range cu.ProxyPorts {
			out = append(out, k)
		}
		sort.Strings(out)
		return out, nil

	case "state-actions":
		// 没有插件名时静默返回空：补全过程里报错只会打断用户输入。
		if len(extra) == 0 || cfg == nil {
			return nil, nil
		}
		p, ok := cfg.Plugin(extra[0])
		if !ok {
			return nil, nil
		}
		out := make([]string, 0, len(p.State))
		for k := range p.State {
			out = append(out, k)
		}
		sort.Strings(out)
		return out, nil

	case "julia-envs":
		return juliamgr.ShortcutNames(cu, runtime.GOOS == "windows"), nil
	}
	return nil, fmt.Errorf("unknown completion context %q", ctx)
}

// elementKeys 只给出当前平台实际存在的 element。
func elementKeys(cfg *config.Config) []string {
	out := make([]string, 0, len(cfg.Elements))
	for _, e := range cfg.Elements {
		if runtime.GOOS == "windows" && e.Win == "" {
			continue
		}
		if runtime.GOOS != "windows" && e.Unix == "" {
			continue
		}
		out = append(out, e.Key)
	}
	sort.Strings(out)
	return out
}
