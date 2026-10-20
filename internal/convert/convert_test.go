package convert

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ivaquero/grit/internal/config"
)

func TestOutputPath(t *testing.T) {
	dir := t.TempDir()
	got, err := OutputPath(filepath.Join(dir, "notes.md"), "html")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "notes.html")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// 转换结果与原文件同名时必须拒绝，否则工具会覆盖源文件。
func TestOutputPathRejectsSameExtension(t *testing.T) {
	dir := t.TempDir()
	if _, err := OutputPath(filepath.Join(dir, "notes.md"), "md"); err == nil {
		t.Fatal("expected refusal when output equals input")
	}
}

func TestOutputPathWithoutExtension(t *testing.T) {
	dir := t.TempDir()
	got, err := OutputPath(filepath.Join(dir, "notes"), "html")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "notes.html") {
		t.Fatalf("got %q", got)
	}
}

func TestExpand(t *testing.T) {
	got := expand(
		[]string{"{in}", "-o", "{out}", "--pdf-engine={engine}"},
		"a.md", "a.pdf", "tectonic",
	)
	want := []string{"a.md", "-o", "a.pdf", "--pdf-engine=tectonic"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d: got %q want %q", i, got[i], want[i])
		}
	}
}

// 参数里没有占位符时不能被改动。
func TestExpandLeavesLiteral(t *testing.T) {
	got := expand([]string{"--to", "md", "{in}"}, "x.ipynb", "x.md", "")
	if got[0] != "--to" || got[1] != "md" || got[2] != "x.ipynb" {
		t.Fatalf("got %v", got)
	}
}

// 固定输出名：输出落在输入所在目录，与扩展名无关。
func TestOutputPathForFixedOut(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cv := &config.Convert{Name: "pdref", Ext: "docx", Out: "custom-reference.docx"}
	got, err := OutputPathFor(in, cv)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "custom-reference.docx") {
		t.Fatalf("got %q", got)
	}
}

// 不需要输入的规则可以不带 file，输出落在当前目录。
func TestOutputPathForFixedOutWithoutInput(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cv := &config.Convert{Name: "pdref", Ext: "docx", Out: "custom-reference.docx"}
	got, err := OutputPathFor("", cv)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(wd, "custom-reference.docx") {
		t.Fatalf("got %q", got)
	}
}

func TestOutputPathForFixedOutRejectsSubpath(t *testing.T) {
	cv := &config.Convert{Name: "x", Ext: "docx", Out: filepath.Join("sub", "ref.docx")}
	if _, err := OutputPathFor("", cv); err == nil {
		t.Fatal("out with a directory component must be rejected")
	}
}

// 固定输出名恰好等于输入文件名时同样要拒绝。
func TestOutputPathForFixedOutRejectsOverwrite(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "ref.docx")
	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cv := &config.Convert{Name: "x", Ext: "docx", Out: "ref.docx"}
	if _, err := OutputPathFor(in, cv); err == nil {
		t.Fatal("expected refusal when the fixed output equals the input")
	}
}

func TestNeedsInput(t *testing.T) {
	if !(&config.Convert{Ext: "md"}).NeedsInput() {
		t.Fatal("a rule without out needs an input")
	}
	template := &config.Convert{Ext: "docx", Out: "ref.docx", Args: []string{"--print-default-data-file", "reference.docx"}}
	if template.NeedsInput() {
		t.Fatal("a template rule does not need an input")
	}
	hybrid := &config.Convert{Ext: "pdf", Out: "ref.pdf", Args: []string{"{in}"}}
	if !hybrid.NeedsInput() {
		t.Fatal("a rule that references {in} still needs an input")
	}
}
