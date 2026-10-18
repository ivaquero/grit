package convert

import (
	"path/filepath"
	"testing"
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
