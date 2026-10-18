package ospath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNorm(t *testing.T) {
	if got := Norm(`C:\a\b`, false); got != "C:/a/b" {
		t.Fatalf("posix style: %q", got)
	}
	if got := Norm("C:/a/b", true); got != filepath.FromSlash("C:/a/b") {
		t.Fatalf("windows style: %q", got)
	}
}

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	want := filepath.ToSlash(home) + "/Documents/x"
	if got := Expand("~/Documents/x", false); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// 相对路径基于用户主目录展开，与 goboot 原有语义一致。
func TestExpandRelative(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	want := filepath.ToSlash(home) + "/Documents/backup"
	if got := Expand("Documents/backup", false); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestExpandEmpty(t *testing.T) {
	if got := Expand("", false); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestJoin(t *testing.T) {
	if got := Join("C:/base", "a/b", false); got != "C:/base/a/b" {
		t.Fatalf("got %q", got)
	}
	// ../ 必须被清理掉，氧化物路径常写成 ../notes/xxx。
	if got := Join("C:/base/backup", "../notes/x", false); got != "C:/base/notes/x" {
		t.Fatalf("dotdot: %q", got)
	}
	if got := Join("", "a", false); got != "" {
		t.Fatalf("empty base: %q", got)
	}
}

func TestJoinWindowsStyle(t *testing.T) {
	got := Join("C:/base", "a/b", true)
	if got != filepath.FromSlash("C:/base/a/b") {
		t.Fatalf("got %q", got)
	}
}

func TestWithin(t *testing.T) {
	base := filepath.Join("a", "b")
	if !Within(base, filepath.Join(base, "c")) {
		t.Fatal("child should be within base")
	}
	if Within(base, filepath.Join("a", "b2")) {
		t.Fatal("sibling must not count as within")
	}
	if Within(base, filepath.Join(base, "..", "..", "etc")) {
		t.Fatal("escape must not count as within")
	}
}
