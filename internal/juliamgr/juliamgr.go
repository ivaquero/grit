// Package juliamgr 实现 Julia 的环境管理与包操作。
//
// 它同时覆盖两层：
//   - L3（环境激活）：把项目路径写回父 shell 的 JULIA_PROJECT，
//     后续所有 julia 调用自动生效，不需要 --project=；
//   - L2（包操作）：把用户参数渲染成 Pkg 表达式再交给 julia 执行，
//     参数到 Julia 数组的转换无法用静态模板表达，必须在这里算。
//
// 「多前缀 + 复杂条件」指的是：同一个插件里有 jl 包操作、jleat 激活、
// jeq 退出三套命令名，而单 abbr 拼不出不规则的 jleat —— 所以环境动作
// 走 state 表的完整命令名，包操作走 abbr + key。
package juliamgr

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ivaquero/grit/internal/custom"
	"github.com/ivaquero/grit/internal/ospath"
	"github.com/pelletier/go-toml/v2"
)

// 环境动作：改变当前 shell 状态。
const (
	OpActivate   = "activate"
	OpDeactivate = "deactivate"
	OpCurrent    = "current"
	OpEnvs       = "envs"
)

// 包操作：交给 julia 子进程。
const (
	OpAdd      = "add"
	OpRm       = "rm"
	OpUpdate   = "update"
	OpPin      = "pin"
	OpFree     = "free"
	OpBuild    = "build"
	OpTest     = "test"
	OpStatus   = "status"
	OpGc       = "gc"
	OpLeaves   = "leaves"
	OpMaturity = "maturity"
)

// Ops 是执行器接受的全部操作，顺序即帮助与补全的顺序。
var Ops = []string{
	OpActivate, OpDeactivate, OpCurrent, OpEnvs,
	OpAdd, OpRm, OpUpdate, OpPin, OpFree, OpBuild, OpTest,
	OpStatus, OpGc, OpLeaves, OpMaturity,
}

// EnvOps 是走 state 通道、必须由调用方 eval 的操作。
var EnvOps = []string{OpActivate, OpDeactivate, OpCurrent, OpEnvs}

// IsEnvOp 判断操作是否属于环境动作。
func IsEnvOp(op string) bool {
	for _, e := range EnvOps {
		if e == op {
			return true
		}
	}
	return false
}

// Valid 判断操作名是否合法。
func Valid(op string) bool {
	for _, o := range Ops {
		if o == op {
			return true
		}
	}
	return false
}

// NeedsPackages 判断操作是否必须带至少一个包名。
func NeedsPackages(op string) bool {
	switch op {
	case OpAdd, OpRm, OpPin, OpFree, OpBuild:
		return true
	}
	return false
}

// TakesNoPackages 判断操作是否不接受包名。
func TakesNoPackages(op string) bool {
	switch op {
	case OpStatus, OpGc:
		return true
	}
	return false
}

// Resolve 把短名 / 相对路径 / 绝对路径 / @named 归一成 julia 能接受的取值。
//
// 判定顺序即「复杂条件」的落点：
//  1. 空 → 当前 Julia 版本的共享环境 @vX.Y；
//  2. @ 开头 → 原样交给 julia（@name / @. 由 julia 自己解析）；
//  3. 命中 [env.julia] 短名 → 展开成路径；
//  4. 其余按路径处理，相对路径基于用户主目录。
func Resolve(name string, cu *custom.Config, isWin bool) (string, error) {
	if name == "" {
		return defaultSpec()
	}
	if strings.HasPrefix(name, "@") {
		return name, nil
	}
	if cu != nil {
		if v, ok := cu.EnvShortcuts("julia", isWin)[name]; ok && v != "" {
			name = v
		}
	}
	spec := ospath.Expand(name, isWin)
	if spec == "" {
		return "", fmt.Errorf("empty julia environment")
	}
	info, err := os.Stat(spec)
	if err != nil {
		return "", fmt.Errorf("julia environment %q: %w", name, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("julia environment %q is not a directory", spec)
	}
	return spec, nil
}

// defaultSpec 返回当前 Julia 版本的共享环境。
// 用 julia 自己报版本，升级 Julia 之后不必改配置。
func defaultSpec() (string, error) {
	bin, err := exec.LookPath("julia")
	if err != nil {
		return "", fmt.Errorf("julia is required but was not found in PATH")
	}
	out, err := exec.Command(bin, "--startup-file=no", "-e",
		`print("v", VERSION.major, ".", VERSION.minor)`).Output()
	if err != nil {
		return "", fmt.Errorf("cannot determine the julia version: %w", err)
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", fmt.Errorf("julia returned an empty version")
	}
	return "@" + v, nil
}

// Shortcuts 返回当前平台的 [env.julia] 短名表。
func Shortcuts(cu *custom.Config, isWin bool) map[string]string {
	if cu == nil {
		return nil
	}
	return cu.EnvShortcuts("julia", isWin)
}

// ShortcutNames 返回排序后的短名，供补全与 envs 使用。
func ShortcutNames(cu *custom.Config, isWin bool) []string {
	m := Shortcuts(cu, isWin)
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ProjectFile 返回环境对应的 Project.toml 路径。
// @ 开头的环境在 depot 下，其余是项目目录。
func ProjectFile(spec string) (string, error) {
	if strings.HasPrefix(spec, "@") {
		name := strings.TrimPrefix(spec, "@")
		if name == "" || name == "." {
			return "", fmt.Errorf("%q has no fixed project file", spec)
		}
		return filepath.Join(depot(), "environments", name, "Project.toml"), nil
	}
	return filepath.Join(spec, "Project.toml"), nil
}

// depot 返回 JULIA_DEPOT_PATH 的第一项，缺省是 ~/.julia。
func depot() string {
	if v := os.Getenv("JULIA_DEPOT_PATH"); v != "" {
		return strings.Split(v, string(os.PathListSeparator))[0]
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".julia"
	}
	return filepath.Join(home, ".julia")
}

// PkgExpr 把操作与包名渲染成一条 Julia 表达式。
func PkgExpr(op string, pkgs []string) (string, error) {
	arr := juliaArray(pkgs)

	switch op {
	case OpAdd:
		return "using Pkg; Pkg.add(" + arr + ")", nil
	case OpRm:
		return "using Pkg; Pkg.rm(" + arr + ")", nil
	case OpPin:
		return "using Pkg; Pkg.pin(" + arr + ")", nil
	case OpFree:
		return "using Pkg; Pkg.free(" + arr + ")", nil
	case OpBuild:
		return "using Pkg; Pkg.build(" + arr + ")", nil
	case OpUpdate:
		if len(pkgs) == 0 {
			return "using Pkg; Pkg.update()", nil
		}
		return "using Pkg; Pkg.update(" + arr + ")", nil
	case OpTest:
		if len(pkgs) == 0 {
			return "using Pkg; Pkg.test()", nil
		}
		return "using Pkg; Pkg.test(" + arr + ")", nil
	case OpStatus:
		return "using Pkg; Pkg.status()", nil
	case OpGc:
		return "using Pkg; Pkg.gc()", nil
	}
	return "", fmt.Errorf("unknown julia package operation %q", op)
}

// juliaArray 生成 Julia 的字符串数组字面量。
func juliaArray(pkgs []string) string {
	quoted := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		q := strings.ReplaceAll(p, `\`, `\\`)
		q = strings.ReplaceAll(q, `"`, `\"`)
		quoted = append(quoted, `"`+q+`"`)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// Options 控制子进程的输入输出。
type Options struct {
	Spec  string
	Stdin io.Reader
	Out   io.Writer
	Err   io.Writer
}

// RunPackage 在指定环境下执行一条 Pkg 操作。
func RunPackage(op string, pkgs []string, opt Options) error {
	if opt.Out == nil {
		opt.Out = io.Discard
	}
	if opt.Err == nil {
		opt.Err = io.Discard
	}
	expr, err := PkgExpr(op, pkgs)
	if err != nil {
		return err
	}
	bin, err := exec.LookPath("julia")
	if err != nil {
		return fmt.Errorf("julia is required but was not found in PATH")
	}

	args := []string{"--startup-file=no", "--quiet"}
	if opt.Spec != "" {
		args = append(args, "--project="+opt.Spec)
	}
	args = append(args, "--eval", expr)

	fmt.Fprintf(opt.Err, "julia %s\n", expr)
	cmd := exec.Command(bin, args...)
	cmd.Stdin = opt.Stdin
	cmd.Stdout = opt.Out
	cmd.Stderr = opt.Err
	return cmd.Run()
}

// Leaves 读取 Project.toml 的 [deps]，返回直接依赖名。
func Leaves(spec string) ([]string, error) {
	path, err := ProjectFile(spec)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc struct {
		Deps map[string]string `toml:"deps"`
	}
	if err := toml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	out := make([]string, 0, len(doc.Deps))
	for k := range doc.Deps {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// Maturity 统计 Manifest 里 0.x 版本包的比例，用于判断环境是否成熟。
type Maturity struct {
	Total    int
	Immature int
}

// Rate 返回成熟度百分比。
func (m Maturity) Rate() int {
	if m.Total == 0 {
		return 100
	}
	return 100 - m.Immature*100/m.Total
}

// CheckMaturity 解析环境目录下的 Manifest.toml。
func CheckMaturity(spec string) (Maturity, error) {
	project, err := ProjectFile(spec)
	if err != nil {
		return Maturity{}, err
	}
	path := filepath.Join(filepath.Dir(project), "Manifest.toml")
	b, err := os.ReadFile(path)
	if err != nil {
		return Maturity{}, fmt.Errorf("read %s: %w", path, err)
	}
	// Julia 的 Manifest 用 [[deps.X]] 这种数组表，每个包恰好一项。
	var doc struct {
		Deps map[string][]struct {
			Version string `toml:"version"`
		} `toml:"deps"`
	}
	if err := toml.Unmarshal(b, &doc); err != nil {
		return Maturity{}, fmt.Errorf("parse %s: %w", path, err)
	}

	var m Maturity
	for _, entries := range doc.Deps {
		for _, d := range entries {
			if d.Version == "" {
				continue
			}
			m.Total++
			if strings.HasPrefix(d.Version, "0.") {
				m.Immature++
			}
		}
	}
	return m, nil
}
