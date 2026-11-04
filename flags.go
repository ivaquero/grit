package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ivaquero/grit/internal/custom"
	"github.com/ivaquero/grit/internal/shell"
)

// 本文件收敛三件此前散落在各 cmd* 里、各写一份且行为不一致的事：
//   1. flagSet —— 统一的参数解析，取代 8 处手写 `for i := range args { switch … i++ }`；
//   2. resolveShell —— 「--shell 显式 > GRIT_SHELL > bash」这条回退链的单一定义；
//   3. resolveContext —— 「目标 shell → 路径风格 → 配置目录 → 用户配置」这条推导链，
//      避免每个命令各推一遍、甚至写死 gritRoot(false) 造成不一致。

// flagSet 是最小但一致的参数解析器。规则对所有子命令统一：
//   - 已登记的 flag（含别名）按 bool / value 两类消费；
//   - 其余 token 收进 Pos（位置参数）；
//   - 未登记、且以 "-" 开头的 token（长度 > 1）视为拼写错误直接报错，
//     这正是此前「有的命令忽略未知 flag、有的报错」被统一的地方；
//   - 单独的 "-" 与 "--" 之后的一切都按位置参数处理，给需要透传短横线参数的
//     场景（如给 julia 传字面量）留出口。
type flagSet struct {
	tokenKind map[string]uint8 // token -> kindFlagValue / kindFlagBool
	tokenName map[string]string
	bools     map[string]bool
	vals      map[string]string
	Pos       []string
}

const (
	kindFlagBool uint8 = iota
	kindFlagValue
)

func newFlagSet() *flagSet {
	return &flagSet{
		tokenKind: map[string]uint8{},
		tokenName: map[string]string{},
		bools:     map[string]bool{},
		vals:      map[string]string{},
	}
}

// Bool 登记一个布尔开关：canonical 是读取用的名字，aliases 是额外接受的写法。
func (f *flagSet) Bool(canonical string, aliases ...string) *flagSet {
	f.register(kindFlagBool, canonical, append([]string{canonical}, aliases...))
	return f
}

// Value 登记一个带值的开关，例如 --shell bash / -s bash。
func (f *flagSet) Value(canonical string, aliases ...string) *flagSet {
	f.register(kindFlagValue, canonical, append([]string{canonical}, aliases...))
	return f
}

func (f *flagSet) register(kind uint8, canonical string, tokens []string) {
	for _, t := range tokens {
		f.tokenKind[flagToken(t)] = kind
		f.tokenName[flagToken(t)] = canonical
	}
}

// flagToken 把声明里的规范名/别名归一化成命令行上真正出现的写法：
// 不带前导短横线的单字符补 "-"（n -> -n），多字符补 "--"（dry-run -> --dry-run）；
// 已经带短横线的别名（如 "-s"）原样保留。
// canonical 仍以不带短横线的形式存进 tokenName，has/value 就按它读取。
func flagToken(name string) string {
	if strings.HasPrefix(name, "-") {
		return name
	}
	if len(name) == 1 {
		return "-" + name
	}
	return "--" + name
}

// parse 消费 args，填充 bools / vals / Pos。遇到未登记的短横线开关返回错误。
func (f *flagSet) parse(args []string) error {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			f.Pos = append(f.Pos, args[i+1:]...)
			return nil
		}
		kind, known := f.tokenKind[a]
		if !known {
			if len(a) > 1 && a[0] == '-' {
				return fmt.Errorf("unknown flag %q", a)
			}
			f.Pos = append(f.Pos, a)
			continue
		}
		if kind == kindFlagValue {
			if i+1 >= len(args) {
				return fmt.Errorf("%s needs a value", a)
			}
			f.vals[f.tokenName[a]] = args[i+1]
			i++
			continue
		}
		f.bools[f.tokenName[a]] = true
	}
	return nil
}

func (f *flagSet) has(name string) bool     { return f.bools[name] }
func (f *flagSet) value(name string) string { return f.vals[name] }

// rejectPositional 给「不接受位置参数」的命令复用：出现多余 token 就报错。
func (f *flagSet) rejectPositional() error {
	if len(f.Pos) > 0 {
		return fmt.Errorf("unexpected argument %q", f.Pos[0])
	}
	return nil
}

// resolveShell 按显式 --shell > GRIT_SHELL > bash 取目标 shell。
// --shell 的提取交给各命令的 flagSet，这里只负责回退优先级。
func resolveShell(explicit string) (shell.Shell, error) {
	if explicit == "" {
		explicit = os.Getenv("GRIT_SHELL")
	}
	if explicit == "" {
		explicit = "bash"
	}
	return shell.Get(explicit)
}

// runtimeContext 是「需要写文件 / 读配置」的命令共同的运行期前提。
type runtimeContext struct {
	sh    shell.Shell
	isWin bool
	root  string
	cu    *custom.Config
}

// resolveContext 走一遍统一推导链：目标 shell → 是否 Windows 风格 → GRIT 根目录 → custom.toml。
// shellName 传 "" 表示按运行期（GRIT_SHELL）解析；传具体值表示 --shell 显式指定。
func resolveContext(shellName string) (*runtimeContext, error) {
	sh, err := resolveShell(shellName)
	if err != nil {
		return nil, err
	}
	isWin := sh.Name() == "pwsh"
	root := gritRoot(isWin)
	cu, err := custom.Load(filepath.Join(root, "custom.toml"))
	if err != nil {
		return nil, err
	}
	return &runtimeContext{sh: sh, isWin: isWin, root: root, cu: cu}, nil
}
