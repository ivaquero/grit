// Package convert 执行格式转换。
// 每条转换由 TOML 声明工具与参数，这里只负责选规则、算输出路径、拼参数。
package convert

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ivaquero/grit/internal/config"
)

// pdfEngines 按优先级探测。
var pdfEngines = []string{"tectonic", "xelatex", "lualatex", "pdflatex"}

// Run 执行一条转换。输出与输入同目录，扩展名替换为规则的 ext；
// 规则带 out 时改用固定文件名。
// file 可以为空，前提是该规则从模板生成结果、不需要输入（NeedsInput 为假）。
func Run(cv *config.Convert, file string, log io.Writer) error {
	if cv.Ext == "" || cv.Tool == "" {
		return fmt.Errorf("conversion %q is misconfigured: ext and tool are required", cv.Name)
	}

	abs := ""
	if file != "" {
		a, err := filepath.Abs(file)
		if err != nil {
			return err
		}
		info, err := os.Stat(a)
		if err != nil {
			return fmt.Errorf("cannot read %s: %w", file, err)
		}
		if info.IsDir() {
			return fmt.Errorf("%s is a directory", file)
		}
		abs = a
	}
	if abs == "" && cv.NeedsInput() {
		return fmt.Errorf("conversion %q needs an input file", cv.Name)
	}

	dst, err := OutputPathFor(abs, cv)
	if err != nil {
		return err
	}

	toolPath, err := exec.LookPath(cv.Tool)
	if err != nil {
		return fmt.Errorf("%s is required for %s but was not found in PATH", cv.Tool, cv.Name)
	}

	engine := ""
	if cv.Engine {
		if engine, err = findPDFEngine(); err != nil {
			return err
		}
	}

	args := expand(cv.Args, abs, dst, engine)

	if log != nil {
		src := "(built-in template)"
		if abs != "" {
			src = filepath.Base(abs)
		}
		fmt.Fprintf(log, "%s -> %s\n", src, filepath.Base(dst))
	}

	cmd := exec.Command(toolPath, args...)
	if cv.Stdout {
		// 工具把结果写到 stdout（marimo / pandoc --print-default-data-file）。
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		return os.WriteFile(dst, buf.Bytes(), 0o644)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// OutputPathFor 按规则的 out / ext 算输出路径。
// in 为空表示规则不需要输入，此时输出落在当前工作目录。
func OutputPathFor(in string, cv *config.Convert) (string, error) {
	if cv.Out == "" {
		if in == "" {
			return "", fmt.Errorf("conversion %q needs an input file", cv.Name)
		}
		return OutputPath(in, cv.Ext)
	}
	// 固定输出名只能是文件名：允许子路径会让 guard 与报告都变得含糊。
	if filepath.Base(cv.Out) != cv.Out {
		return "", fmt.Errorf("conversion %q: out must be a bare filename, got %q", cv.Name, cv.Out)
	}
	base := ""
	if in != "" {
		base = filepath.Dir(in)
	} else {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		base = wd
	}
	dst := filepath.Join(base, cv.Out)
	if in != "" && strings.EqualFold(dst, in) {
		return "", fmt.Errorf("refusing to overwrite the input file %s", filepath.Base(in))
	}
	return dst, nil
}

// OutputPath 返回同目录、扩展名被替换的路径。
// 若结果与原文件相同则报错：多数转换工具会直接覆盖，不能让它吃掉源文件。
func OutputPath(file, ext string) (string, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	dst := strings.TrimSuffix(abs, filepath.Ext(abs)) + "." + ext
	if strings.EqualFold(dst, abs) {
		return "", fmt.Errorf("refusing to overwrite the input file %s", filepath.Base(abs))
	}
	return dst, nil
}

// findPDFEngine 返回第一个可用的 pdf 引擎。
func findPDFEngine() (string, error) {
	for _, e := range pdfEngines {
		if _, err := exec.LookPath(e); err == nil {
			return e, nil
		}
	}
	return "", errors.New("no PDF engine found: install one of " + strings.Join(pdfEngines, ", "))
}

// expand 把模板参数里的 {in} / {out} / {engine} 替换成实际值。
func expand(args []string, in, out, engine string) []string {
	r := strings.NewReplacer("{in}", in, "{out}", out, "{engine}", engine)
	res := make([]string, len(args))
	for i, a := range args {
		res[i] = r.Replace(a)
	}
	return res
}
