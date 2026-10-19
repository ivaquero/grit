# Grit

> 取自 grit（砂砾）：磨料自己不上台面，却决定上层工具的锋利度。这个工具做的是同一件事——把 shell 装配这层磨平，日常敲击直接落在原生命令上。
>
> 前身是 `oxidizer` 的 v2 重写版，曾用名 `goboot`；CLI 名即 `grit`。入口是 shell 启动时执行 `eval "$(grit init)"`，与 Go 的 `init()` 同一个语义：包被加载时自动装配。

目标：**消灭 `.sh` / `.ps1` 双实现**。逻辑只存在 Go 里，shell 代码全部由 `grit init` 生成。

## 日常开发全景

装配只发生在 shell 启动那一刻：`grit init` 把 TOML 一次性翻成纯 shell 代码，之后命令都在启动脚本内部展开，不再逐次读配置。

```mermaid
flowchart LR
    subgraph decl["声明式配置 · 全部内嵌进二进制"]
        d1["defaults/*.toml<br/>alias / registry / oxygen / convert / plugins"]
        d2["custom.toml<br/>oxides / 代理端口 / 平台开关"]
    end

    g["grit init --shell bash / zsh / pwsh"]

    d1 --> g
    d2 --> g
    g --> s["shell 启动脚本<br/>eval 一次，此后不再变"]

    s --> l1["L1 原生 alias"]
    s --> l2["L2 命令短名<br/>epf / ipf / iif / ppgf"]
    s --> l3["L3 state 包装<br/>ceat / ceq / pxy"]

    l1 --> u1["直接 exec 原生工具<br/>零额外进程"]
    l2 --> u2["短名展开为 grit 子命令<br/>file / conv"]
    l3 --> u3["取回 shell 代码再 eval<br/>环境 / 工作目录 / 代理"]

    classDef hot fill:#fde68a,stroke:#b45309,color:#111827
    classDef cold fill:#e0e7ff,stroke:#4338ca,color:#111827
    class g,s cold
    class u1,u2,u3 hot
```

日常开发分两条时间线：

1. **冷路径** —— 改了 `custom.toml` 或 `defaults/*.toml`、跑过 `grit self-update` 之后，重跑一次 `grit init`。不重生成，改动就不生效。
2. **热路径** —— `ll`（L1，纯 alias，零额外进程）、`epf g`（L2，一次短命 `grit file`）、`ceat kaggle`（L3，一次 `grit state` + `eval`）。日常敲击绝大多数落在 L1。

## 边界：什么必须留在 shell 里

子进程无法修改父进程的环境变量、工作目录与 alias。所以下面这些**永远**是 shell 代码：

| 能力 | 原因 |
|---|---|
| `cd` / `Set-Location` | 改父进程工作目录 |
| `export` / `$env:` | 改父进程环境 |
| `alias` / `function` 定义 | 注册进父进程 |
| `source` / `.` | 在当前 shell 执行 |
| `history` 清理 | shell 内建状态 |

这部分收敛为每平台一个生成物，人工不再维护。

## 三层结构

| 层 | 形式 | 例子 |
|---|---|---|
| L1 纯转发 | TOML `[[alias]]` → native alias | `ll` `bis` `cis` |
| L2 计算型 | Go 子命令，子进程执行 | `epf` `ipf` `iif` `ppgf` |
| L3 状态型 | `eval "$(grit state ...)"` | `ceat` `ceq` `pxy` |

判定标准只有一条：**是否改变当前 shell 状态**。

## 目录

```text
├── main.go                     CLI：init / state / file / shell / migrate / self-update / list
├── internal/complete/           shell 补全脚本与候选
├── internal/config/             TOML 模型、加载、冲突校验
├── internal/convert/            格式转换执行器
├── internal/custom/             用户配置 custom.toml
├── internal/filemgr/            L2 四个文件动作
├── internal/migrate/            custom.json → custom.toml
├── internal/ospath/             路径展开、拼接、风格归一
├── internal/selfupdate/         接管 upox
├── internal/shell/              bash/zsh 与 pwsh 的语法渲染
├── .goreleaser.yaml            发布配置：六目标 / 归档 / checksum / changelog
├── .github/workflows/          ci.yml（测试 + snapshot 构建）、release.yml（tag 发布）
├── install.sh / install.ps1     二进制安装器
└── defaults/
    ├── aliases.toml             L1 别名（含 L2 的短名）
    ├── registry.toml            GRIT_ELEMENT 平台路径
    ├── oxygen.toml              catalyze 的默认模板表
    ├── convert.toml             格式转换规则
    ├── files/                   默认配置模板本体
    └── plugins/*.toml           包管理器与工具的能力矩阵
```

测试与被测包同目录（Go 惯例），`xxx_test.go` 可以直接验证未导出实现。跨包复用的路径逻辑集中在 `ospath`：`Join`（按目标 shell 风格拼接）与 `Within`（路径包含判断），`filemgr` 与 `selfupdate` 共用，不再各自实现一份。

`defaults/` 通过 `//go:embed` 编进二进制，`grit` 因此可独立分发。

## 四张表

L2 的全部逻辑就是这四张表之间怎么配对。键带各自前缀，动作决定方向：

| 动作 | 源 | 目标 | 短名 |
|---|---|---|---|
| `backup` | `element[key]` | `backup/oxides[bk+key]` | `epf` |
| `restore` | `backup/oxides[bk+key]` | `element[key]` | `ipf` |
| `catalyze` | `oxygen[gr+key]` | `element[key]` | `iif` |
| `propagate` | `oxygen[gr+key]` | `backup/oxides[bk+key]` | `ppgf` |

- `element` 来自 `registry.toml`（真实配置路径，平台相关）
- `oxides` 来自 `custom.toml`（备份相对路径）
- `oxygen` 来自 `oxygen.toml`（默认模板，相对 GRIT）
- 备份根来自 `custom.toml` 的 `oxide_folder` / `oxide_folder_win`

```mermaid
flowchart LR
    o["oxygen<br/>oxygen.toml<br/>默认模板"]
    e["element<br/>registry.toml<br/>真实配置路径"]
    x["oxides<br/>备份根 + bk 前缀"]

    o -->|"catalyze · iif"| e
    e -->|"backup · epf"| x
    x -->|"restore · ipf"| e
    o -->|"propagate · ppgf"| x
```

四条边就是四个动作，方向即语义：`oxygen` 只出不进，`element` 是唯一的落地点，`oxides` 只做归档。

目录与文件由 `os.Stat` 判定，不依赖 `_` 后缀约定；目录复制前会清空目标。

## 插件矩阵

15 个插件，命令名 = `abbr` + `actions` 的 key。

| 插件 | abbr | 平台 | 覆盖 |
|---|---|---|---|
| `brew` | `b` | — | install / uninstall / upgrade / list / leaves / search / info / cleanup |
| `scoop` | `s` | — | 同上 + update all |
| `conda` | `c` | — | 包管理 7 项 + `ceat` / `ceq` 环境激活（L3） |
| `npm` | `n` | — | 17 项，含 project 动作 |
| `tlmgr` | `tl` | — | TeXLive 11 项 |
| `cargo` | `cg` | — | 16 项，含 build / run / test / publish |
| `rustup` | `rs` | — | component add / remove / list / update |
| `winget` | `w` | windows | 12 项，含 source 管理 |
| `wsl` | `wsl` | windows | install / uninstall / list / list-online |
| `apt` | `a` | linux | 13 项，含 depends 与 source 管理 |
| `mas` | `m` | darwin | 7 项 |
| `ollama` | `ol` | — | 9 项 |
| `vscode` | `vs` | — | 扩展 install / uninstall / list |
| `proxy` | `p` | — | `pxy` 设置/清除代理（L3） |
| `demo` | `d` | — | state 通道的验证样例 |

平台过滤用 `runtime.GOOS`（命令是否存在由宿主决定），与路径风格用目标 shell 决定是两件事。

默认生成全部插件 —— 命令暂时不在 PATH 不等于用户不需要那个别名（`conda` 常出现在非标准安装路径）。`--only-installed` 会按 `requires` 过滤，并把跳过原因打到 stderr。因此 `custom.toml` 的 `plugins_load` 已弃用，只作为迁移数据保留。

## 格式转换

`grit conv <name> <file>`：输出与输入同目录，扩展名替换。规则全在 `convert.toml`，加一条转换不需要碰代码。

| 命令 | 输出 | 工具 |
|---|---|---|
| `tohtml` | `.html` | pandoc |
| `todocx` | `.docx` | pandoc |
| `totyp` | `.typ` | pandoc |
| `topdf` | `.pdf` | pandoc + 引擎探测 |
| `tomd` | `.md` | pandoc（`.ipynb` 走 jupytext） |
| `toipynb` | `.ipynb` | jupytext |
| `topng` / `tojpg` | `.png` / `.jpg` | magick |
| `tomp3` / `tomp4` | `.mp3` / `.mp4` | ffmpeg |
| `ipynb2py` / `py2html` | `.py` / `.html` | marimo |

三个字段表达差异，不需要为此写分支代码：

- `in_ext`：同名多条规则时按输入扩展名精确匹配（`tomd` 对 `.ipynb` 切到 jupytext）
- `engine`：按 tectonic → xelatex → lualatex → pdflatex 顺序探测
- `stdout`：工具把结果写到 stdout，由 grit 落盘（marimo 属于这种）

输出路径与原文件同名时直接拒绝 —— 否则 pandoc 这类工具会吃掉源文件。

## Shell 补全

```sh
eval "$(grit completion --shell "${SHELL##*/}")"
```

```powershell
grit completion --shell pwsh | Out-String | Invoke-Expression
```

**补全脚本是静态的，候选值是动态的**：脚本只生成一次，每次按 TAB 调 `grit complete <context>` 取当前配置。新增转换、element 或插件之后不需要重新生成脚本。

| 补全点 | 候选来源 |
|---|---|
| `grit <TAB>` | 子命令 |
| `grit file <TAB>` | backup / restore / catalyze / propagate |
| `grit file backup <TAB>` | 当前平台的 element key |
| `grit conv <TAB>` | 转换名（去重后） |
| `epf` / `ipf` / `iif` / `ppgf <TAB>` | element key |
| `pxy <TAB>` | `proxy_ports` 的别名 |

PowerShell 的原生命令补全（`grit` 本身）需要 6+；`epf` 这类函数形式的补全在 5.1 上可用。

## 数据模型

### L1：一条别名

```toml
[[alias]]
name = "ll"
cmd = "lsd -l"
os = ""            # 可省略；windows / linux / darwin 限定平台
```

生成 `alias ll='lsd -l'`（bash/zsh）或 `function ll { lsd -l @args }`（pwsh）。

### L2：用同一张表接短名

```toml
[[alias]]
name = "epf"
cmd = "grit file backup"
```

生成的是原生 alias，参数由 shell 自动追加，不额外包一层函数。

### L3：一个包管理器

```toml
name = "conda"
abbr = "c"
requires = "conda"

[actions]              # exec：生成 alias，命令名 = abbr + key
is = "conda install {0}"
ls = "conda list"

[state]                # state：生成 eval 包装，key 即完整命令名
ceat = "conda shell.{shell} activate {0}"
```

占位符：`{0}` 用户参数，`{shell}` 目标 shell 名，`{self}` 二进制自身路径。无参数时 `{0}` 连同前导空格一起移除。

> `[state]` 的 key 是完整命令名而非后缀，因为前缀不规则 —— `conda` 的 `abbr` 是 `c`，但环境动作叫 `ceat` 不是 `cat`。骨架第一版把这两者拼在一起，装配器当场撞上 `cat` 别名。

## 命令

```sh
grit init --shell bash            # 生成 shell 集成，供 eval / source
grit init --shell bash --only-installed   # 只生成依赖已安装的插件
grit init --shell pwsh
grit state conda ceat kaggle      # 打印 shell 代码，由调用方 eval
grit state proxy pxy c            # 按别名设置代理；无参数则清除
grit file backup zs               # L2：备份
grit file restore zs -n           # L2：恢复（dry-run）
grit migrate -n                   # 预演 custom.json -> custom.toml
grit migrate                      # 实际转换
grit self-update                  # 更新本体与插件仓库
grit self-update -f --no-plugins  # 重克隆插件 / 只更新本体
grit conv tohtml notes.md         # 格式转换
grit completion --shell bash      # 生成补全脚本
grit list plugins                 # 查看能力矩阵
grit list conv                    # 查看转换表
```

接入方式（每个 shell 各一次，之后不再修改）：

```sh
eval "$(grit init --shell "${SHELL##*/}")"
```

```powershell
grit init --shell pwsh | Out-String | Invoke-Expression
```

## 已验证

| 项 | 结果 |
|---|---|
| 编译 | 4.1 MB 单文件，`CGO_ENABLED=0` |
| bash / zsh 输出 | alias + 关联数组 + eval 包装语法正确 |
| pwsh 输出 | `function` + `$Global:` hashtable 语法正确 |
| 平台过滤 | Windows 宿主不生成 `ais` / `mis`，生成 `wis` / `wslis` |
| 全量生成 | 131 条 alias（含 12 条转换短名），`ceat` / `pxy` 等 state 包装齐全 |
| 按需生成 | `--only-installed` 跳过项在 stderr 各报一次，不重复 |
| `eval` 通道 | 变量回写成功；`ceat` / `pxy` 均走通 |
| 代理三态 | 别名 `pxy c`、直给端口 `pxy 8080`、无参清除均正确 |
| L2 四个动作 | backup / restore / catalyze / propagate 全部通过 |
| L2 短名 | `epf g` 正确落到 `grit file backup g` |
| 目录复制 | 目标先清空，陈旧文件不残留 |
| 错误隔离 | 单个 key 失败后其余 key 继续 |
| guard | 拒绝以主目录为目标 |
| dry-run | 不产生任何文件 |
| migrate | 真实 custom.json 无损转换：43 个 oxides、代理端口、两组环境别名全部保留 |
| migrate 往返 | 写出的 TOML 读回与转换结果一致 |
| self-update | 真实 git 仓库验证：本地改动被重置，未跟踪文件保留 |
| 格式转换 | md→html、md→ipynb、ipynb→md、wav→mp3 实测通过 |
| in_ext 选择 | `tomd` 对 `.ipynb` 正确切到 jupytext |
| 覆盖保护 | 输出与输入同名时拒绝执行 |
| 补全 | bash 五个补全点实测通过；zsh / pwsh 脚本通过语法检查 |
| 交叉编译 | `goreleaser release --snapshot` 复现六目标，归档名 `grit_<ver>_<os>_<arch>`；tar 内为 `grit`，zip 内为 `grit.exe` |
| 发布配置 | `goreleaser check` 通过（v2.18.3）；`sha256sums.txt` 为 `<hash>  <name>`，与 release workflow 的解析一致 |
| 版本注入 | 归档内 `grit.exe version` 输出 `grit 0.0.1-next`，与 snapshot 版本一致 |
| 独立分发 | 二进制脱离仓库可 `propagate`：模板来自内嵌 FS，内容与仓库模板一致 |
| 安装脚本 | `install.sh` / `install.ps1` 语法检查通过 |
| 单元测试 | 43 项，`go test ./...` 通过 |

## 安装

```sh
curl -fsSL https://raw.githubusercontent.com/ivaquero/grit/master/install.sh | sh
```

```powershell
irm https://raw.githubusercontent.com/ivaquero/grit/master/install.ps1 | iex
```

两者都从 GitHub Release 下载对应二进制，装到 `~/.local/bin`（Windows 为 `~/bin`）。`GRIT_VERSION` 可指定版本，`GRIT_BINDIR` 可改安装位置。

安装后只需一行接入，`defaults/` 全部内嵌在二进制里，所以不需要克隆仓库：

```sh
eval "$(grit init --shell ${SHELL##*/})"
```

用户配置放在系统配置目录（`~/.config/grit/custom.toml`）。若设置了 `GRIT`，则回到仓库模式，配置从仓库根读取。

## 构建与发布

本地验证：

```sh
go vet ./...
go test ./...
go build -trimpath -ldflags "-s -w -X main.version=$(git describe --tags --always)" -o grit .
goreleaser release --snapshot --clean   # 复现 CI 的六目标归档，不发布
```

发布完全由 `.goreleaser.yaml` 声明，workflow 只负责调用：

| workflow | 触发 | 产物 |
|---|---|---|
| `ci.yml` | push / PR / 手动 | 三平台 `go vet` + `go test`；随后 `goreleaser release --snapshot` 产出六目标归档，作为 artifact 保留 7 天 |
| `release.yml` | 推 `v*` 标签 | `goreleaser release --clean`：建 Release、上传归档与 `sha256sums.txt`，再由 checksums 反推补发 `grit.json`（Scoop manifest） |

| 目标 | 归档 |
|---|---|
| linux/amd64、linux/arm64 | `grit_<ver>_linux_<arch>.tar.gz` |
| darwin/amd64、darwin/arm64 | `grit_<ver>_darwin_<arch>.tar.gz` |
| windows/amd64、windows/arm64 | `grit_<ver>_windows_<arch>.zip` |

snapshot 与正式发布共用同一套命名模板，所以 PR 上验收到的产物结构与 Release 完全一致。二进制 `CGO_ENABLED=0` + `-s -w -trimpath`，约 3.6 MB（压缩后 1.3–1.5 MB）；版本号经 `-X main.version={{ .Version }}` 注入，`grit version` 可核对。

发版方式是纯标签驱动：

```sh
git tag v0.1.0
git push origin v0.1.0
```

## 尚未实现

- `lang_julia` 的环境管理（多前缀 + 复杂条件，属 L3）
- `vscl` 等缓存清理命令、`pdref`（固定输出文件名，需要额外字段）
- 发布流水线尚未跑过真实 tag：`goreleaser` 建 Release 与 Scoop manifest 补发这两步待首次 `v*` 验证
- zsh / pwsh 补全只做了语法检查，未在真实交互式会话里逐键验证
- `requires` 只用于 `--only-installed` 过滤；运行时缺工具才报错，尚无启动期检查
