#!/bin/sh
# Install a StreamForge release for the current user.
set -eu

platform=$(uname -s)
machine=$(uname -m)
case "$platform" in
    Linux) os=linux ;;
    Darwin)
        os=darwin
        if [ "$(sysctl -n hw.optional.arm64 2>/dev/null || true)" = 1 ]; then
            machine=arm64
        fi ;;
    *) echo "No StreamForge shell installer for $platform." >&2; exit 1 ;;
esac

case "$machine" in
    x86_64) arch=amd64 ;;
    i[3-6]86) arch=386 ;;
    armv[5-8]l) arch=arm ;;
    loongarch64) arch=loong64 ;;
    mips|mips64|mips64el|mips64le|mipsel|mipsle)
        case "$(od -An -tu1 -j4 -N2 /proc/self/exe | xargs)" in
            "1 1") arch=mipsle ;;
            "1 2") arch=mips ;;
            "2 1") arch=mips64le ;;
            "2 2") arch=mips64 ;;
            *) echo "Could not determine the MIPS userspace architecture." >&2; exit 1 ;;
        esac ;;
    ppc64|ppc64le|riscv64|s390x) arch=$machine ;;
    aarch64|arm64) arch=arm64 ;;
    *) echo "No StreamForge release for $platform $machine." >&2; exit 1 ;;
esac

missing=
for tool in ffmpeg ffprobe; do
    if ! command -v "$tool" >/dev/null 2>&1 || ! "$tool" -version >/dev/null 2>&1; then
        missing="$missing $tool"
    elif [ "$os" = darwin ] && ! "$tool" -protocols 2>/dev/null | grep -Eq '^[[:space:]]+fd[[:space:]]*$'; then
        missing="$missing $tool (requires fd protocol)"
    fi
done
if [ -n "$missing" ]; then
    echo "StreamForge requires runnable programs on PATH. Missing:$missing" >&2
    if [ "$os" = darwin ]; then
        echo '  brew install ffmpeg' >&2
    elif command -v apt-get >/dev/null 2>&1; then
        echo '  sudo apt-get update && sudo apt-get install ffmpeg' >&2
    elif command -v apk >/dev/null 2>&1; then
        echo '  sudo apk add ffmpeg' >&2
    elif command -v pacman >/dev/null 2>&1; then
        echo '  sudo pacman -S ffmpeg' >&2
    elif command -v dnf >/dev/null 2>&1; then
        echo '  sudo dnf install ffmpeg-free' >&2
    elif command -v zypper >/dev/null 2>&1; then
        echo '  sudo zypper install ffmpeg' >&2
    else
        echo 'Install FFmpeg from your distribution packages and make both programs available on PATH.' >&2
    fi
    exit 1
fi

repo=https://github.com/kpirnie/streamforge/releases
if [ -n "${STREAMFORGE_RELEASE:-}" ]; then
    case "$STREAMFORGE_RELEASE" in
        *[!a-zA-Z0-9._-]*) echo 'Invalid STREAMFORGE_RELEASE tag.' >&2; exit 1 ;;
    esac
    url="$repo/download/$STREAMFORGE_RELEASE"
else
    url="$repo/latest/download"
fi

bin_dir="${STREAMFORGE_BIN_DIR:-$HOME/.local/bin}"
asset="streamforge-$os-$arch"
mkdir -p -- "$bin_dir"
tmp=$(mktemp -d "$bin_dir/.streamforge.XXXXXX")
trap 'rm -rf -- "$tmp"' EXIT
trap 'exit 1' HUP INT TERM

curl -fsSL "$url/$asset" -o "$tmp/$asset"
curl -fsSL "$url/SHA256SUMS" -o "$tmp/SHA256SUMS"
(
    cd -- "$tmp"
    awk -v name="$asset" '$2 == name { print; found++ } END { if (found != 1) exit 1 }' SHA256SUMS > selected.sha256
    if [ "$os" = darwin ]; then
        shasum -a 256 -c selected.sha256
    else
        sha256sum -c selected.sha256
    fi
)
chmod 0755 "$tmp/$asset"
mv -- "$tmp/$asset" "$tmp/streamforge"
mv -f -- "$tmp/streamforge" "$bin_dir/"
printf 'Installed StreamForge at %s/streamforge\n' "$bin_dir"
case ":$PATH:" in
    *":$bin_dir:"*) ;;
    *) printf 'Add %s to PATH to run streamforge by name.\n' "$bin_dir" ;;
esac
