// Package custom 加载用户配置。TOML 是唯一格式，不再有 JSON 双路径。
package custom

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// Env 是环境别名表：把短名映射到具体环境。
// julia_win 是平台变体，与 oxide_folder_win 同一套约定。
type Env struct {
	Conda    map[string]string `toml:"conda,omitempty"`
	Julia    map[string]string `toml:"julia,omitempty"`
	JuliaWin map[string]string `toml:"julia_win,omitempty"`
}

// Config 是用户的个性化设置，对应原来的 custom.json。
type Config struct {
	OxideFolder    string `toml:"oxide_folder,omitempty"`
	OxideFolderWin string `toml:"oxide_folder_win,omitempty"`
	DownloadFolder string `toml:"download_folder,omitempty"`
	StartupFolder  string `toml:"startup_folder,omitempty"`

	Oxides          map[string]string `toml:"oxides,omitempty"`
	PluginsLoad     []string          `toml:"plugins_load,omitempty"`
	PluginsPlus     map[string]string `toml:"plugins_plus,omitempty"`
	PluginsLoadPlus []string          `toml:"plugins_load_plus,omitempty"`

	ProxyPorts map[string]int `toml:"proxy_ports,omitempty"`
	Env        Env            `toml:"env,omitempty"`

	Path string `toml:"-"`
}

// Load 读取 custom.toml。文件不存在时返回空配置，不视为错误。
func Load(path string) (*Config, error) {
	c := &Config{Path: path}

	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		c.init()
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := toml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	c.init()
	c.Path = path
	return c, nil
}

func (c *Config) init() {
	if c.Oxides == nil {
		c.Oxides = map[string]string{}
	}
	if c.PluginsPlus == nil {
		c.PluginsPlus = map[string]string{}
	}
	if c.ProxyPorts == nil {
		c.ProxyPorts = map[string]int{}
	}
}

// BackupRoot 返回备份根目录（未展开）。Windows 上优先用 oxide_folder_win。
func (c *Config) BackupRoot(isWin bool) string {
	if isWin && c.OxideFolderWin != "" {
		return c.OxideFolderWin
	}
	return c.OxideFolder
}

// EnvShortcuts 按平台返回环境别名表。
func (c *Config) EnvShortcuts(kind string, isWin bool) map[string]string {
	switch kind {
	case "conda":
		return c.Env.Conda
	case "julia":
		if isWin && len(c.Env.JuliaWin) > 0 {
			return c.Env.JuliaWin
		}
		return c.Env.Julia
	}
	return nil
}

// Marshal 把配置序列化成 TOML。
func (c *Config) Marshal() ([]byte, error) {
	c.Path = ""
	b, err := toml.Marshal(c)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Save 写回 TOML。
func (c *Config) Save(path string) error {
	b, err := c.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
