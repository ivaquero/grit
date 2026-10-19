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
)

var subcommands = []string{
	"init", "state", "file", "shell", "migrate", "self-update",
	"conv", "completion", "complete", "list", "version",
}

var fileActions = []string{"backup", "restore", "catalyze", "propagate"}

var listTargets = []string{"aliases", "plugins", "elements", "conv", "keys"}

// Contexts 是 `grit complete` 接受的全部上下文。
func Contexts() []string {
	return []string{"subcommands", "actions", "keys", "plugins", "converts", "ports", "shells", "list-targets"}
}

// Candidates 返回某个上下文下的候选值。
func Candidates(ctx string, cfg *config.Config, cu *custom.Config) ([]string, error) {
	switch ctx {
	case "subcommands":
		return append([]string(nil), subcommands...), nil
	case "actions":
		return append([]string(nil), fileActions...), nil
	case "list-targets":
		return append([]string(nil), listTargets...), nil
	case "shells":
		return []string{"bash", "zsh", "pwsh"}, nil

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
