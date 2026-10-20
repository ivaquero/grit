package juliamgr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivaquero/grit/internal/custom"
)

func TestPkgExpr(t *testing.T) {
	cases := []struct {
		op   string
		pkgs []string
		want string
	}{
		{"add", []string{"A", "B"}, `using Pkg; Pkg.add(["A", "B"])`},
		{"rm", []string{"A"}, `using Pkg; Pkg.rm(["A"])`},
		{"pin", []string{"A"}, `using Pkg; Pkg.pin(["A"])`},
		{"free", []string{"A"}, `using Pkg; Pkg.free(["A"])`},
		{"build", []string{"A"}, `using Pkg; Pkg.build(["A"])`},
		{"update", nil, `using Pkg; Pkg.update()`},
		{"update", []string{"A"}, `using Pkg; Pkg.update(["A"])`},
		{"test", nil, `using Pkg; Pkg.test()`},
		{"test", []string{"A"}, `using Pkg; Pkg.test(["A"])`},
		{"status", nil, `using Pkg; Pkg.status()`},
		{"gc", nil, `using Pkg; Pkg.gc()`},
	}
	for _, c := range cases {
		got, err := PkgExpr(c.op, c.pkgs)
		if err != nil {
			t.Fatalf("%s: %v", c.op, err)
		}
		if got != c.want {
			t.Fatalf("%s: got %q want %q", c.op, got, c.want)
		}
	}
	if _, err := PkgExpr("frobnicate", nil); err == nil {
		t.Fatal("unknown operation must fail")
	}
}

// 包名里的引号与反斜杠必须转义，否则生成的 Julia 代码会截断。
func TestPkgExprEscapesNames(t *testing.T) {
	got, err := PkgExpr("add", []string{`a"b`, `c\d`})
	if err != nil {
		t.Fatal(err)
	}
	want := `using Pkg; Pkg.add(["a\"b", "c\\d"])`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// 短名、相对路径、绝对路径、@named 四种输入都要落到正确取值。
func TestResolve(t *testing.T) {
	dir := t.TempDir()
	want := filepath.ToSlash(dir) // 非 Windows shell 风格：正斜杠
	cu := &custom.Config{Env: custom.Env{Julia: map[string]string{"t": dir}}}

	got, err := Resolve("t", cu, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("shortcut: got %q want %q", got, want)
	}

	got, err = Resolve(dir, cu, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("path: got %q want %q", got, want)
	}

	got, err = Resolve("@v1.13", cu, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "@v1.13" {
		t.Fatalf("@named: got %q", got)
	}

	if _, err := Resolve(filepath.Join(dir, "nope"), cu, false); err == nil {
		t.Fatal("missing path must fail")
	}
}

// Windows 上优先用 julia_win 表。
func TestResolveUsesPlatformShortcuts(t *testing.T) {
	win := t.TempDir()
	unix := t.TempDir()
	cu := &custom.Config{Env: custom.Env{
		Julia:    map[string]string{"t": unix},
		JuliaWin: map[string]string{"t": win},
	}}

	got, err := Resolve("t", cu, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != win {
		t.Fatalf("got %q want %q", got, win)
	}
}

func TestProjectFileUsesDepot(t *testing.T) {
	depot := t.TempDir()
	t.Setenv("JULIA_DEPOT_PATH", depot+string(os.PathListSeparator)+"other")

	got, err := ProjectFile("@v1.13")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(depot, "environments", "v1.13", "Project.toml")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	got, err = ProjectFile(filepath.Join("a", "b"))
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join("a", "b", "Project.toml") {
		t.Fatalf("project dir: got %q", got)
	}

	if _, err := ProjectFile("@."); err == nil {
		t.Fatal("@. has no fixed project file")
	}
}

func TestLeaves(t *testing.T) {
	dir := t.TempDir()
	body := "[deps]\nB = \"uuid-b\"\nA = \"uuid-a\"\n"
	if err := os.WriteFile(filepath.Join(dir, "Project.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Leaves(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("got %v", got)
	}
}

// Manifest 用 [[deps.X]] 数组表，解析要按数组处理。
func TestCheckMaturity(t *testing.T) {
	dir := t.TempDir()
	project := "[deps]\nA = \"uuid-a\"\n"
	manifest := `[[deps.A]]
version = "0.4.0"

[[deps.B]]
version = "1.2.0"

[[deps.Stdlib]]
`
	if err := os.WriteFile(filepath.Join(dir, "Project.toml"), []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Manifest.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := CheckMaturity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Total != 2 || m.Immature != 1 {
		t.Fatalf("got total=%d immature=%d", m.Total, m.Immature)
	}
	if m.Rate() != 50 {
		t.Fatalf("rate = %d", m.Rate())
	}
	if (Maturity{}).Rate() != 100 {
		t.Fatal("empty manifest should count as mature")
	}
}

func TestOpsMetadata(t *testing.T) {
	if !Valid(OpAdd) || Valid("nope") {
		t.Fatal("Valid is wrong")
	}
	if !IsEnvOp(OpActivate) || IsEnvOp(OpAdd) {
		t.Fatal("IsEnvOp is wrong")
	}
	if !NeedsPackages(OpAdd) || NeedsPackages(OpUpdate) {
		t.Fatal("NeedsPackages is wrong")
	}
	if !TakesNoPackages(OpStatus) || TakesNoPackages(OpUpdate) {
		t.Fatal("TakesNoPackages is wrong")
	}
	for _, op := range Ops {
		if strings.TrimSpace(op) == "" {
			t.Fatal("empty operation in Ops")
		}
	}
}
