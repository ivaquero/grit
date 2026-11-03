package convert

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivaquero/grit/internal/config"
)

// convert.Run 此前只有纯函数被覆盖，实际执行入口 Run 的错误分支是空白。
// 这些分支不依赖 pandoc/ffmpeg 等外部工具是否存在，可以确定性测试。

func TestRunRejectsMisconfiguredRule(t *testing.T) {
	// 缺 ext 或缺 tool 的规则无法执行，应在触碰文件系统前就拒绝。
	cases := []*config.Convert{
		{Name: "x", Tool: "pandoc"}, // 无 ext
		{Name: "y", Ext: "html"},    // 无 tool
		{Name: "z"},                 // 两者皆无
	}
	for _, cv := range cases {
		if err := Run(cv, "", io.Discard); err == nil || !strings.Contains(err.Error(), "misconfigured") {
			t.Fatalf("%+v: want misconfigured error, got %v", cv, err)
		}
	}
}

func TestRunRejectsMissingInputFile(t *testing.T) {
	cv := &config.Convert{Name: "tohtml", Ext: "html", Tool: "pandoc"}
	missing := filepath.Join(t.TempDir(), "nope.md")
	if err := Run(cv, missing, io.Discard); err == nil || !strings.Contains(err.Error(), "cannot read") {
		t.Fatalf("want 'cannot read' for missing file, got %v", err)
	}
}

func TestRunRejectsDirectoryInput(t *testing.T) {
	cv := &config.Convert{Name: "tohtml", Ext: "html", Tool: "pandoc"}
	dir := t.TempDir()
	if err := Run(cv, dir, io.Discard); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("want 'is a directory', got %v", err)
	}
}

// 不带 file、而规则又需要输入时，必须在查找工具之前就报缺输入。
func TestRunNeedsInputWhenFileEmpty(t *testing.T) {
	cv := &config.Convert{Name: "tohtml", Ext: "html", Tool: "pandoc", Args: []string{"{in}"}}
	if err := Run(cv, "", io.Discard); err == nil || !strings.Contains(err.Error(), "needs an input file") {
		t.Fatalf("want 'needs an input file', got %v", err)
	}
}

// 依赖的外部工具不在 PATH 时，给出明确的「需要该工具」错误，而不是模糊的 exec 失败。
func TestRunMissingToolInPath(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(in, []byte("# hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	cv := &config.Convert{
		Name: "tohtml", Ext: "html", Tool: "grit-nonexistent-tool-xyz",
		Args: []string{"{in}", "-o", "{out}"},
	}
	err := Run(cv, in, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not found in PATH") {
		t.Fatalf("want 'not found in PATH', got %v", err)
	}
	if !strings.Contains(err.Error(), "grit-nonexistent-tool-xyz") {
		t.Fatalf("error should name the missing tool, got %v", err)
	}
}
