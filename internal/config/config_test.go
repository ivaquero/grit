package config

import "testing"

func TestConvertForPrefersMatchingInExt(t *testing.T) {
	c := &Config{Converts: []Convert{
		{Name: "tomd", Ext: "md", Tool: "pandoc"},
		{Name: "tomd", Ext: "md", Tool: "jupytext", InExt: "ipynb"},
	}}

	got, ok := c.ConvertFor("tomd", "ipynb")
	if !ok {
		t.Fatal("expected a rule for .ipynb")
	}
	if got.Tool != "jupytext" {
		t.Fatalf("got tool %q, want jupytext", got.Tool)
	}

	got, ok = c.ConvertFor("tomd", "rst")
	if !ok {
		t.Fatal("expected the fallback rule")
	}
	if got.Tool != "pandoc" {
		t.Fatalf("got tool %q, want pandoc", got.Tool)
	}
}

// 只有 in_ext 规则时，不匹配的输入不应命中。
func TestConvertForNoFallback(t *testing.T) {
	c := &Config{Converts: []Convert{
		{Name: "ipynb2py", Ext: "py", Tool: "marimo", InExt: "ipynb"},
	}}
	if _, ok := c.ConvertFor("ipynb2py", "md"); ok {
		t.Fatal("a rule with in_ext must not match other extensions")
	}
	if _, ok := c.ConvertFor("unknown", "md"); ok {
		t.Fatal("unknown name must not match")
	}
}

func TestConvertForEmptyConfig(t *testing.T) {
	c := &Config{}
	if _, ok := c.ConvertFor("tohtml", "md"); ok {
		t.Fatal("empty config must not match anything")
	}
}
