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

	"github.com/ivaquero/goboot/internal/config"
)

// pdfEngines 按优先级探测。
var pdfEngines = []string{"tectonic", "xelatex", "lualatex", "pdflatex"}

// Run 执行一条转换。输出与输入同目录，扩展名替换为规则的 ext。
func Run(cv *config.Convert, file string, log io.Writer) error {
	if cv.Ext == "" || cv.Tool == "" {
		return fmt.Errorf("conversion %q is misconfigured: ext and tool are required", cv.Name)
	}

	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", file, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", file)
	}

	dst, err := OutputPath(abs, cv.Ext)
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
		fmt.Fprintf(log, "%s -> %s\n", filepath.Base(abs), filepath.Base(dst))
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
