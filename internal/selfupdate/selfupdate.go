// Package selfupdate 用 GitHub Release 更新 grit 本体。
//
// 它不依赖 git，也不要求安装目录是仓库：从 release 列表取最新 tag，
// 下载对应平台的归档，用 sha256sums.txt 校验，再把二进制换掉。
// 这样独立分发模式（~/.config/grit + ~/bin/grit）和仓库模式走同一条路径。
//
// 插件声明已经 //go:embed 进二进制，磁盘上不再有需要同步的插件目录，
// 所以这里只负责本体。
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// defaultRepo 是发布仓库；fork 或镜像用 GRIT_REPO 覆盖。
const defaultRepo = "ivaquero/grit"

// DefaultAPIBase 是 GitHub API 的根地址；企业版或镜像可覆盖。
const DefaultAPIBase = "https://api.github.com"

const (
	maxDownload = 128 << 20 // 归档上限，防止异常响应把内存吃满
	maxBinary   = 64 << 20  // 解压后二进制上限
)

// Options 控制本次更新。
type Options struct {
	Repo       string    // owner/name；空则取 GRIT_REPO，再退回 defaultRepo
	Version    string    // 目标 tag（可带 v）；空表示最新正式版
	Current    string    // 当前版本，用于比较
	Force      bool      // 同版本也重装
	DryRun     bool      // 只报告将做什么
	Out        io.Writer // 进度输出
	APIBase    string    // 测试与镜像用；空则 DefaultAPIBase
	GOOS       string    // 空则 runtime.GOOS
	GOARCH     string    // 空则 runtime.GOARCH
	Executable string    // 空则 os.Executable()
	Client     *http.Client
}

type release struct {
	TagName    string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []asset `json:"assets"`
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// httpError 保留状态码，好把 404 翻译成「这个版本/仓库不存在」。
type httpError struct {
	URL    string
	Code   int
	Status string
	Body   string
}

func (e *httpError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("%s: %s", e.URL, e.Status)
	}
	return fmt.Sprintf("%s: %s: %s", e.URL, e.Status, e.Body)
}

// Run 执行一次自更新。
func Run(opt Options) error {
	if opt.Out == nil {
		opt.Out = io.Discard
	}

	repo, err := resolveRepo(opt.Repo)
	if err != nil {
		return err
	}
	goos := opt.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opt.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	api := strings.TrimRight(opt.APIBase, "/")
	if api == "" {
		api = DefaultAPIBase
	}
	client := opt.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	rel, err := fetchRelease(client, api, repo, opt.Version)
	if err != nil {
		return err
	}
	tag := rel.TagName
	ver := strings.TrimPrefix(tag, "v")

	if !opt.Force && Compare(opt.Current, ver) >= 0 {
		fmt.Fprintf(opt.Out, "grit: %s is up to date (latest %s)\n", opt.Current, tag)
		return nil
	}
	fmt.Fprintf(opt.Out, "grit: %s -> %s\n", opt.Current, tag)

	name := assetName(ver, goos, goarch)
	archiveURL, err := assetURL(rel, name)
	if err != nil {
		return err
	}
	sumsURL, err := assetURL(rel, "sha256sums.txt")
	if err != nil {
		return err
	}

	exe, err := resolveExecutable(opt.Executable)
	if err != nil {
		return err
	}
	if opt.DryRun {
		fmt.Fprintf(opt.Out, "[dry] would download %s and replace %s\n", name, exe)
		return nil
	}

	fmt.Fprintf(opt.Out, "downloading %s\n", name)
	archive, err := fetchBytes(client, archiveURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", name, err)
	}
	sums, err := fetchBytes(client, sumsURL)
	if err != nil {
		return fmt.Errorf("download sha256sums.txt: %w", err)
	}

	want := parseChecksums(sums)[name]
	if want == "" {
		return fmt.Errorf("%s is not listed in sha256sums.txt", name)
	}
	if got := sha256Hex(archive); got != want {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", name, got, want)
	}

	bin, err := extractBinary(archive, goos)
	if err != nil {
		return fmt.Errorf("extract %s: %w", name, err)
	}
	if err := install(exe, bin); err != nil {
		return err
	}
	fmt.Fprintf(opt.Out, "installed %s (%s)\n", exe, tag)
	return nil
}

// resolveRepo 校验 owner/name 形式，避免 GRIT_REPO 被拼进 URL 时带出意外路径。
func resolveRepo(explicit string) (string, error) {
	repo := explicit
	if repo == "" {
		repo = os.Getenv("GRIT_REPO")
	}
	if repo == "" {
		repo = defaultRepo
	}
	if !regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`).MatchString(repo) {
		return "", fmt.Errorf("invalid repository %q (want owner/name)", repo)
	}
	// 正则允许点号，但 . / .. 会被拼进 URL 变成路径穿越。
	for _, part := range strings.Split(repo, "/") {
		if part == "." || part == ".." {
			return "", fmt.Errorf("invalid repository %q (want owner/name)", repo)
		}
	}
	return repo, nil
}

// fetchRelease 取目标 release：指定版本走 tag 端点，否则取最新正式版。
func fetchRelease(client *http.Client, api, repo, want string) (*release, error) {
	if want != "" {
		tag := want
		if !strings.HasPrefix(tag, "v") {
			tag = "v" + tag
		}
		endpoint := fmt.Sprintf("%s/repos/%s/releases/tags/%s", api, repo, url.PathEscape(tag))
		var rel release
		if err := getJSON(client, endpoint, &rel); err != nil {
			if isNotFound(err) {
				return nil, fmt.Errorf("release %s not found in %s", tag, repo)
			}
			return nil, err
		}
		return &rel, nil
	}

	endpoint := fmt.Sprintf("%s/repos/%s/releases?per_page=30", api, repo)
	var list []release
	if err := getJSON(client, endpoint, &list); err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("repository %s not found", repo)
		}
		return nil, err
	}
	for i := range list {
		r := &list[i]
		// 与安装脚本同一口径：只认 v 开头的正式版。
		if r.Draft || r.Prerelease || !isStableTag(r.TagName) {
			continue
		}
		return r, nil
	}
	return nil, fmt.Errorf("no stable release found for %s", repo)
}

func isStableTag(tag string) bool {
	if len(tag) < 2 || tag[0] != 'v' {
		return false
	}
	_, err := strconv.Atoi(string(tag[1]))
	return err == nil
}

// assetName 拼出归档名，与 .goreleaser.yaml 的 name_template 一致。
func assetName(version, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("grit_%s_%s_%s%s", strings.TrimPrefix(version, "v"), goos, goarch, ext)
}

func assetURL(rel *release, name string) (string, error) {
	for _, a := range rel.Assets {
		if a.Name == name {
			if a.URL == "" {
				break
			}
			return a.URL, nil
		}
	}
	return "", fmt.Errorf("release %s has no asset %s", rel.TagName, name)
}

// Compare 比较版本号，返回 -1 / 0 / 1。
// 只比较前三段数字；带 -dev / -next / -rc 后缀的版本低于同号正式版。
func Compare(a, b string) int {
	an, apre := parseVersion(a)
	bn, bpre := parseVersion(b)
	for i := range an {
		if an[i] != bn[i] {
			if an[i] < bn[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case apre == "" && bpre != "":
		return 1
	case apre != "" && bpre == "":
		return -1
	default:
		return strings.Compare(apre, bpre)
	}
}

func parseVersion(v string) ([3]int, string) {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	var out [3]int
	core, pre := v, ""
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core, pre = core[:i], core[i+1:]
	}
	for i, part := range strings.Split(core, ".") {
		if i >= len(out) {
			break
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return out, pre
		}
		out[i] = n
	}
	return out, pre
}

func getJSON(client *http.Client, endpoint string, v any) error {
	body, err := fetchBytes(client, endpoint)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("parse %s: %w", endpoint, err)
	}
	return nil
}

func fetchBytes(client *http.Client, endpoint string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "grit-selfupdate")
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, &httpError{
			URL:    endpoint,
			Code:   resp.StatusCode,
			Status: resp.Status,
			Body:   strings.TrimSpace(string(snippet)),
		}
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxDownload))
}

func isNotFound(err error) bool {
	var he *httpError
	return errors.As(err, &he) && he.Code == http.StatusNotFound
}

func parseChecksums(b []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		// sha256sum 的二进制模式会在文件名前加 *。
		out[strings.TrimPrefix(f[1], "*")] = strings.ToLower(f[0])
	}
	return out
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// extractBinary 从归档里取出可执行文件。只认文件名，忽略目录。
func extractBinary(data []byte, goos string) ([]byte, error) {
	want := "grit"
	if goos == "windows" {
		want = "grit.exe"
		return fromZip(data, want)
	}
	return fromTarGz(data, want)
}

func fromTarGz(data []byte, want string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != want {
			continue
		}
		return io.ReadAll(io.LimitReader(tr, maxBinary))
	}
	return nil, fmt.Errorf("archive does not contain %s", want)
}

func fromZip(data []byte, want string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) != want {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, maxBinary))
	}
	return nil, fmt.Errorf("archive does not contain %s", want)
}

// resolveExecutable 返回要被替换的真实路径。
// 解析符号链接：装在 ~/.local/bin 的链接应当替换它指向的文件。
func resolveExecutable(explicit string) (string, error) {
	exe := explicit
	if exe == "" {
		e, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("cannot locate the running binary: %w", err)
		}
		exe = e
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Abs(exe)
}

// install 把新二进制写到目标位置。
// 先落到同目录临时文件，保证 rename 不跨文件系统。
func install(exe string, data []byte) error {
	dir := filepath.Dir(exe)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(exe)+".*")
	if err != nil {
		return fmt.Errorf("create temp binary: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil && runtime.GOOS != "windows" {
		return err
	}
	return swap(exe, tmpName)
}

// swap 用新文件替换旧文件。
// Windows 不允许覆盖运行中的 exe，但允许改名：先把旧文件挪开再落位。
func swap(exe, newPath string) error {
	if runtime.GOOS != "windows" {
		return os.Rename(newPath, exe)
	}

	// 旧文件可能仍被另一个 shell 里运行的进程占着，先尽力清一遍。
	cleanupOld(exe)

	old := exe + ".old"
	if err := os.Rename(exe, old); err != nil {
		// 目标被占用（另一个旧进程还没退出）：换一个不会撞的名字，
		// 否则这次更新就被一个旁观进程卡死了。
		old = fmt.Sprintf("%s.old-%d", exe, os.Getpid())
		if err := os.Rename(exe, old); err != nil {
			return fmt.Errorf("move the current binary aside: %w", err)
		}
	}
	if err := os.Rename(newPath, exe); err != nil {
		_ = os.Rename(old, exe) // 回滚，别把用户的 grit 弄丢
		return fmt.Errorf("install the new binary: %w", err)
	}
	// 本进程还在运行 .old，删不掉是正常的，下次更新会清。
	_ = os.Remove(old)
	return nil
}

// cleanupOld 删除上一次更新留下的 .old 文件。
// 被其他进程占用的会静默失败，留到下次。
func cleanupOld(exe string) {
	dir := filepath.Dir(exe)
	prefix := filepath.Base(exe) + ".old"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
}
