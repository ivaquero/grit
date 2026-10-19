// Package selfupdate 接管原来的 upox：拉取 Grit 与插件仓库的最新状态。
// 用 cmd.Dir 而不是改变进程工作目录，避免在 Windows 上锁住仓库目录。
package selfupdate

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ivaquero/grit/internal/ospath"
)

const (
	repoPosix = "https://github.com/ivaquero/oxplugins.git"
	repoPwsh  = "https://github.com/ivaquero/oxplugins-pwsh.git"
)

// Options 控制更新行为。
type Options struct {
	Root      string // Grit 根目录
	Shell     string // pwsh 或其它（决定插件仓库）
	Force     bool   // 删除插件目录后重新克隆
	NoPlugins bool   // 只更新本体
	Out       io.Writer
}

// Run 执行更新。先更新本体，再处理插件目录。
func Run(opt Options) error {
	if opt.Out == nil {
		opt.Out = io.Discard
	}
	if opt.Root == "" {
		return fmt.Errorf("Grit root is unknown")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is required but was not found in PATH")
	}
	if _, err := os.Stat(filepath.Join(opt.Root, ".git")); err != nil {
		return fmt.Errorf("%s is not a git repository", opt.Root)
	}

	fmt.Fprintln(opt.Out, "Updating Grit...")
	if err := syncRepo(opt.Root, "master", opt.Out); err != nil {
		return fmt.Errorf("update Grit: %w", err)
	}

	if opt.NoPlugins {
		return nil
	}

	pluginDir := filepath.Join(opt.Root, "plugins")
	if !ospath.Within(opt.Root, pluginDir) {
		return fmt.Errorf("refusing to manage %s", pluginDir)
	}

	repo := repoPosix
	if opt.Shell == "pwsh" {
		repo = repoPwsh
	}

	if opt.Force {
		if _, err := os.Stat(pluginDir); err == nil {
			fmt.Fprintf(opt.Out, "Removing %s...\n", pluginDir)
			if err := os.RemoveAll(pluginDir); err != nil {
				return fmt.Errorf("remove plugins: %w", err)
			}
		}
	}

	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		fmt.Fprintln(opt.Out, "Cloning Grit plugins...")
		if err := git("", "clone", "--depth=1", repo, pluginDir); err != nil {
			return fmt.Errorf("clone plugins: %w", err)
		}
		return nil
	}

	fmt.Fprintln(opt.Out, "Updating Grit plugins...")
	return syncRepo(pluginDir, "main", opt.Out)
}

// syncRepo 把仓库硬同步到远端分支。
// 与原来的 upox 一致：本地改动会被丢弃。
func syncRepo(dir, branch string, out io.Writer) error {
	_ = out
	if err := git(dir, "fetch", "origin", branch); err != nil {
		return err
	}
	return git(dir, "reset", "--hard", "origin/"+branch)
}

func git(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stderr // 进度信息走 stderr，保持 stdout 干净
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Remote 返回当前 shell 对应的插件仓库地址，供 list 之类的命令展示。
func Remote(shellName string) string {
	if shellName == "pwsh" {
		return repoPwsh
	}
	return repoPosix
}
