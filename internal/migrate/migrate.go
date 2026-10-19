// Package migrate 把旧的 custom.json 转成 custom.toml。
// 只做格式转换，不猜测语义：每一个字段都有一一对应的落点。
package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ivaquero/grit/internal/custom"
)

// Legacy 是 custom.json 的旧结构。description 字段仅作说明，不迁移。
type Legacy struct {
	PluginsLoad          []string          `json:"plugins_load"`
	PluginsPlus          map[string]string `json:"plugins_plus"`
	PluginsLoadPlus      []string          `json:"plugins_load_plus"`
	ProxyPorts           map[string]int    `json:"proxy_ports"`
	CondaEnvShortcuts    map[string]string `json:"conda_env_shortcuts"`
	JuliaEnvShortcuts    map[string]string `json:"julia_env_shortcuts"`
	JuliaEnvShortcutsWin map[string]string `json:"julia_env_shortcuts_win"`
	StartupFolder        string            `json:"startup_folder"`
	DownloadFolder       string            `json:"download_folder"`
	OxideFolder          string            `json:"oxide_folder"`
	OxideFolderWin       string            `json:"oxide_folder_win"`
	Oxides               map[string]string `json:"oxides"`
}

// FromJSON 解析 custom.json。
func FromJSON(data []byte) (*custom.Config, error) {
	var l Legacy
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parse custom.json: %w", err)
	}

	c := &custom.Config{
		OxideFolder:     slash(l.OxideFolder),
		OxideFolderWin:  slash(l.OxideFolderWin),
		DownloadFolder:  slash(l.DownloadFolder),
		StartupFolder:   slash(l.StartupFolder),
		Oxides:          slashMap(l.Oxides),
		PluginsLoad:     l.PluginsLoad,
		PluginsPlus:     slashMap(l.PluginsPlus),
		PluginsLoadPlus: l.PluginsLoadPlus,
		ProxyPorts:      l.ProxyPorts,
	}
	if len(l.CondaEnvShortcuts) > 0 {
		c.Env.Conda = l.CondaEnvShortcuts
	}
	if len(l.JuliaEnvShortcuts) > 0 {
		c.Env.Julia = slashMap(l.JuliaEnvShortcuts)
	}
	if len(l.JuliaEnvShortcutsWin) > 0 {
		c.Env.JuliaWin = slashMap(l.JuliaEnvShortcutsWin)
	}
	return c, nil
}

// FromFile 读取并转换。
func FromFile(path string) (*custom.Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return FromJSON(b)
}

// Render 生成带说明头的 TOML。
func Render(c *custom.Config) ([]byte, error) {
	body, err := c.Marshal()
	if err != nil {
		return nil, err
	}
	const header = "# 由 `grit migrate` 从 custom.json 生成。\n" +
		"# 这是唯一被读取的配置格式，custom.json 不再使用。\n" +
		"# 相对路径基于用户主目录。\n\n"
	return append([]byte(header), body...), nil
}

// slash 把 Windows 反斜杠统一成正斜杠。
// 正斜杠在 bash 与 PowerShell 下都有效，不必猜测目标平台。
func slash(p string) string {
	return strings.ReplaceAll(p, `\`, "/")
}

func slashMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = slash(v)
	}
	return out
}
