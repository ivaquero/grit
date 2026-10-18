#!/bin/sh
# Goboot installer for macOS / Linux.
# Downloads a release binary; the repository is not needed at runtime.
set -eu

# 发布仓库地址；GitHub 上改名后同步更新。
REPO="${GOBOOT_REPO:-ivaquero/goboot}"
BINDIR="${GOBOOT_BINDIR:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
darwin | linux) ;;
*)
    echo "goboot: unsupported OS '$os' (use install.ps1 on Windows)" >&2
    exit 1
    ;;
esac

arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*)
    echo "goboot: unsupported architecture '$arch'" >&2
    exit 1
    ;;
esac

version="${GOBOOT_VERSION:-}"
if [ -z "$version" ]; then
        version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases?per_page=30" |
        grep '"tag_name"' | cut -d'"' -f4 | grep -E '^v[0-9]' | head -n1 | sed 's/^v//') || true
fi
if [ -z "${version:-}" ]; then
    echo "goboot: cannot determine the latest release; set GOBOOT_VERSION" >&2
    exit 1
fi

name="ox_${version}_${os}_${arch}"
url="https://github.com/${REPO}/releases/download/v${version}/${name}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "goboot: downloading ${version} (${os}/${arch})"
curl -fsSL "$url" -o "${tmp}/${name}.tar.gz"
tar -xzf "${tmp}/${name}.tar.gz" -C "$tmp"

mkdir -p "$BINDIR"
install -m 755 "${tmp}/goboot" "${BINDIR}/goboot"
echo "goboot: installed ${BINDIR}/goboot"

case ":${PATH}:" in
*":${BINDIR}:"*) ;;
*) echo "goboot: ${BINDIR} is not in PATH" ;;
esac

cat <<'EOF'

Next: add this line to your shell rc (~/.zshrc or ~/.bashrc):

  eval "$(goboot init --shell ${SHELL##*/})"

Then reload the shell. Completion is optional:

  eval "$(goboot completion --shell ${SHELL##*/})"
EOF
